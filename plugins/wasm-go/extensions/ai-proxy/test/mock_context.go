package test

import "github.com/higress-group/wasm-go/pkg/iface"

// MockHttpContext is a minimal mock for wrapper.HttpContext used in unit tests
// that call provider functions directly (e.g. streaming thinking promotion).
type MockHttpContext struct {
	contextMap map[string]interface{}
}

func NewMockHttpContext() *MockHttpContext {
	return &MockHttpContext{contextMap: make(map[string]interface{})}
}

func (m *MockHttpContext) SetContext(key string, value interface{})          { m.contextMap[key] = value }
func (m *MockHttpContext) GetContext(key string) interface{}                 { return m.contextMap[key] }
func (m *MockHttpContext) GetBoolContext(key string, def bool) bool          { return def }
func (m *MockHttpContext) GetStringContext(key, def string) string           { return def }
func (m *MockHttpContext) GetByteSliceContext(key string, def []byte) []byte { return def }
func (m *MockHttpContext) Scheme() string                                    { return "" }
func (m *MockHttpContext) Host() string                                      { return "" }
func (m *MockHttpContext) Path() string                                      { return "" }
func (m *MockHttpContext) Method() string                                    { return "" }
func (m *MockHttpContext) GetUserAttribute(key string) interface{}           { return nil }
func (m *MockHttpContext) SetUserAttribute(key string, value interface{})    {}
func (m *MockHttpContext) SetUserAttributeMap(kvmap map[string]interface{})  {}
func (m *MockHttpContext) GetUserAttributeMap() map[string]interface{}       { return nil }
func (m *MockHttpContext) WriteUserAttributeToLog() error                    { return nil }
func (m *MockHttpContext) WriteUserAttributeToLogWithKey(key string) error   { return nil }
func (m *MockHttpContext) WriteUserAttributeToTrace() error                  { return nil }
func (m *MockHttpContext) DontReadRequestBody()                              {}
func (m *MockHttpContext) DontReadResponseBody()                             {}
func (m *MockHttpContext) BufferRequestBody()                                {}
func (m *MockHttpContext) BufferResponseBody()                               {}
func (m *MockHttpContext) NeedPauseStreamingResponse()                       {}
func (m *MockHttpContext) PushBuffer(buffer []byte)                          {}
func (m *MockHttpContext) PopBuffer() []byte                                 { return nil }
func (m *MockHttpContext) BufferQueueSize() int                              { return 0 }
func (m *MockHttpContext) DisableReroute()                                   {}
func (m *MockHttpContext) SetRequestBodyBufferLimit(byteSize uint32)         {}
func (m *MockHttpContext) SetResponseBodyBufferLimit(byteSize uint32)        {}
func (m *MockHttpContext) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (m *MockHttpContext) GetExecutionPhase() iface.HTTPExecutionPhase { return 0 }
func (m *MockHttpContext) HasRequestBody() bool                        { return false }
func (m *MockHttpContext) HasResponseBody() bool                       { return false }
func (m *MockHttpContext) IsWebsocket() bool                           { return false }
func (m *MockHttpContext) IsBinaryRequestBody() bool                   { return false }
func (m *MockHttpContext) IsBinaryResponseBody() bool                  { return false }
