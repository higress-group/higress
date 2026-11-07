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

type NacosMcpRegistry struct {
	// mu guards toolsDescription, toolsRpcContext and currentServiceSet, which are
	// mutated by the background poll goroutine and the Nacos config/naming callback
	// goroutines while being read on the Envoy request path.
	mu                       sync.RWMutex
	serviceMatcher           map[string]string
	configClient             config_client.IConfigClient
	namingClient             naming_client.INamingClient
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
	defer n.mu.RUnlock()
	result := []*registry.ToolDescription{}
	for _, tool := range n.toolsDescription {
		result = append(result, tool)
	}
	return result
}

func (n *NacosMcpRegistry) GetToolRpcContext(toolName string) (*registry.RpcContext, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	tool, ok := n.toolsRpcContext[toolName]
	return tool, ok
}

func (n *NacosMcpRegistry) RegisterToolChangeEventListener(listener registry.ToolChangeEventListener) {
	n.toolChangeEventListeners = append(n.toolChangeEventListeners, listener)
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

	currentServiceList := map[string]bool{}

	for _, service := range serviceList {
		if !pattern.MatchString(service) {
			continue
		}

		formatServiceName := getFormatServiceName(group, service)
		n.mu.RLock()
		_, known := n.currentServiceSet[formatServiceName]
		n.mu.RUnlock()
		if !known {
			refreshed := n.refreshToolsListForService(group, service)
			n.listenToService(group, service)
			if refreshed {
				changed = true
			}
		}

		currentServiceList[formatServiceName] = true
	}

	n.mu.Lock()
	serviceShouldBeDeleted := []string{}
	for serviceName := range n.currentServiceSet {
		if !strings.HasPrefix(serviceName, group) {
			continue
		}

		if _, ok := currentServiceList[serviceName]; !ok {
			serviceShouldBeDeleted = append(serviceShouldBeDeleted, serviceName)
			changed = true
			toolsShouldBeDeleted := []string{}
			for toolName := range n.toolsDescription {
				if strings.HasPrefix(toolName, serviceName) {
					toolsShouldBeDeleted = append(toolsShouldBeDeleted, toolName)
				}
			}

			for _, toolName := range toolsShouldBeDeleted {
				delete(n.toolsDescription, toolName)
				delete(n.toolsRpcContext, toolName)
			}
		}
	}

	for _, service := range serviceShouldBeDeleted {
		delete(n.currentServiceSet, service)
	}
	n.mu.Unlock()

	return changed
}

func getFormatServiceName(group string, service string) string {
	return fmt.Sprintf("%s_%s", group, service)
}

// deleteToolForService removes all tools registered for a service. Callers must
// hold n.mu for writing.
func (n *NacosMcpRegistry) deleteToolForService(group string, service string) {
	toolsNeedReset := []string{}

	formatServiceName := getFormatServiceName(group, service)
	for tool := range n.toolsDescription {
		if strings.HasPrefix(tool, formatServiceName) {
			toolsNeedReset = append(toolsNeedReset, tool)
		}
	}

	for _, tool := range toolsNeedReset {
		delete(n.toolsDescription, tool)
		delete(n.toolsRpcContext, tool)
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
		n.mu.Lock()
		n.deleteToolForService(group, service)
		n.mu.Unlock()
		return true
	}

	err := json.Unmarshal([]byte(*newConfig), &applicationDescription)
	if err != nil {
		api.LogError(fmt.Sprintf("Parse tools config for sercice %s:%s error, config is %s, error is %s", group, service, *newConfig, err))
		return false
	}

	wrappedInstances := []registry.Instance{}
	for _, instance := range *instances {
		wrappedInstance := registry.Instance{
			Host: instance.Ip,
			Port: instance.Port,
			Meta: instance.Metadata,
		}
		wrappedInstances = append(wrappedInstances, wrappedInstance)
	}

	// Build the tool entries before taking the lock: GetCredential performs a
	// blocking Nacos network call and must not run inside the critical section.
	type toolEntry struct {
		name    string
		desc    *registry.ToolDescription
		context *registry.RpcContext
	}
	entries := make([]toolEntry, 0, len(applicationDescription.ToolsDescription))
	for _, tool := range applicationDescription.ToolsDescription {
		meta := applicationDescription.ToolsMeta[tool.Name]

		var cred *registry.CredentialInfo
		credentialRef := meta.CredentialRef
		if credentialRef != nil {
			cred = n.GetCredential(*credentialRef, group)
		}

		context := registry.RpcContext{
			ToolMeta:   meta,
			Instances:  &wrappedInstances,
			Protocol:   applicationDescription.Protocol,
			Credential: cred,
		}

		tool.Name = makeToolName(group, service, tool.Name)
		entries = append(entries, toolEntry{name: tool.Name, desc: tool, context: &context})
	}

	n.mu.Lock()
	n.deleteToolForService(group, service)
	for _, entry := range entries {
		n.toolsDescription[entry.name] = entry.desc
		n.toolsRpcContext[entry.name] = entry.context
	}
	n.currentServiceSet[getFormatServiceName(group, service)] = true
	n.mu.Unlock()
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
			for _, listener := range n.toolChangeEventListeners {
				listener.OnToolChanged(n)
			}
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
			for _, listener := range n.toolChangeEventListeners {
				listener.OnToolChanged(n)
			}
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
