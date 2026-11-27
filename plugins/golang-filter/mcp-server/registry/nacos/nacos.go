package nacos

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/registry"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// NacosMcpRegistry keeps the MCP tool catalogue discovered from Nacos in
// memory. Tool lookups happen on request-handler goroutines while service and
// configuration callbacks refresh the catalogue from independent goroutines,
// so every mutable field is protected by mu.
//
// Locking discipline:
//   - serviceMatcher is immutable after construction and may be read without a
//     lock.
//   - mu protects toolsDescription, toolsRpcContext, currentServiceSet and
//     toolChangeEventListeners.
//   - mu is never held while talking to Nacos or while invoking a listener.
//     New tool entries are assembled in local maps first and only published
//     under the write lock, which keeps the critical section short and avoids
//     blocking handlers on network I/O.
type NacosMcpRegistry struct {
	serviceMatcher map[string]string
	configClient   config_client.IConfigClient
	namingClient   naming_client.INamingClient

	mu sync.RWMutex

	toolsDescription         map[string]*registry.ToolDescription
	toolsRpcContext          map[string]*registry.RpcContext
	toolChangeEventListeners []registry.ToolChangeEventListener
	currentServiceSet        map[string]bool
}

const (
	DEFAULT_SERVICE_LIST_MAX_PGSIZXE = 10000
	MCP_TOOL_SUBFIX                  = "-mcp-tools.json"
)

func (n *NacosMcpRegistry) ListToolsDescription() []*registry.ToolDescription {
	n.mu.RLock()
	if n.toolsDescription != nil {
		result := n.snapshotToolsDescriptionLocked()
		n.mu.RUnlock()
		return result
	}
	n.mu.RUnlock()

	// Preserve the lazy initialisation behaviour: callers that arrive before
	// the periodic refresh performed its first pass get an immediate refresh.
	n.refreshToolsList()

	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.snapshotToolsDescriptionLocked()
}

func (n *NacosMcpRegistry) GetToolRpcContext(toolName string) (*registry.RpcContext, bool) {
	n.mu.RLock()
	if n.toolsRpcContext != nil {
		context, ok := n.toolsRpcContext[toolName]
		n.mu.RUnlock()
		return context, ok
	}
	n.mu.RUnlock()

	// As with ListToolsDescription, do the lazy refresh without holding the
	// read lock because refreshing acquires the write lock itself.
	n.refreshToolsList()

	n.mu.RLock()
	defer n.mu.RUnlock()
	context, ok := n.toolsRpcContext[toolName]
	return context, ok
}

func (n *NacosMcpRegistry) RegisterToolChangeEventListener(listener registry.ToolChangeEventListener) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.toolChangeEventListeners = append(n.toolChangeEventListeners, listener)
}

func (n *NacosMcpRegistry) snapshotToolsDescriptionLocked() []*registry.ToolDescription {
	result := make([]*registry.ToolDescription, 0, len(n.toolsDescription))
	for _, tool := range n.toolsDescription {
		result = append(result, tool)
	}
	return result
}

// notifyToolChangeListeners copies the listener slice while holding the read
// lock and invokes the callbacks after releasing it. Listeners commonly call
// back into the registry to list the new tools, so invoking them under mu
// would deadlock.
func (n *NacosMcpRegistry) notifyToolChangeListeners() {
	n.mu.RLock()
	listeners := make([]registry.ToolChangeEventListener, len(n.toolChangeEventListeners))
	copy(listeners, n.toolChangeEventListeners)
	n.mu.RUnlock()

	for _, listener := range listeners {
		listener.OnToolChanged(n)
	}
}

func (n *NacosMcpRegistry) refreshToolsList() bool {
	changed := false
	for group, serviceMatcher := range n.serviceMatcher {
		if n.refreshToolsListForGroup(group, serviceMatcher) {
			changed = true
		}
	}
	return changed
}

