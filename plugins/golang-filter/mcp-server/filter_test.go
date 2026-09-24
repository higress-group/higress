package mcp_server

import (
	"net/http"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/stretchr/testify/require"
)

type mockCAPI struct{}

func (m *mockCAPI) Log(api.LogType, string) {}

func (m *mockCAPI) LogLevel() api.LogType {
	return api.Debug
}

type testRequestHeaderMap struct {
	api.RequestHeaderMap
	values map[string][]string
}

func (h testRequestHeaderMap) Get(key string) (string, bool) {
	values := h.values[key]
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func (h testRequestHeaderMap) Range(f func(key, value string) bool) {
	for key, values := range h.values {
		for _, value := range values {
			if !f(key, value) {
				return
			}
		}
	}
}

type testBuffer struct {
	api.BufferInstance
	data []byte
}

func (b testBuffer) Bytes() []byte {
	return b.data
}

type testCallbacks struct {
	api.FilterCallbackHandler
	decoder *testDecoderCallbacks
}

func (c *testCallbacks) DecoderFilterCallbacks() api.DecoderFilterCallbacks {
	return c.decoder
}

type testDecoderCallbacks struct {
	api.DecoderFilterCallbacks
	statusCode int
	body       string
	headers    map[string][]string
}

func (c *testDecoderCallbacks) SendLocalReply(responseCode int, bodyText string, headers map[string][]string, grpcStatus int64, details string) {
	c.statusCode = responseCode
	c.body = bodyText
	c.headers = headers
}

func TestDecodeDataSendsBridgeSafeJSONLocalReplyHeaders(t *testing.T) {
	api.SetCommonCAPI(&mockCAPI{})

	decoder := &testDecoderCallbacks{}
	callbacks := &testCallbacks{decoder: decoder}
	sseServer := common.NewSSEServer(
		common.NewMCPServer("test", "1.0.0"),
		common.WithMessageEndpoint("/mcp"),
	)
	f := &filter{
		callbacks: callbacks,
		config: &config{servers: []*SSEServerWrapper{{
			BaseServer:   sseServer,
			HostMatchers: []common.HostMatcher{common.ParseHostPattern("*")},
		}}},
	}

	headerStatus := f.DecodeHeaders(testRequestHeaderMap{values: map[string][]string{
		":method":    {http.MethodPost},
		":scheme":    {"http"},
		":authority": {"example.com"},
		":path":      {"/mcp"},
	}}, false)
	require.Equal(t, api.StopAndBuffer, headerStatus)

	dataStatus := f.DecodeData(testBuffer{data: []byte(`not-json-at-all`)}, true)

	require.Equal(t, api.LocalReply, dataStatus)
	require.Equal(t, http.StatusOK, decoder.statusCode)
	require.Equal(t, "application/json", decoder.headers["Content-Type"][0])
	require.Contains(t, decoder.body, "Failed to parse message")
}
