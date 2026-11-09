package nacos

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/registry"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// fakeConfigClient implements only the config operations used by the registry.
// The embedded interface keeps the stub small while still satisfying
// config_client.IConfigClient.
type fakeConfigClient struct {
	config_client.IConfigClient

	mu        sync.Mutex
	configs   map[string]string
	listeners []func(namespace, group, dataId, data string)
}

func newFakeConfigClient() *fakeConfigClient {
	return &fakeConfigClient{configs: map[string]string{}}
}

func configKey(group, dataId string) string {
	return group + "/" + dataId
}

func (c *fakeConfigClient) setConfig(group, dataId, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[configKey(group, dataId)] = content
}

func (c *fakeConfigClient) GetConfig(param vo.ConfigParam) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	content, ok := c.configs[configKey(param.Group, param.DataId)]
	if !ok {
		return "", fmt.Errorf("config %s not found", configKey(param.Group, param.DataId))
	}
	return content, nil
}

func (c *fakeConfigClient) ListenConfig(param vo.ConfigParam) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if param.OnChange != nil {
		c.listeners = append(c.listeners, param.OnChange)
	}
	return nil
}

func (c *fakeConfigClient) callbacks() []func(namespace, group, dataId, data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]func(namespace, group, dataId, data string), len(c.listeners))
	copy(result, c.listeners)
	return result
}

// fakeNamingClient implements the discovery and subscription operations used
// by the registry.
type fakeNamingClient struct {
	naming_client.INamingClient

	mu        sync.Mutex
	services  []string
	instances map[string][]model.Instance
	listeners []func([]model.Instance, error)
}

func newFakeNamingClient() *fakeNamingClient {
	return &fakeNamingClient{instances: map[string][]model.Instance{}}
}

func (c *fakeNamingClient) setServices(services ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services = append([]string(nil), services...)
}

func (c *fakeNamingClient) setInstances(service string, instances []model.Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instances[service] = append([]model.Instance(nil), instances...)
}

func (c *fakeNamingClient) GetAllServicesInfo(param vo.GetAllServiceInfoParam) (model.ServiceList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	services := append([]string(nil), c.services...)
	return model.ServiceList{Doms: services}, nil
}

func (c *fakeNamingClient) SelectInstances(param vo.SelectInstancesParam) ([]model.Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]model.Instance(nil), c.instances[param.ServiceName]...), nil
}

func (c *fakeNamingClient) Subscribe(param *vo.SubscribeParam) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if param.SubscribeCallback != nil {
		c.listeners = append(c.listeners, param.SubscribeCallback)
	}
	return nil
}

func (c *fakeNamingClient) callbacks() []func([]model.Instance, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]func([]model.Instance, error), len(c.listeners))
	copy(result, c.listeners)
	return result
}

type countingListener struct {
	count int64
}

func (l *countingListener) OnToolChanged(reg registry.McpServerRegistry) {
	atomic.AddInt64(&l.count, 1)
	// Exercise the same read paths that request handlers use. The listener
	// must be able to run without the registry holding its write lock.
	reg.ListToolsDescription()
	reg.GetToolRpcContext(makeToolName("group", "svc", "echo"))
}

func (l *countingListener) calls() int64 {
	return atomic.LoadInt64(&l.count)
}

// registeringListener adds another listener during notification. Notification
// must use the snapshot taken before callbacks run, so the late listener is
// only invoked by a subsequent notification.
type registeringListener struct {
	once   sync.Once
	reg    *NacosMcpRegistry
	target *countingListener
}

func (l *registeringListener) OnToolChanged(_ registry.McpServerRegistry) {
	l.once.Do(func() {
		l.reg.RegisterToolChangeEventListener(l.target)
	})
}

func toolsConfig(protocol string, names ...string) string {
	description := registry.McpApplicationDescription{
		Protocol:  protocol,
		ToolsMeta: make(map[string]registry.ToolMeta, len(names)),
	}
	for _, name := range names {
		description.ToolsDescription = append(description.ToolsDescription, &registry.ToolDescription{
			Name:        name,
			Description: "description for " + name,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		})
		description.ToolsMeta[name] = registry.ToolMeta{
			InvokeContext: map[string]string{"path": "/" + name},
		}
	}
	encoded, err := json.Marshal(description)
	if err != nil {
		panic(fmt.Sprintf("marshal tools config: %v", err))
	}
	return string(encoded)
}

func newRegistry(config *fakeConfigClient, naming *fakeNamingClient) *NacosMcpRegistry {
	return &NacosMcpRegistry{
		serviceMatcher: map[string]string{"group": ".*"},
		configClient:   config,
		namingClient:   naming,
	}
}

