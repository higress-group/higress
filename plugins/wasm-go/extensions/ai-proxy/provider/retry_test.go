package provider

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// mapCtx is a minimal wrapper.HttpContext for offline tests (no import cycle with test package).
type mapCtx struct {
	kv map[string]interface{}
}

func newMapCtx() *mapCtx {
	return &mapCtx{kv: make(map[string]interface{})}
}

func (m *mapCtx) SetContext(key string, value interface{})          { m.kv[key] = value }
func (m *mapCtx) GetContext(key string) interface{}                 { return m.kv[key] }
func (m *mapCtx) GetBoolContext(key string, def bool) bool          { return def }
func (m *mapCtx) GetStringContext(key, def string) string           { return def }
func (m *mapCtx) GetByteSliceContext(key string, def []byte) []byte { return def }
func (m *mapCtx) Scheme() string                                    { return "" }
func (m *mapCtx) Host() string                                      { return "" }
func (m *mapCtx) Path() string                                      { return "" }
func (m *mapCtx) Method() string                                    { return "" }
func (m *mapCtx) GetUserAttribute(key string) interface{}           { return nil }
func (m *mapCtx) SetUserAttribute(key string, value interface{})    {}
func (m *mapCtx) SetUserAttributeMap(kvmap map[string]interface{})  {}
func (m *mapCtx) GetUserAttributeMap() map[string]interface{}       { return nil }
func (m *mapCtx) WriteUserAttributeToLog() error                    { return nil }
func (m *mapCtx) WriteUserAttributeToLogWithKey(key string) error   { return nil }
func (m *mapCtx) WriteUserAttributeToTrace() error                  { return nil }
func (m *mapCtx) DontReadRequestBody()                              {}
func (m *mapCtx) DontReadResponseBody()                             {}
func (m *mapCtx) BufferRequestBody()                                {}
func (m *mapCtx) BufferResponseBody()                               {}
func (m *mapCtx) NeedPauseStreamingResponse()                       {}
func (m *mapCtx) PushBuffer(buffer []byte)                          {}
func (m *mapCtx) PopBuffer() []byte                                 { return nil }
func (m *mapCtx) BufferQueueSize() int                              { return 0 }
func (m *mapCtx) DisableReroute()                                   {}
func (m *mapCtx) SetRequestBodyBufferLimit(byteSize uint32)         {}
func (m *mapCtx) SetResponseBodyBufferLimit(byteSize uint32)        {}
func (m *mapCtx) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (m *mapCtx) GetExecutionPhase() iface.HTTPExecutionPhase { return 0 }
func (m *mapCtx) HasRequestBody() bool                        { return false }
func (m *mapCtx) HasResponseBody() bool                       { return false }
func (m *mapCtx) IsWebsocket() bool                           { return false }
func (m *mapCtx) IsBinaryRequestBody() bool                   { return false }
func (m *mapCtx) IsBinaryResponseBody() bool                  { return false }

var _ wrapper.HttpContext = (*mapCtx)(nil)

type stubProviderType struct{}

func (stubProviderType) GetProviderType() string { return providerTypeOpenAI }

// TransformRequestBody is a no-op so tests never reach defaultTransformRequestBody,
// which calls proxywasm host functions unavailable in the unit-test environment.
func (stubProviderType) TransformRequestBody(_ wrapper.HttpContext, _ ApiName, body []byte) ([]byte, error) {
	return body, nil
}

func TestRemoveApiTokenFromRetryList(t *testing.T) {
	t.Run("removes_token", func(t *testing.T) {
		got := removeApiTokenFromRetryList([]string{"a", "b", "c"}, "b")
		assert.Equal(t, []string{"a", "c"}, got)
	})
	t.Run("removes_all_when_single", func(t *testing.T) {
		got := removeApiTokenFromRetryList([]string{"x"}, "x")
		assert.Empty(t, got)
	})
	t.Run("no_match_unchanged", func(t *testing.T) {
		got := removeApiTokenFromRetryList([]string{"a", "b"}, "z")
		assert.Equal(t, []string{"a", "b"}, got)
	})
	t.Run("empty_input", func(t *testing.T) {
		got := removeApiTokenFromRetryList(nil, "a")
		assert.Empty(t, got)
	})
}

func TestGetRandomToken(t *testing.T) {
	assert.Equal(t, "", GetRandomToken(nil))
	assert.Equal(t, "", GetRandomToken([]string{}))
	assert.Equal(t, "only", GetRandomToken([]string{"only"}))
	tokens := []string{"a", "b", "c"}
	for i := 0; i < 20; i++ {
		got := GetRandomToken(tokens)
		assert.Contains(t, tokens, got)
	}
}