func (n *NacosMcpRegistry) refreshToolsListForGroup(group string, serviceMatcher string) bool {
	// The Nacos request must run without any registry lock held.
	services, err := n.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
		GroupName: group,
		PageNo:    1,
		PageSize:  DEFAULT_SERVICE_LIST_MAX_PGSIZXE,
	})
	if err != nil {
		api.LogError(fmt.Sprintf("Get service list error when refresh tools list for group %s, error %s", group, err))
		return false
	}

	changed := false
	serviceList := services.Doms
	pattern, err := regexp.Compile(serviceMatcher)
	if err != nil {
		api.LogErrorf("Match service error for pattern %s", serviceMatcher)
		return false
	}

	// Snapshot the services that already have tools so the loop below can make
	// subscription decisions without reading a map that callbacks may mutate.
	n.mu.RLock()
	knownServices := make(map[string]bool, len(n.currentServiceSet))
	for serviceName := range n.currentServiceSet {
		knownServices[serviceName] = true
	}
	n.mu.RUnlock()

	currentServiceList := make(map[string]bool, len(serviceList))

	for _, service := range serviceList {
		if !pattern.MatchString(service) {
			continue
		}

		formatServiceName := getFormatServiceName(group, service)
		if !knownServices[formatServiceName] {
			refreshed := n.refreshToolsListForService(group, service)
			n.listenToService(group, service)
			if refreshed {
				changed = true
			}
		}

		currentServiceList[formatServiceName] = true
	}

	// Only the in-memory bookkeeping is done under the write lock. Removing a
	// service and all of its tools is a single atomic catalogue update from a
	// handler's point of view.
	n.mu.Lock()
	defer n.mu.Unlock()

	for serviceName := range n.currentServiceSet {
		if !strings.HasPrefix(serviceName, group) {
			continue
		}

		if _, ok := currentServiceList[serviceName]; !ok {
			changed = true
			n.deleteToolsByPrefixLocked(serviceName)
			delete(n.currentServiceSet, serviceName)
		}
	}

	return changed
}

func getFormatServiceName(group string, service string) string {
	return fmt.Sprintf("%s_%s", group, service)
}

func (n *NacosMcpRegistry) deleteToolForService(group string, service string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.deleteToolsByPrefixLocked(getFormatServiceName(group, service))
}

// deleteToolsByPrefixLocked removes every tool belonging to a service. The
// caller must hold n.mu for writing.
func (n *NacosMcpRegistry) deleteToolsByPrefixLocked(prefix string) {
	for toolName := range n.toolsDescription {
		if strings.HasPrefix(toolName, prefix) {
			delete(n.toolsDescription, toolName)
			delete(n.toolsRpcContext, toolName)
		}
	}
}