func TestConcurrentRefreshCallbacksAndReads(t *testing.T) {
	const (
		group   = "group"
		service = "svc"
	)

	config := newFakeConfigClient()
	config.setConfig(group, makeToolsConfigId(service), toolsConfig("http", "echo", "sum"))

	naming := newFakeNamingClient()
	naming.setServices(service)
	naming.setInstances(service, []model.Instance{{
		Ip:       "127.0.0.1",
		Port:     8080,
		Metadata: map[string]string{"zone": "test"},
	}})

	reg := newRegistry(config, naming)
	listener := &countingListener{}
	reg.RegisterToolChangeEventListener(listener)

	if !reg.refreshToolsList() {
		t.Fatal("initial refresh did not report any change")
	}
	if got := len(reg.ListToolsDescription()); got != 2 {
		t.Fatalf("initial tools count = %d, want 2", got)
	}
	if _, ok := reg.GetToolRpcContext(makeToolName(group, service, "echo")); !ok {
		t.Fatal("echo tool is missing after initial refresh")
	}

	configCallbacks := config.callbacks()
	if len(configCallbacks) != 1 {
		t.Fatalf("config listeners = %d, want 1", len(configCallbacks))
	}
	instanceCallbacks := naming.callbacks()
	if len(instanceCallbacks) != 1 {
		t.Fatalf("instance listeners = %d, want 1", len(instanceCallbacks))
	}
	configCallback := configCallbacks[0]
	instanceCallback := instanceCallbacks[0]

	const iterations = 150
	emptyConfig := ""
	start := make(chan struct{})
	var wait sync.WaitGroup

	run := func(work func(int)) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for i := 0; i < iterations; i++ {
				work(i)
			}
		}()
	}

	// Full refresh goroutine, mirroring the permanent refresh loop.
	run(func(int) {
		reg.refreshToolsList()
	})

	// Direct catalogue replacement, mirroring the config callback path.
	run(func(i int) {
		if i%11 == 0 {
			reg.refreshToolsListForServiceWithContent(group, service, &emptyConfig, nil)
			return
		}
		content := toolsConfig("http", "echo")
		if i%3 == 0 {
			content = toolsConfig("http", "echo", "sum")
		}
		reg.refreshToolsListForServiceWithContent(group, service, &content, nil)
	})

	// The callbacks installed by listenToService refresh from their own
	// goroutines in production, so invoke them concurrently here as well.
	run(func(i int) {
		content := toolsConfig("http", "echo")
		if i%2 == 0 {
			content = toolsConfig("http", "echo", "sum")
		}
		configCallback("", group, makeToolsConfigId(service), content)
	})
	run(func(int) {
		instanceCallback([]model.Instance{{
			Ip:       "10.0.0.1",
			Port:     9090,
			Metadata: map[string]string{"zone": "callback"},
		}}, nil)
	})

	// Request-handler read paths.
	run(func(int) {
		reg.ListToolsDescription()
	})
	run(func(int) {
		reg.GetToolRpcContext(makeToolName(group, service, "echo"))
	})

	// Listener registration also mutates the registry state.
	run(func(int) {
		reg.RegisterToolChangeEventListener(&countingListener{})
	})

	close(start)
	wait.Wait()

	if listener.calls() == 0 {
		t.Fatal("listener was never notified")
	}
}

