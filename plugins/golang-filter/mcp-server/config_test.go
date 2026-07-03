package mcp_server

import (
	"errors"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	xds "github.com/cncf/xds/go/xds/type/v3"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type mockCommonCAPI struct{}

func (m *mockCommonCAPI) Log(api.LogType, string) {}

func (m *mockCommonCAPI) LogLevel() api.LogType {
	return api.Debug
}

type testMCPServerConfig struct {
	parseErr error
	newErr   error
	dsn      string
}

func (c *testMCPServerConfig) ParseConfig(config map[string]any) error {
	if dsn, ok := config["dsn"].(string); ok {
		c.dsn = dsn
	}
	return c.parseErr
}

func (c *testMCPServerConfig) NewServer(serverName string) (*common.MCPServer, error) {
	if c.newErr != nil {
		return nil, c.newErr
	}
	return common.NewMCPServer(serverName, "test"), nil
}

type pollutingMCPServerConfig struct {
	dsn string
}

func (c *pollutingMCPServerConfig) ParseConfig(config map[string]any) error {
	if dsn, ok := config["dsn"].(string); ok {
		c.dsn = dsn
	}
	if shouldFail, ok := config["fail"].(bool); ok && shouldFail {
		return errors.New("bad config")
	}
	return nil
}

func (c *pollutingMCPServerConfig) NewServer(serverName string) (*common.MCPServer, error) {
	if c.dsn != "" {
		return nil, errors.New("server config was polluted by previous entry")
	}
	return common.NewMCPServer(serverName, "test"), nil
}

func TestParserParseSkipsInvalidServerAndKeepsValidServers(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	common.GlobalRegistry.RegisterServer("test-valid", &testMCPServerConfig{})
	common.GlobalRegistry.RegisterServer("test-invalid", &testMCPServerConfig{parseErr: errors.New("bad config")})

	cfg := typedStructAny(t, map[string]any{
		"servers": []any{
			map[string]any{
				"name": "valid-server",
				"type": "test-valid",
				"path": "/valid",
			},
			map[string]any{
				"name": "broken-server",
				"type": "test-invalid",
				"path": "/broken",
			},
		},
	})

	parsed, err := (&Parser{}).Parse(cfg, nil)
	if err != nil {
		t.Fatalf("Parse should keep valid server configs when another server is invalid: %v", err)
	}
	conf := parsed.(*config)
	defer conf.Destroy()

	if len(conf.servers) != 1 {
		t.Fatalf("expected exactly one valid server to load, got %d", len(conf.servers))
	}
}

func TestParserParseSkipsInvalidServerEntry(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	common.GlobalRegistry.RegisterServer("test-valid-entry", &testMCPServerConfig{})

	cfg := typedStructAny(t, map[string]any{
		"servers": []any{
			"not-an-object",
			map[string]any{
				"name": "valid-server",
				"type": "test-valid-entry",
				"path": "/valid",
			},
		},
	})

	parsed, err := (&Parser{}).Parse(cfg, nil)
	if err != nil {
		t.Fatalf("Parse should skip malformed server entries and keep valid servers: %v", err)
	}
	conf := parsed.(*config)
	defer conf.Destroy()

	if len(conf.servers) != 1 {
		t.Fatalf("expected exactly one valid server to load, got %d", len(conf.servers))
	}
}

func TestParserParseUsesFreshServerConfigForEachEntry(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	common.GlobalRegistry.RegisterServer("test-fresh-entry", &pollutingMCPServerConfig{})

	cfg := typedStructAny(t, map[string]any{
		"servers": []any{
			map[string]any{
				"name": "broken-server",
				"type": "test-fresh-entry",
				"path": "/broken",
				"config": map[string]any{
					"dsn":  "dirty-state",
					"fail": true,
				},
			},
			map[string]any{
				"name":   "valid-server",
				"type":   "test-fresh-entry",
				"path":   "/valid",
				"config": map[string]any{},
			},
		},
	})

	parsed, err := (&Parser{}).Parse(cfg, nil)
	if err != nil {
		t.Fatalf("Parse should isolate failed server config state from later entries: %v", err)
	}
	conf := parsed.(*config)
	defer conf.Destroy()

	if len(conf.servers) != 1 {
		t.Fatalf("expected exactly one valid server to load, got %d", len(conf.servers))
	}
}

func typedStructAny(t *testing.T, cfg map[string]any) *anypb.Any {
	t.Helper()

	value, err := structpb.NewStruct(cfg)
	if err != nil {
		t.Fatalf("build typed struct value: %v", err)
	}
	typed, err := anypb.New(&xds.TypedStruct{Value: value})
	if err != nil {
		t.Fatalf("build typed struct any: %v", err)
	}
	return typed
}
