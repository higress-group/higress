package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	Endpoint, AdminURL, ExpectedInstructions, UpstreamInstructions, ServerName, Phase string
	ExpectedMethods                                                                   []string
}

type Report struct {
	Phase      string                `json:"phase"`
	Endpoint   string                `json:"endpoint"`
	SDKVersion string                `json:"sdkVersion"`
	SDKResult  *mcp.InitializeResult `json:"sdkResult"`
	Listed     *mcp.ListToolsResult  `json:"listResult"`
	Called     *mcp.CallToolResult   `json:"callResult"`
	Upstream   State                 `json:"upstream"`
}

type observedTransport struct {
	base   http.RoundTripper
	prefix string
	seq    atomic.Uint64
}

func (t *observedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("X-Request-ID", fmt.Sprintf("demo05-%s-%d", t.prefix, t.seq.Add(1)))
	return t.base.RoundTrip(copy)
}

func auxiliary(ctx context.Context, method, url string, value any) error {
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("auxiliary %s %s: HTTP %d", method, url, response.StatusCode)
	}
	if value != nil {
		return json.NewDecoder(response.Body).Decode(value)
	}
	return nil
}

// Run uses SDK-generated Connect/ListTools/CallTool requests. InitializeResult
// is the Go SDK's carrier for modern discovery; the observed method ledger
// separately proves that this carrier did not come from a legacy initialize.
func Run(ctx context.Context, options Options) (*Report, error) {
	admin := strings.TrimRight(options.AdminURL, "/")
	if err := auxiliary(ctx, http.MethodPost, admin+"/__reset", nil); err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "instructions-demo-client", Version: "1.0.0"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: options.Endpoint, DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: &observedTransport{base: http.DefaultTransport, prefix: options.Phase}},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("SDK Connect: %w", err)
	}
	defer session.Close()
	result := session.InitializeResult()
	if result == nil || result.ProtocolVersion != ProtocolVersion {
		return nil, fmt.Errorf("SDK must negotiate modern %s: %+v", ProtocolVersion, result)
	}
	if result.Instructions != options.ExpectedInstructions {
		return nil, fmt.Errorf("SDK instructions mismatch: got %q, want %q", result.Instructions, options.ExpectedInstructions)
	}
	if result.ServerInfo == nil || result.ServerInfo.Name != options.ServerName || result.ServerInfo.Version != "1.0.0" {
		return nil, fmt.Errorf("SDK serverInfo mismatch: %+v", result.ServerInfo)
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil || listed == nil || len(listed.Tools) != 1 || listed.Tools[0].Name != "get_weather" {
		return nil, fmt.Errorf("SDK ListTools: result=%+v error=%v", listed, err)
	}
	called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_weather", Arguments: map[string]any{"city": "杭州"}})
	if err != nil || called == nil || called.IsError || len(called.Content) != 1 {
		return nil, fmt.Errorf("SDK CallTool: result=%+v error=%v", called, err)
	}
	text, ok := called.Content[0].(*mcp.TextContent)
	if !ok || text.Text != WeatherText {
		return nil, fmt.Errorf("SDK CallTool content mismatch: %+v", called.Content)
	}
	if err := session.Close(); err != nil {
		return nil, err
	}
	var state State
	if err := auxiliary(ctx, http.MethodGet, admin+"/__state", &state); err != nil {
		return nil, err
	}
	if state.Instructions != options.UpstreamInstructions {
		return nil, fmt.Errorf("upstream configuration mismatch: got %q, want %q", state.Instructions, options.UpstreamInstructions)
	}
	methods := make([]string, 0, len(state.Events))
	seenIDs := make(map[string]bool)
	for _, event := range state.Events {
		methods = append(methods, event.Method)
		if event.ProtocolVersion != ProtocolVersion || event.MethodHeader != event.Method || !strings.HasPrefix(event.RequestID, "demo05-"+options.Phase+"-") || seenIDs[event.RequestID] {
			return nil, fmt.Errorf("upstream SDK method metadata mismatch: %+v", event)
		}
		seenIDs[event.RequestID] = true
	}
	if !reflect.DeepEqual(methods, options.ExpectedMethods) {
		return nil, fmt.Errorf("upstream methods mismatch: got %v, want %v (legacy initialize is forbidden)", methods, options.ExpectedMethods)
	}
	return &Report{Phase: options.Phase, Endpoint: options.Endpoint, SDKVersion: "v1.7.0", SDKResult: result,
		Listed: listed, Called: called, Upstream: state}, nil
}