func TestRefreshToolsListForServiceReplacesEntries(t *testing.T) {
	const (
		group   = "g"
		service = "s"
	)
	reg := &NacosMcpRegistry{}
	instances := []model.Instance{{
		Ip:       "10.0.0.1",
		Port:     9090,
		Metadata: map[string]string{"zone": "first"},
	}}

	first := toolsConfig("https", "echo", "sum")
	if !reg.refreshToolsListForServiceWithContent(group, service, &first, &instances) {
		t.Fatal("first refresh reported no change")
	}
	if got := len(reg.ListToolsDescription()); got != 2 {
		t.Fatalf("tools count = %d, want 2", got)
	}

	echoName := makeToolName(group, service, "echo")
	echoContext, ok := reg.GetToolRpcContext(echoName)
	if !ok {
		t.Fatal("echo tool is missing")
	}
	if echoContext.Protocol != "https" {
		t.Fatalf("protocol = %q, want https", echoContext.Protocol)
	}
	if echoContext.Instances == nil || len(*echoContext.Instances) != 1 {
		t.Fatalf("instances = %v, want one entry", echoContext.Instances)
	}
	if got := (*echoContext.Instances)[0].Host; got != "10.0.0.1" {
		t.Fatalf("instance host = %q, want 10.0.0.1", got)
	}

	second := toolsConfig("https", "echo")
	if !reg.refreshToolsListForServiceWithContent(group, service, &second, &instances) {
		t.Fatal("second refresh reported no change")
	}
	if _, ok := reg.GetToolRpcContext(makeToolName(group, service, "sum")); ok {
		t.Fatal("sum tool should have been removed")
	}
	if got := len(reg.ListToolsDescription()); got != 1 {
		t.Fatalf("tools count after replacement = %d, want 1", got)
	}

	empty := ""
	if !reg.refreshToolsListForServiceWithContent(group, service, &empty, &instances) {
		t.Fatal("empty config refresh reported no change")
	}
	if got := len(reg.ListToolsDescription()); got != 0 {
		t.Fatalf("tools count after deletion = %d, want 0", got)
	}
	if _, ok := reg.GetToolRpcContext(echoName); ok {
		t.Fatal("echo tool should have been removed by an empty config")
	}
}

func TestRefreshToolsListForGroupAddsAndRemovesServices(t *testing.T) {
	const group = "group"

	config := newFakeConfigClient()
	config.setConfig(group, makeToolsConfigId("svc"), toolsConfig("http", "echo"))

	naming := newFakeNamingClient()
	naming.setServices("svc")
	naming.setInstances("svc", []model.Instance{{Ip: "127.0.0.1", Port: 8080}})

	reg := &NacosMcpRegistry{
		serviceMatcher: map[string]string{group: ".*"},
		configClient:   config,
		namingClient:   naming,
	}

	if !reg.refreshToolsListForGroup(group, ".*") {
		t.Fatal("adding a service reported no change")
	}
	if _, ok := reg.GetToolRpcContext(makeToolName(group, "svc", "echo")); !ok {
		t.Fatal("service tool was not added")
	}
	if len(config.callbacks()) != 1 || len(naming.callbacks()) != 1 {
		t.Fatal("service callbacks were not registered")
	}

	naming.setServices()
	if !reg.refreshToolsListForGroup(group, ".*") {
		t.Fatal("removing a service reported no change")
	}
	if got := len(reg.ListToolsDescription()); got != 0 {
		t.Fatalf("tools count after service removal = %d, want 0", got)
	}

	reg.mu.RLock()
	remaining := len(reg.currentServiceSet)
	reg.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("currentServiceSet size = %d, want 0", remaining)
	}
}

func TestConcurrentListenerRegistrationAndNotification(t *testing.T) {
	reg := &NacosMcpRegistry{
		toolsDescription:  map[string]*registry.ToolDescription{},
		toolsRpcContext:   map[string]*registry.RpcContext{},
		currentServiceSet: map[string]bool{},
	}

	const listenerCount = 64
	listeners := make([]*countingListener, listenerCount)
	start := make(chan struct{})
	var wait sync.WaitGroup

	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		for i := 0; i < 40; i++ {
			reg.notifyToolChangeListeners()
			reg.ListToolsDescription()
		}
	}()

	for i := range listeners {
		listeners[i] = &countingListener{}
		wait.Add(1)
		go func(listener *countingListener) {
			defer wait.Done()
			<-start
			reg.RegisterToolChangeEventListener(listener)
		}(listeners[i])
	}

	close(start)
	wait.Wait()

	// Every listener is registered now, so one final notification must reach
	// all of them.
	reg.notifyToolChangeListeners()
	var total int64
	for _, listener := range listeners {
		total += listener.calls()
	}
	if total < listenerCount {
		t.Fatalf("listener callbacks = %d, want at least %d", total, listenerCount)
	}
}

func TestNotifyUsesListenerSnapshot(t *testing.T) {
	reg := &NacosMcpRegistry{
		toolsDescription:  map[string]*registry.ToolDescription{},
		toolsRpcContext:   map[string]*registry.RpcContext{},
		currentServiceSet: map[string]bool{},
	}
	late := &countingListener{}
	reg.RegisterToolChangeEventListener(&registeringListener{reg: reg, target: late})

	reg.notifyToolChangeListeners()
	if got := late.calls(); got != 0 {
		t.Fatalf("late listener calls during first notification = %d, want 0", got)
	}

	reg.notifyToolChangeListeners()
	if got := late.calls(); got != 1 {
		t.Fatalf("late listener calls during second notification = %d, want 1", got)
	}
}