func (n *NacosMcpRegistry) refreshToolsListForServiceWithContent(group string, service string, newConfig *string, instances *[]model.Instance) bool {
	if newConfig == nil {
		dataId := makeToolsConfigId(service)
		content, err := n.configClient.GetConfig(vo.ConfigParam{
			DataId: dataId,
			Group:  group,
		})
		if err != nil {
			api.LogError(fmt.Sprintf("Get tools config for sercice %s:%s error %s", group, service, err))
			return false
		}

		newConfig = &content
	}

	if instances == nil {
		instancesFromNacos, err := n.namingClient.SelectInstances(vo.SelectInstancesParam{
			ServiceName: service,
			GroupName:   group,
			HealthyOnly: true,
		})
		if err != nil {
			api.LogError(fmt.Sprintf("List instance for sercice %s:%s error %s", group, service, err))
			return false
		}

		instances = &instancesFromNacos
	}

	var applicationDescription registry.McpApplicationDescription
	if newConfig == nil {
		return false
	}

	// config deleted, tools should be removed
	if len(*newConfig) == 0 {
		n.deleteToolForService(group, service)
		return true
	}

	err := json.Unmarshal([]byte(*newConfig), &applicationDescription)
	if err != nil {
		api.LogError(fmt.Sprintf("Parse tools config for sercice %s:%s error, config is %s, error is %s", group, service, *newConfig, err))
		return false
	}

	wrappedInstances := make([]registry.Instance, 0, len(*instances))
	for _, instance := range *instances {
		wrappedInstance := registry.Instance{
			Host: instance.Ip,
			Port: instance.Port,
			Meta: instance.Metadata,
		}
		wrappedInstances = append(wrappedInstances, wrappedInstance)
	}

	// Build the replacement catalogue in local maps. Credential lookups in
	// particular are network calls and must not run while holding n.mu, so the
	// registry is only updated once everything is ready.
	tools := make(map[string]*registry.ToolDescription, len(applicationDescription.ToolsDescription))
	rpcContexts := make(map[string]*registry.RpcContext, len(applicationDescription.ToolsDescription))

	for _, tool := range applicationDescription.ToolsDescription {
		meta := applicationDescription.ToolsMeta[tool.Name]

		var cred *registry.CredentialInfo
		credentialRef := meta.CredentialRef
		if credentialRef != nil {
			cred = n.GetCredential(*credentialRef, group)
		}

		context := &registry.RpcContext{
			ToolMeta:   meta,
			Instances:  &wrappedInstances,
			Protocol:   applicationDescription.Protocol,
			Credential: cred,
		}

		tool.Name = makeToolName(group, service, tool.Name)
		tools[tool.Name] = tool
		rpcContexts[tool.Name] = context
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.toolsDescription == nil {
		n.toolsDescription = make(map[string]*registry.ToolDescription)
	}
	if n.toolsRpcContext == nil {
		n.toolsRpcContext = make(map[string]*registry.RpcContext)
	}
	if n.currentServiceSet == nil {
		n.currentServiceSet = make(map[string]bool)
	}

	// Replace this service's entries atomically so readers never observe a
	// half-refreshed catalogue.
	n.deleteToolsByPrefixLocked(getFormatServiceName(group, service))
	for name, tool := range tools {
		n.toolsDescription[name] = tool
	}
	for name, context := range rpcContexts {
		n.toolsRpcContext[name] = context
	}
	n.currentServiceSet[getFormatServiceName(group, service)] = true
	return true
}

func (n *NacosMcpRegistry) GetCredential(name string, group string) *registry.CredentialInfo {
	dataId := makeCredentialDataId(name)
	content, err := n.configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		api.LogError(fmt.Sprintf("Get credentials for %s:%s error %s", group, dataId, err))
		return nil
	}

	var credential registry.CredentialInfo
	err = json.Unmarshal([]byte(content), &credential)
	if err != nil {
		api.LogError(fmt.Sprintf("Parse credentials for %s:%s error %s", group, dataId, err))
		return nil
	}

	return &credential
}

func (n *NacosMcpRegistry) refreshToolsListForService(group string, service string) bool {
	return n.refreshToolsListForServiceWithContent(group, service, nil, nil)
}

func (n *NacosMcpRegistry) listenToService(group string, service string) {
	// config changed, tools description may be changed
	err := n.configClient.ListenConfig(vo.ConfigParam{
		DataId: makeToolsConfigId(service),
		Group:  group,
		OnChange: func(namespace, group, dataId, data string) {
			n.refreshToolsListForServiceWithContent(group, service, &data, nil)
			n.notifyToolChangeListeners()
		},
	})
	if err != nil {
		api.LogError(fmt.Sprintf("Listen to service's tool config error %s", err))
	}

	err = n.namingClient.Subscribe(&vo.SubscribeParam{
		ServiceName: service,
		GroupName:   group,
		SubscribeCallback: func(services []model.Instance, err error) {
			n.refreshToolsListForServiceWithContent(group, service, nil, &services)
			n.notifyToolChangeListeners()
		},
	})
	if err != nil {
		api.LogError(fmt.Sprintf("Listen to service's tool instance list error %s", err))
	}
}

func makeToolName(group string, service string, toolName string) string {
	return fmt.Sprintf("%s_%s_%s", group, service, toolName)
}

func makeToolsConfigId(service string) string {
	return service + MCP_TOOL_SUBFIX
}

func makeCredentialDataId(credentialName string) string {
	return credentialName
}
