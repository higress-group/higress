package demo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	Endpoint, AdminURL, ExpectedInstructions, UpstreamInstructions, ServerName, Phase, RunID string
	ExpectedMethods                                                                          []string
}

type Report struct {
	RunID      string                `json:"runId"`
	Requests   []RequestObservation  `json:"httpRequests"`
	Phase      string                `json:"phase"`
	Endpoint   string                `json:"endpoint"`
	SDKVersion string                `json:"sdkVersion"`
	SDKResult  *mcp.InitializeResult `json:"sdkResult"`
	Listed     *mcp.ListToolsResult  `json:"listResult"`
	Called     *mcp.CallToolResult   `json:"callResult"`
	Upstream   State                 `json:"upstream"`
}

type RequestObservation struct {
	Method       string `json:"method"`
	HTTPMethod   string `json:"httpMethod"`
	Path         string `json:"path"`
	Marker       string `json:"marker"`
	UserAgent    string `json:"userAgent"`
	ResponseCode int    `json:"responseCode"`
}

type observedTransport struct {
	base     http.RoundTripper
	prefix   string
	seq      atomic.Uint64
	mu       sync.Mutex
	requests []RequestObservation
}

func (t *observedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	marker := fmt.Sprintf("demo05-%s-%d", t.prefix, t.seq.Add(1))
	userAgent := "higress-instructions-demo/" + marker
	// Envoy may replace external request IDs. Correlate independently using
	// ordinary headers, while retaining the actual assigned ID in SDK events.
	copy.Header.Set("X-Request-ID", marker)
	copy.Header.Set("X-Mcp-Demo-Request", marker)
	copy.Header.Set("User-Agent", userAgent)
	response, err := t.base.RoundTrip(copy)
	if err == nil {
		t.mu.Lock()
		t.requests = append(t.requests, RequestObservation{Method: copy.Header.Get("Mcp-Method"),
			HTTPMethod: copy.Method, Path: copy.URL.Path, Marker: marker, UserAgent: userAgent, ResponseCode: response.StatusCode})
		t.mu.Unlock()
	}
	return response, err
}

func (t *observedTransport) snapshot() []RequestObservation {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]RequestObservation{}, t.requests...)
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
	if options.RunID == "" {
		var value [16]byte
		if _, err := rand.Read(value[:]); err != nil {
			return nil, err
		}
		options.RunID = hex.EncodeToString(value[:])
	}
	transport := &observedTransport{base: http.DefaultTransport, prefix: options.RunID + "-" + options.Phase}
	admin := strings.TrimRight(options.AdminURL, "/")
	if err := auxiliary(ctx, http.MethodPost, admin+"/__reset", nil); err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "instructions-demo-client", Version: "1.0.0"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: options.Endpoint, DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: transport},
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
	requests := transport.snapshot()
	requestMethods := make([]string, 0, len(requests))
	byMarker := make(map[string]RequestObservation)
	for _, request := range requests {
		requestMethods = append(requestMethods, request.Method)
		if request.HTTPMethod != http.MethodPost || request.ResponseCode != http.StatusOK {
			return nil, fmt.Errorf("SDK HTTP exchange failed: %+v", request)
		}
		byMarker[request.Marker] = request
	}
	if !reflect.DeepEqual(requestMethods, []string{"server/discover", "tools/list", "tools/call"}) {
		return nil, fmt.Errorf("SDK HTTP method sequence changed: %v", requestMethods)
	}
	methods := make([]string, 0, len(state.Events))
	seenIDs := make(map[string]bool)
	seenMarkers := make(map[string]bool)
	for _, event := range state.Events {
		methods = append(methods, event.Method)
		request, observed := byMarker[event.Marker]
		if event.ProtocolVersion != ProtocolVersion || event.MethodHeader != event.Method || event.RequestID == "" || seenIDs[event.RequestID] ||
			!observed || request.Method != event.Method || event.UserAgent != request.UserAgent || seenMarkers[event.Marker] {
			return nil, fmt.Errorf("upstream SDK method metadata mismatch: %+v", event)
		}
		seenIDs[event.RequestID] = true
		seenMarkers[event.Marker] = true
	}
	if !reflect.DeepEqual(methods, options.ExpectedMethods) {
		return nil, fmt.Errorf("upstream methods mismatch: got %v, want %v (legacy initialize is forbidden)", methods, options.ExpectedMethods)
	}
	return &Report{RunID: options.RunID, Requests: requests, Phase: options.Phase, Endpoint: options.Endpoint, SDKVersion: "v1.7.0", SDKResult: result,
		Listed: listed, Called: called, Upstream: state}, nil
}