func TestRetryOnFailure_FromJson_defaults(t *testing.T) {
	var c ProviderConfig
	c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t"],"retryOnFailure":{"enabled":true}}`))
	require.True(t, c.IsRetryOnFailureEnabled())
	assert.Equal(t, int64(1), c.retryOnFailure.maxRetries)
	assert.Equal(t, int64(60*1000), c.retryOnFailure.retryTimeout)
	assert.Equal(t, []string{"4.*", "5.*"}, c.retryOnFailure.retryOnStatus)
}

// --- helpers for streaming tests ---

// stubStreamingBodyProvider implements StreamingResponseBodyHandler.
// It uppercases each chunk so tests can verify the handler was called.
type stubStreamingBodyProvider struct{ stubProviderType }

func (s stubStreamingBodyProvider) OnStreamingResponseBody(_ wrapper.HttpContext, _ ApiName, chunk []byte, _ bool) ([]byte, error) {
	return []byte(strings.ToUpper(string(chunk))), nil
}

// stubStreamingBodyProviderErr always returns an error from OnStreamingResponseBody.
type stubStreamingBodyProviderErr struct{ stubProviderType }

func (s stubStreamingBodyProviderErr) OnStreamingResponseBody(_ wrapper.HttpContext, _ ApiName, chunk []byte, _ bool) ([]byte, error) {
	return nil, errors.New("handler error")
}

// stubStreamingEventProvider implements StreamingEventHandler.
// It echoes each event back with a "HANDLED:" prefix in the data field.
type stubStreamingEventProvider struct{ stubProviderType }

func (s stubStreamingEventProvider) OnStreamingEvent(_ wrapper.HttpContext, _ ApiName, event StreamEvent) ([]StreamEvent, error) {
	out := event
	out.Data = "HANDLED:" + event.Data
	out.RawEvent = "data: HANDLED:" + event.Data + "\n\n"
	return []StreamEvent{out}, nil
}

// stubStreamingEventProviderErr always returns an error from OnStreamingEvent.
type stubStreamingEventProviderErr struct{ stubProviderType }

func (s stubStreamingEventProviderErr) OnStreamingEvent(_ wrapper.HttpContext, _ ApiName, event StreamEvent) ([]StreamEvent, error) {
	return nil, errors.New("event error")
}

// newOriginalConfig returns a ProviderConfig with protocol=original.
func newOriginalConfig() ProviderConfig {
	var c ProviderConfig
	c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t"],"protocol":"original"}`))
	return c
}

// newOpenAIConfig returns a plain openai ProviderConfig (non-original).
func newOpenAIConfig() ProviderConfig {
	var c ProviderConfig
	c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t"]}`))
	return c
}

// sseBody builds a minimal SSE body with a single data event.
func sseBody(data string) []byte {
	return []byte("data: " + data + "\n\n")
}

// --- transformStreamingRetryResponse ---

func TestTransformStreamingRetryResponse_Original(t *testing.T) {
	c := newOriginalConfig()
	ctx := newMapCtx()
	body := sseBody(`{"id":"1"}`)
	_, got := c.transformStreamingRetryResponse(ctx, stubProviderType{}, ApiNameChatCompletion, make(http.Header), body)
	assert.Equal(t, body, got, "original protocol must pass body through unchanged")
}

func TestTransformStreamingRetryResponse_StreamingBodyHandler(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	raw := "data: hello\n\n"
	_, got := c.transformStreamingRetryResponse(ctx, stubStreamingBodyProvider{}, ApiNameChatCompletion, make(http.Header), []byte(raw))
	// handler uppercases the chunk
	assert.Contains(t, string(got), "DATA: HELLO")
}

func TestTransformStreamingRetryResponse_StreamingBodyHandlerError_FallsBackToRaw(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	raw := "data: hello\n\n"
	_, got := c.transformStreamingRetryResponse(ctx, stubStreamingBodyProviderErr{}, ApiNameChatCompletion, make(http.Header), []byte(raw))
	// on error the raw event must be preserved
	assert.Contains(t, string(got), "data: hello")
}

func TestTransformStreamingRetryResponse_StreamingEventHandler(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	raw := "data: world\n\n"
	_, got := c.transformStreamingRetryResponse(ctx, stubStreamingEventProvider{}, ApiNameChatCompletion, make(http.Header), []byte(raw))
	assert.Contains(t, string(got), "HANDLED:world")
}

func TestTransformStreamingRetryResponse_StreamingEventHandlerError_FallsBackToRaw(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	raw := "data: world\n\n"
	_, got := c.transformStreamingRetryResponse(ctx, stubStreamingEventProviderErr{}, ApiNameChatCompletion, make(http.Header), []byte(raw))
	assert.Contains(t, string(got), "data: world")
}

func TestTransformStreamingRetryResponse_NoHandler_PassThrough(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	body := []byte("data: plain\n\n")
	_, got := c.transformStreamingRetryResponse(ctx, stubProviderType{}, ApiNameChatCompletion, make(http.Header), body)
	assert.Equal(t, body, got)
}

