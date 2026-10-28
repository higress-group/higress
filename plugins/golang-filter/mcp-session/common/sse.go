package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetSSEChannelName returns the Redis channel name for the given session ID
func GetSSEChannelName(sessionID string) string {
	return fmt.Sprintf("mcp-server-sse:%s", sessionID)
}

// SSEServer implements a Server-Sent Events (SSE) based MCP server.
// It provides real-time communication capabilities over HTTP using the SSE protocol.
type SSEServer struct {
	server          *MCPServer
	baseURL         string
	messageEndpoint string
	sseEndpoint     string
	sessions        sync.Map
	redisClient     *RedisClient // Redis client for pub/sub
	// registry pins live sessions to the Redis client that created their
	// subscription. It is usually left nil and then resolves to
	// GlobalSessionRegistry; tests inject an isolated instance instead.
	registry *SessionRegistry
}

func (s *SSEServer) GetMessageEndpoint() string {
	return s.messageEndpoint
}

func (s *SSEServer) GetSSEEndpoint() string {
	return s.sseEndpoint
}

func (s *SSEServer) GetServerName() string {
	return s.server.name
}

// sessionRegistry returns the registry that pins live sessions, falling back to
// the process wide one.
func (s *SSEServer) sessionRegistry() *SessionRegistry {
	if s.registry != nil {
		return s.registry
	}
	return GlobalSessionRegistry
}

// registerSessionForRedisBackend pins sessionID to the Redis client that backs
// this SSE connection and releases the pin once the connection is torn down.
//
// The SSE connection is the only subscriber on the session channel, so pinning
// it here is what keeps a POST carrying the same sessionId on the same backend
// after mcpServer.redis has been hot-updated: without the pin the POST would
// publish through the newly configured client while this subscription keeps
// reading from the client created at connection time, and the response would be
// lost. The pin is dropped when stopChan is closed by filter.OnDestroy; a nil
// stopChan means the caller never tears the connection down, in which case the
// pin stays for the lifetime of the process.
func (s *SSEServer) registerSessionForRedisBackend(sessionID string, stopChan chan struct{}) {
	if s.redisClient == nil {
		return
	}

	s.sessionRegistry().Register(sessionID, s.redisClient)
	if stopChan == nil {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				api.LogErrorf("Failed to release redis session pin: %v", r)
			}
		}()
		<-stopChan
		s.sessionRegistry().Unregister(sessionID)
	}()
}

// Option defines a function type for configuring SSEServer
type Option func(*SSEServer)

// WithBaseURL sets the base URL for the SSE server
func WithBaseURL(baseURL string) Option {
	return func(s *SSEServer) {
		s.baseURL = baseURL
	}
}

// WithMessageEndpoint sets the message endpoint path
func WithMessageEndpoint(endpoint string) Option {
	return func(s *SSEServer) {
		s.messageEndpoint = endpoint
	}
}

// WithSSEEndpoint sets the SSE endpoint path
func WithSSEEndpoint(endpoint string) Option {
	return func(s *SSEServer) {
		s.sseEndpoint = endpoint
	}
}

func WithRedisClient(redisClient *RedisClient) Option {
	return func(s *SSEServer) {
		s.redisClient = redisClient
	}
}

// WithSessionRegistry overrides the registry used to pin live sessions to their
// Redis backend. Production code relies on GlobalSessionRegistry; the option
// exists so tests can run against an isolated registry.
func WithSessionRegistry(registry *SessionRegistry) Option {
	return func(s *SSEServer) {
		s.registry = registry
	}
}

