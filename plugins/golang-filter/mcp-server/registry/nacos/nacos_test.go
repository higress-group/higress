package nacos

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/registry"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
)

// newTestRegistry builds a NacosMcpRegistry with initialized maps and no Nacos
// clients. The clients stay nil on purpose: the concurrency test only drives the
// map mutators/readers directly and passes non-nil config/instances so the Nacos
// SDK network paths in refreshToolsListForServiceWithContent are never taken.
func newTestRegistry() *NacosMcpRegistry {
	return &NacosMcpRegistry{
		toolsDescription:  map[string]*registry.ToolDescription{},
		toolsRpcContext:   map[string]*registry.RpcContext{},
		currentServiceSet: map[string]bool{},
	}
}

// toolsConfig serializes a minimal McpApplicationDescription for a single tool,
// deliberately without a credentialRef so GetCredential (which needs a client) is
// never invoked.
func toolsConfig(toolName string) string {
	desc := registry.McpApplicationDescription{
		Protocol: registry.PROTOCOL_HTTP,
		ToolsDescription: []*registry.ToolDescription{
			{
				Name:        toolName,
				Description: "test tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolsMeta: map[string]registry.ToolMeta{},
	}
	raw, _ := json.Marshal(desc)
	return string(raw)
}

// TestNacosMcpRegistryConcurrentAccess reproduces the concurrent map access
// between the Nacos refresh goroutines (config OnChange / naming SubscribeCallback
// / the background poll) that mutate toolsDescription, toolsRpcContext and
// currentServiceSet, and the Envoy request path that reads them via
// GetToolRpcContext and ListToolsDescription.
//
// Before the maps are guarded by a mutex this fails under `go test -race` with a
// DATA RACE (and can escalate to an unrecoverable `fatal error: concurrent map
// read and map write`). With the mutex in place it passes.
func TestNacosMcpRegistryConcurrentAccess(t *testing.T) {
	n := newTestRegistry()

	const (
		group      = "test-group"
		service    = "test-service"
		goroutines = 8
		iterations = 500
	)

	instances := []model.Instance{{Ip: "127.0.0.1", Port: 8080}}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch id % 4 {
				case 0, 1:
					// Simulate the Nacos config OnChange / naming callback and
					// poll goroutines mutating the shared maps.
					toolName := fmt.Sprintf("t-%d", j%3)
					cfg := toolsConfig(toolName)
					insts := instances
					n.refreshToolsListForServiceWithContent(group, service, &cfg, &insts)
				case 2:
					// Simulate the request path: tools/call -> GetToolRpcContext.
					n.GetToolRpcContext(makeToolName(group, service, fmt.Sprintf("t-%d", j%3)))
				case 3:
					// Simulate the request/reset path: tools/list ->
					// ListToolsDescription.
					_ = n.ListToolsDescription()
				}
			}
		}(i)
	}
	wg.Wait()
}
