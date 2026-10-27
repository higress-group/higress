package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ProtocolVersion = "2026-07-28"
	UpstreamName    = "native-instructions-upstream"
	GatewayName     = "instructions-gateway"
	WeatherText     = "杭州：晴，25°C"
)

type Event struct {
	Method          string `json:"method"`
	ProtocolVersion string `json:"protocolVersion"`
	MethodHeader    string `json:"methodHeader"`
	RequestID       string `json:"requestId"`
}

type State struct {
	Instructions string  `json:"instructions"`
	Events       []Event `json:"events"`
}

type Fixture struct {
	mu           sync.Mutex
	instructions string
	events       []Event
}

// NewFixture delegates every MCP operation to the official SDK. The auxiliary
// endpoints observe/reset only this demo's method ledger, never MCP messages.
func NewFixture(instructions string) (*Fixture, http.Handler) {
	f := &Fixture{instructions: instructions, events: []Event{}}
	server := mcp.NewServer(&mcp.Implementation{Name: UpstreamName, Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: instructions, Capabilities: &mcp.ServerCapabilities{},
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			event := Event{Method: method}
			if extra := request.GetExtra(); extra != nil {
				event.ProtocolVersion = extra.Header.Get("MCP-Protocol-Version")
				event.MethodHeader = extra.Header.Get("Mcp-Method")
				event.RequestID = extra.Header.Get("X-Request-ID")
			}
			f.mu.Lock()
			f.events = append(f.events, event)
			f.mu.Unlock()
			return next(ctx, method, request)
		}
	})
	type weatherArgs struct {
		City string `json:"city" jsonschema:"City to query"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "get_weather", Description: "Return deterministic weather for the demo"},
		func(_ context.Context, _ *mcp.CallToolRequest, args weatherArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s：晴，25°C", args.City)}}}, nil, nil
		})
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /__state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.Snapshot())
	})
	mux.HandleFunc("POST /__reset", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.events = []Event{}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return f, mux
}

func (f *Fixture) Snapshot() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return State{Instructions: f.instructions, Events: append([]Event{}, f.events...)}
}
