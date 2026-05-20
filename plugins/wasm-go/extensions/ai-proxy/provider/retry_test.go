package provider

import (
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