func TestTransformStreamingRetryResponse_SetsContentTypeEventStream(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	headers, _ := c.transformStreamingRetryResponse(ctx, stubProviderType{}, ApiNameChatCompletion, make(http.Header), []byte("data: x\n\n"))
	headerMap := make(map[string]string)
	for _, h := range headers {
		headerMap[strings.ToLower(h[0])] = h[1]
	}
	assert.Equal(t, "text/event-stream", headerMap["content-type"])
}

// --- transformResponseHeadersAndBody ---

// stubTransformBodyProvider implements TransformResponseBodyHandler.
// It appends "-transformed" to the body.
type stubTransformBodyProvider struct{ stubProviderType }

func (s stubTransformBodyProvider) TransformResponseBody(_ wrapper.HttpContext, _ ApiName, body []byte) ([]byte, error) {
	return append(body, []byte("-transformed")...), nil
}

func TestTransformResponseHeadersAndBody_NonOriginal_CallsHandler(t *testing.T) {
	c := newOpenAIConfig()
	ctx := newMapCtx()
	body := []byte(`{"choices":[]}`)
	_, got := c.transformResponseHeadersAndBody(ctx, stubTransformBodyProvider{}, ApiNameChatCompletion, make(http.Header), body)
	assert.Contains(t, string(got), "-transformed")
}

func TestTransformResponseHeadersAndBody_Original_SkipsHandler(t *testing.T) {
	c := newOriginalConfig()
	ctx := newMapCtx()
	body := []byte(`{"choices":[]}`)
	_, got := c.transformResponseHeadersAndBody(ctx, stubTransformBodyProvider{}, ApiNameChatCompletion, make(http.Header), body)
	// original protocol must NOT call the transform handler
	assert.Equal(t, body, got)
}

// --- ctxRetryIsStreaming propagation ---

// TestSendRetryRequest_PropagatesStreamingFlag verifies that ctxRetryIsStreaming is
// written from ctxKeyIsStreaming before the HTTP call is dispatched.
// The retryClient.Post call panics in the unit-test environment (no proxywasm host),
// so we recover from the panic and then inspect the context value.
func TestSendRetryRequest_PropagatesStreamingFlag(t *testing.T) {
	ctx := newMapCtx()
	ctx.SetContext(ctxKeyIsStreaming, true)
	ctx.SetContext(CtxKeyApiName, ApiNameChatCompletion)
	ctx.SetContext(CtxRequestHost, "api.example.com")
	ctx.SetContext(CtxRequestPath, "/v1/chat/completions")

	var c ProviderConfig
	c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t","t2"],"retryOnFailure":{"enabled":true}}`))
	c.initVariable()

	func() {
		defer func() { recover() }() // absorb the proxywasm host-call panic
		_ = c.sendRetryRequest(ctx, ApiNameChatCompletion, stubProviderType{}, createRetryClient(), "t", []string{"t", "t2"})
	}()

	isStreaming, _ := ctx.GetContext(ctxRetryIsStreaming).(bool)
	assert.True(t, isStreaming)
}

func TestSendRetryRequest_PropagatesNonStreamingFlag(t *testing.T) {
	ctx := newMapCtx()
	ctx.SetContext(ctxKeyIsStreaming, false)
	ctx.SetContext(CtxKeyApiName, ApiNameChatCompletion)
	ctx.SetContext(CtxRequestHost, "api.example.com")
	ctx.SetContext(CtxRequestPath, "/v1/chat/completions")

	var c ProviderConfig
	c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t","t2"],"retryOnFailure":{"enabled":true}}`))
	c.initVariable()

	func() {
		defer func() { recover() }()
		_ = c.sendRetryRequest(ctx, ApiNameChatCompletion, stubProviderType{}, createRetryClient(), "t", []string{"t", "t2"})
	}()

	isStreaming, _ := ctx.GetContext(ctxRetryIsStreaming).(bool)
	assert.False(t, isStreaming)
}

func TestOnRequestFailed_offlineBranches(t *testing.T) {
	t.Run("no_failover_no_retry_always_continue", func(t *testing.T) {
		var c ProviderConfig
		c.FromJson(gjson.Parse(`{"type":"openai","apiTokens":["t"]}`))
		ctx := newMapCtx()
		act := c.OnRequestFailed(stubProviderType{}, ctx, "t", []string{"t"}, "503")
		assert.Equal(t, types.ActionContinue, act)
	})

	t.Run("retry_enabled_single_token_returns_continue_before_post", func(t *testing.T) {
		var c ProviderConfig
		c.FromJson(gjson.Parse(`{
			"type":"openai",
			"apiTokens":["only"],
			"retryOnFailure":{"enabled":true,"retryOnStatus":["429","503"]}
		}`))
		ctx := newMapCtx()
		ctx.SetContext(CtxKeyApiName, ApiNameChatCompletion)
		act := c.OnRequestFailed(stubProviderType{}, ctx, "only", []string{"only"}, "503")
		assert.Equal(t, types.ActionContinue, act)
	})
}