// NewSSEServer creates a new SSE server instance with the given MCP server and options.
func NewSSEServer(server *MCPServer, opts ...Option) *SSEServer {
	s := &SSEServer{
		server:          server,
		sseEndpoint:     "/sse",
		messageEndpoint: "/message",
	}

	// Apply all options
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// handleSSE handles incoming SSE connection requests.
// It sets up appropriate headers and creates a new session for the client.
func (s *SSEServer) HandleSSE(cb api.FilterCallbackHandler, stopChan chan struct{}) {
	sessionID := uuid.New().String()

	s.sessions.Store(sessionID, true)
	defer s.sessions.Delete(sessionID)

	// Bind the session to the Redis backend that is being subscribed below, so
	// later POSTs for the same session keep publishing on the same backend even
	// if mcpServer.redis is hot-updated while the connection is live.
	s.registerSessionForRedisBackend(sessionID, stopChan)

	channel := GetSSEChannelName(sessionID)
	u, err := url.Parse(s.baseURL + s.messageEndpoint)
	if err != nil {
		api.LogErrorf("Failed to parse base URL: %v", err)
	}

	q := u.Query()
	q.Set("sessionId", sessionID)
	u.RawQuery = q.Encode()
	messageEndpoint := u.String()

	// go func() {
	// 	for {
	// 		select {
	// 		case serverNotification := <-s.server.notifications:
	// 			// Only forward notifications meant for this session
	// 			if serverNotification.Context.SessionID == sessionID {
	// 				eventData, err := json.Marshal(serverNotification.Notification)
	// 				if err == nil {
	// 					select {
	// 					case session.eventQueue <- fmt.Sprintf("event: message\ndata: %s\n\n", eventData):
	// 						// Event queued successfully
	// 					case <-session.done:
	// 						return
	// 					}
	// 				}
	// 			}
	// 		case <-session.done:
	// 			return
	// 		case <-r.Context().Done():
	// 			return
	// 		}
	// 	}
	// }()

	err = s.redisClient.Subscribe(channel, stopChan, func(message string) {
		defer cb.EncoderFilterCallbacks().RecoverPanic()
		api.LogDebugf("SSE Send message: %s", message)
		cb.EncoderFilterCallbacks().InjectData([]byte(message))
	})
	if err != nil {
		api.LogErrorf("Failed to subscribe to Redis channel: %v", err)
	}

	// Send the initial endpoint event
	initialEvent := fmt.Sprintf("event: endpoint\ndata: %s\n\n", messageEndpoint)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				api.LogErrorf("Failed to send initial event: %v", r)
			}
		}()
		defer cb.EncoderFilterCallbacks().RecoverPanic()
		api.LogDebugf("SSE Send message: %s", initialEvent)
		cb.EncoderFilterCallbacks().InjectData([]byte(initialEvent))
	}()

	// Start health check handler
	go func() {
		defer func() {
			if r := recover(); r != nil {
				api.LogErrorf("Health check handler recovered from panic: %v", r)
			}
		}()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				// Send health check message
				currentTime := time.Now().Format(time.RFC3339)
				pingRequest := mcp.JSONRPCRequest{
					JSONRPC: mcp.JSONRPC_VERSION,
					ID:      currentTime,
					Request: mcp.Request{
						Method: "ping",
					},
				}
				pingData, _ := json.Marshal(pingRequest)
				healthCheckEvent := fmt.Sprintf("event: message\ndata: %s\n\n", pingData)
				if err := s.redisClient.Publish(channel, healthCheckEvent); err != nil {
					api.LogErrorf("Failed to send health check: %v", err)
				}
			}
		}
	}()
}

// handleMessage processes incoming JSON-RPC messages from clients and sends responses
// back through both the SSE connection and HTTP response.
func (s *SSEServer) HandleMessage(w http.ResponseWriter, r *http.Request, body json.RawMessage) int {
	if r.Method != http.MethodPost {
		s.writeJSONRPCError(w, nil, mcp.INVALID_REQUEST, fmt.Sprintf("Method %s not allowed", r.Method))
		return http.StatusBadRequest
	}

	sessionID := r.URL.Query().Get("sessionId")
	// support streamable http without sessionId
	// if sessionID == "" {
	// 	s.writeJSONRPCError(w, nil, mcp.INVALID_PARAMS, "Missing sessionId")
	// 	return
	// }

	// Set the client context in the server before handling the message
	ctx := s.server.WithContext(r.Context(), NotificationContext{
		ClientID:  sessionID,
		SessionID: sessionID,
	})

	// Extract Authorization header from HTTP request and add to context
	// This is used for Higress Console API authentication
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		ctx = WithAuthHeader(ctx, authHeader)
	}

	// Extract X-Istiod-Token header from HTTP request and add to context
	// This is used for Istiod debug API authentication
	if istiodToken := r.Header.Get("X-Istiod-Token"); istiodToken != "" {
		ctx = WithIstiodToken(ctx, istiodToken)
	}

	//TODO： check session id
	// _, ok := s.sessions.Load(sessionID)
	// if !ok {
	// 	s.writeJSONRPCError(w, nil, mcp.INVALID_PARAMS, "Invalid session ID")
	// 	return
	// }

	// Process message through MCPServer
	response := s.server.HandleMessage(ctx, body)
	var status int
	// Only send response if there is one (not for notifications)
	if response != nil {
		w.Header().Set("Content-Type", "application/json")
		if sessionID != "" {
			w.WriteHeader(http.StatusAccepted)
			status = http.StatusAccepted
		} else {
			// support streamable http
			w.WriteHeader(http.StatusOK)
			status = http.StatusOK
		}
		// Send HTTP response
		jsonData, err := json.Marshal(response)
		if err != nil {
			api.LogErrorf("Failed to marshal SSE Message response: %v", err)
			status = http.StatusInternalServerError
		}
		w.Write(jsonData)
	} else {
		// For notifications, just send 202 Accepted with no body
		w.WriteHeader(http.StatusAccepted)
		status = http.StatusAccepted
	}
	return status
}

// writeJSONRPCError writes a JSON-RPC error response with the given error details.
func (s *SSEServer) writeJSONRPCError(
	w http.ResponseWriter,
	id interface{},
	code int,
	message string,
) {
	response := createErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}

func (s *SSEServer) Close() {
	s.server.Close()
}
