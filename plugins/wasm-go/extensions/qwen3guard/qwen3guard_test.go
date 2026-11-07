package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmlog "github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type testHTTPContext struct {
	wrapper.HttpContext
	values map[string]interface{}
}

func newTestHTTPContext() *testHTTPContext {
	return &testHTTPContext{values: map[string]interface{}{}}
}

func (c *testHTTPContext) SetContext(key string, value interface{}) {
	c.values[key] = value
}

func (c *testHTTPContext) GetStringContext(key string, defaultValue string) string {
	value, ok := c.values[key].(string)
	if !ok {
		return defaultValue
	}
	return value
}

func (c *testHTTPContext) GetBoolContext(key string, defaultValue bool) bool {
	value, ok := c.values[key].(bool)
	if !ok {
		return defaultValue
	}
	return value
}

type testLog struct {
	wasmlog.Log
}

func (testLog) Warnf(string, ...interface{}) {}

func TestParseConfig(t *testing.T) {
	raw := `{
		"serviceSource": "k8s",
		"serviceName": "qwen3guard",
		"servicePort": 8000
	}`
	var config pluginConfig
	require.NoError(t, parseConfig(gjson.Parse(raw), &config, nil))

	require.NotNil(t, config.client)
	require.Equal(t, "default", config.namespace)
	require.Equal(t, "outbound|8000||qwen3guard.default.svc.cluster.local", config.client.ClusterName())
	require.Equal(t, defaultRequestPath, config.requestPath)
	require.Equal(t, defaultAPIKey, config.apiKey)
	require.Equal(t, defaultModel, config.model)
	require.Equal(t, uint32(defaultTimeoutMS), config.timeoutMS)
	require.True(t, config.checkRequest)
	require.True(t, config.checkResponse)
	require.Equal(t, defaultRequestContentJSONPath, config.requestContentJSONPath)
	require.Equal(t, defaultResponseContentJSONPath, config.responseContentJSONPath)
	require.Equal(t, defaultStreamingResponseContentJSONPath, config.streamingResponseContentJSONPath)
	require.Equal(t, defaultStreamBufferChars, config.streamBufferChars)
	require.Equal(t, riskLevelUnsafe, config.riskLevelBar)
	require.Equal(t, defaultDenyCode, config.denyCode)
	require.Equal(t, defaultDenyMessage, config.denyMessage)
	require.Equal(t, uint32(defaultMaxBodyBytes), config.maxBodyBytes)
}

func TestParseConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing service source",
			raw:  `{"serviceName":"qwen3guard","servicePort":8000}`,
		},
		{
			name: "missing service name",
			raw:  `{"serviceSource":"k8s","servicePort":8000}`,
		},
		{
			name: "invalid service port",
			raw:  `{"serviceSource":"k8s","serviceName":"qwen3guard","servicePort":0}`,
		},
		{
			name: "dns missing domain",
			raw:  `{"serviceSource":"dns","serviceName":"qwen3guard","servicePort":8000}`,
		},
		{
			name: "invalid risk level bar",
			raw:  `{"serviceSource":"k8s","serviceName":"qwen3guard","servicePort":8000,"riskLevelBar":"Safe"}`,
		},
		{
			name: "invalid timeout",
			raw:  `{"serviceSource":"k8s","serviceName":"qwen3guard","servicePort":8000,"timeout_ms":0}`,
		},
		{
			name: "invalid stream buffer",
			raw:  `{"serviceSource":"k8s","serviceName":"qwen3guard","servicePort":8000,"stream_buffer_chars":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config pluginConfig
			require.Error(t, parseConfig(gjson.Parse(tt.raw), &config, nil))
		})
	}
}

func TestBuildModerationBodies(t *testing.T) {
	requestBody, err := buildPromptModerationBody("guard-model", "hello")
	require.NoError(t, err)
	var request qwenChatRequest
	require.NoError(t, json.Unmarshal(requestBody, &request))
	require.Equal(t, "guard-model", request.Model)
	require.Equal(t, guardMaxTokens, request.MaxTokens)
	require.Equal(t, []qwenMessage{{Role: "user", Content: "hello"}}, request.Messages)

	responseBody, err := buildResponseModerationBody("guard-model", "hello", "world")
	require.NoError(t, err)
	var response qwenChatRequest
	require.NoError(t, json.Unmarshal(responseBody, &response))
	require.Equal(t, []qwenMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}, response.Messages)

	_, err = buildPromptModerationBody("guard-model", " ")
	require.ErrorIs(t, err, errEmptyModerated)
	_, err = buildResponseModerationBody("guard-model", "hello", " ")
	require.ErrorIs(t, err, errEmptyModerated)
}

func TestParseGuardResult(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		safety     string
		categories []string
		refusal    string
	}{
		{
			name:       "safe",
			content:    "Safety: Safe\nCategories: None",
			safety:     riskLevelSafe,
			categories: []string{"None"},
		},
		{
			name:       "unsafe with refusal",
			content:    "Safety: Unsafe\nCategories: Violence, Hate\nRefusal: No",
			safety:     riskLevelUnsafe,
			categories: []string{"Violence", "Hate"},
			refusal:    "No",
		},
		{
			name:       "controversial",
			content:    "Safety: Controversial\nCategories: Politics\nRefusal: Yes",
			safety:     riskLevelControversial,
			categories: []string{"Politics"},
			refusal:    "Yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseGuardResult(tt.content)
			require.NoError(t, err)
			require.Equal(t, tt.safety, result.Safety)
			require.Equal(t, tt.categories, result.Categories)
			require.Equal(t, tt.refusal, result.Refusal)
		})
	}

	_, err := parseGuardResult("Categories: None")
	require.ErrorIs(t, err, errMissingSafety)
}

func TestParseGuardHTTPResponse(t *testing.T) {
	raw := `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Cyber"}}]}`
	result, err := parseGuardHTTPResponse(200, []byte(raw))
	require.NoError(t, err)
	require.Equal(t, riskLevelUnsafe, result.Safety)
	require.Equal(t, []string{"Cyber"}, result.Categories)

	_, err = parseGuardHTTPResponse(500, []byte(raw))
	require.Error(t, err)
	_, err = parseGuardHTTPResponse(200, []byte(`{"choices":[]}`))
	require.ErrorIs(t, err, errEmptyChoices)
}

func TestJSONAndSSEHelpers(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":"ignored"},{"role":"user","content":"prompt"}],"choices":[{"delta":{"content":"hello"}}]}`)
	text, ok := extractJSONText(body, defaultRequestContentJSONPath)
	require.True(t, ok)
	require.Equal(t, "prompt", text)
	text, ok = extractJSONText(body, defaultStreamingResponseContentJSONPath)
	require.True(t, ok)
	require.Equal(t, "hello", text)

	complete, leftover := splitCompleteSSE("data: one\n\ndata: tw", false)
	require.Equal(t, "data: one\n\n", complete)
	require.Equal(t, "data: tw", leftover)

	payloads := extractSSEDataPayloads("event: message\r\ndata: {\"x\":1}\r\n\r\ndata: [DONE]\r\n\r\n")
	require.Equal(t, []string{`{"x":1}`, "[DONE]"}, payloads)
	require.True(t, isDonePayload(payloads[1]))
	require.Equal(t, 2, charCount("安全"))
}

func TestExceedsByteLimit(t *testing.T) {
	require.False(t, exceedsByteLimit(3, 2, 5))
	require.True(t, exceedsByteLimit(3, 3, 5))
	require.False(t, exceedsByteLimit(0, len("安全"), 6))
	require.True(t, exceedsByteLimit(0, len("安全"), 5))
}

func TestStreamingResponseExceedingMaxBodyBytesFailsOpen(t *testing.T) {
	t.Run("pending data", func(t *testing.T) {
		ctx := newTestHTTPContext()
		ctx.SetContext(ctxStreamPending, "old")
		ctx.SetContext(ctxStreamPartial, "part")
		config := pluginConfig{maxBodyBytes: 8}
		data := []byte("next")

		output := onHttpStreamingResponseBody(ctx, config, data, false, testLog{})

		require.Equal(t, []byte("oldpartnext"), output)
		require.True(t, ctx.GetBoolContext(ctxStreamBypass, false))
		require.Empty(t, ctx.GetStringContext(ctxStreamPending, ""))
		require.Empty(t, ctx.GetStringContext(ctxStreamPartial, ""))
	})

	t.Run("accumulated response text", func(t *testing.T) {
		ctx := newTestHTTPContext()
		ctx.SetContext(ctxResponseText, strings.Repeat("a", 95))
		config := pluginConfig{
			maxBodyBytes:                     100,
			streamingResponseContentJSONPath: "x",
		}
		data := []byte("data: {\"x\":\"1234567890\"}\n\n")

		output := onHttpStreamingResponseBody(ctx, config, data, false, testLog{})

		require.Equal(t, data, output)
		require.True(t, ctx.GetBoolContext(ctxStreamBypass, false))
		require.Empty(t, ctx.GetStringContext(ctxResponseText, ""))
		require.Empty(t, ctx.GetStringContext(ctxUncheckedText, ""))
	})

	t.Run("subsequent chunks pass through", func(t *testing.T) {
		ctx := newTestHTTPContext()
		ctx.SetContext(ctxStreamBypass, true)
		data := []byte("next")

		require.Equal(t, data, onHttpStreamingResponseBody(ctx, pluginConfig{}, data, false, testLog{}))
	})
}

func TestDenyBodies(t *testing.T) {
	chatBody := buildChatDenyBody("blocked")
	require.Equal(t, "chat.completion", gjson.GetBytes(chatBody, "object").String())
	require.Equal(t, "blocked", gjson.GetBytes(chatBody, "choices.0.message.content").String())
	require.Equal(t, "stop", gjson.GetBytes(chatBody, "choices.0.finish_reason").String())

	streamBody := string(buildStreamDenyBody("blocked"))
	payloads := extractSSEDataPayloads(streamBody)
	require.Len(t, payloads, 3)
	require.Equal(t, "blocked", gjson.Get(payloads[0], "choices.0.delta.content").String())
	require.Equal(t, "stop", gjson.Get(payloads[1], "choices.0.finish_reason").String())
	require.True(t, isDonePayload(payloads[2]))
}

// unsafeGuardResponse is the OpenAI-compatible classification reply a
// Qwen3Guard service returns for a prompt it wants blocked.
const unsafeGuardResponse = `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Violence\nRefusal: No"}}]}`

// requestOnlyDenyMessage is the custom denyMessage used by the intercept tests
// below. It is deliberately distinct from the plugin default so the assertions
// cannot pass by accident.
const requestOnlyDenyMessage = "blocked by qwen3guard"

// requestOnlyConfig only moderates the request side, which is the flow reported
// for Dify: the platform asks for a streaming completion and the plugin has to
// answer in the same shape.
var requestOnlyConfig = json.RawMessage(`{
	"serviceSource": "k8s",
	"serviceName": "qwen3guard",
	"servicePort": 8000,
	"checkRequest": true,
	"checkResponse": false,
	"denyMessage": "` + requestOnlyDenyMessage + `"
}`)

// responseOnlyConfig keeps request moderation off but still reads the request
// body, so it exercises the buffered (non-SSE) response intercept path.
var responseOnlyConfig = json.RawMessage(`{
	"serviceSource": "k8s",
	"serviceName": "qwen3guard",
	"servicePort": 8000,
	"checkRequest": false,
	"checkResponse": true,
	"denyMessage": "` + requestOnlyDenyMessage + `"
}`)

func headerValue(headers [][2]string, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return header[1]
		}
	}
	return ""
}

func TestIsStreamingRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "boolean true", body: `{"stream":true}`, want: true},
		{name: "string true", body: `{"stream":"true"}`, want: true},
		{name: "boolean false", body: `{"stream":false}`, want: false},
		{name: "missing flag", body: `{"messages":[]}`, want: false},
		{name: "invalid json", body: `not-json`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isStreamingRequest([]byte(tt.body)))
		})
	}
}

func TestBuildDenyResponsePicksMatchingFormat(t *testing.T) {
	streaming := buildDenyResponse("blocked", true)
	require.Equal(t, contentTypeSSE, streaming.contentType)
	payloads := extractSSEDataPayloads(string(streaming.body))
	require.Len(t, payloads, 3)
	require.Equal(t, "blocked", gjson.Get(payloads[0], "choices.0.delta.content").String())

	nonStreaming := buildDenyResponse("blocked", false)
	require.Equal(t, contentTypeJSON, nonStreaming.contentType)
	require.Equal(t, "chat.completion", gjson.GetBytes(nonStreaming.body, "object").String())
	require.Equal(t, "blocked", gjson.GetBytes(nonStreaming.body, "choices.0.message.content").String())
}

// TestRequestDenyMatchesRequestStreamMode is the regression test for the
// reported Dify behaviour: a streaming request that gets blocked used to be
// answered with a non-streaming chat.completion document, so the client's SSE
// reader produced an empty message. The intercept must now speak the same
// protocol the request asked for.
func TestRequestDenyMatchesRequestStreamMode(t *testing.T) {
	tests := []struct {
		name            string
		requestBody     string
		wantContentType string
	}{
		{
			name:            "streaming request gets an SSE intercept",
			requestBody:     `{"model":"qwen","stream":true,"messages":[{"role":"user","content":"unsafe prompt"}]}`,
			wantContentType: contentTypeSSE,
		},
		{
			name:            "non-streaming request keeps the JSON intercept",
			requestBody:     `{"model":"qwen","stream":false,"messages":[{"role":"user","content":"unsafe prompt"}]}`,
			wantContentType: contentTypeJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, status := test.NewTestHost(requestOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(tt.requestBody)))
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(unsafeGuardResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected the request intercept to send a local reply")
			require.Equal(t, uint32(defaultDenyCode), local.StatusCode)
			require.Equal(t, tt.wantContentType, headerValue(local.Headers, "content-type"))

			if tt.wantContentType == contentTypeSSE {
				require.True(t, strings.HasPrefix(string(local.Data), "data: "),
					"streaming intercept must be SSE framed so the client can parse it")
				payloads := extractSSEDataPayloads(string(local.Data))
				require.Len(t, payloads, 3)
				require.Equal(t, "chat.completion.chunk", gjson.Get(payloads[0], "object").String())
				require.Equal(t, requestOnlyDenyMessage, gjson.Get(payloads[0], "choices.0.delta.content").String())
				require.Equal(t, "stop", gjson.Get(payloads[1], "choices.0.finish_reason").String())
				require.True(t, isDonePayload(payloads[2]))
			} else {
				require.Equal(t, "chat.completion", gjson.GetBytes(local.Data, "object").String())
				require.Equal(t, requestOnlyDenyMessage, gjson.GetBytes(local.Data, "choices.0.message.content").String())
			}
		})
	}
}

// TestResponseDenyFollowsRequestStreamMode covers the buffered response
// intercept: even when the upstream produced a non-SSE body, the replacement
// reply has to match the streaming mode the caller negotiated.
func TestResponseDenyFollowsRequestStreamMode(t *testing.T) {
	host, status := test.NewTestHost(responseOnlyConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
	})
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(
		[]byte(`{"model":"qwen","stream":true,"messages":[{"role":"user","content":"hello"}]}`)))

	require.Equal(t, types.HeaderStopIteration, host.CallOnHttpResponseHeaders([][2]string{
		{":status", "200"},
		{"content-type", "application/json"},
	}))
	require.Equal(t, types.ActionPause, host.CallOnHttpResponseBody(
		[]byte(`{"choices":[{"message":{"content":"unsafe answer"}}]}`)))

	host.CallOnHttpCall([][2]string{
		{":status", "200"},
		{"content-type", "application/json"},
	}, []byte(unsafeGuardResponse))

	local := host.GetLocalResponse()
	require.NotNil(t, local, "expected the response intercept to send a local reply")
	require.Equal(t, contentTypeSSE, headerValue(local.Headers, "content-type"))
	payloads := extractSSEDataPayloads(string(local.Data))
	require.NotEmpty(t, payloads)
	require.Equal(t, requestOnlyDenyMessage, gjson.Get(payloads[0], "choices.0.delta.content").String())
}

func TestShouldBlockRisk(t *testing.T) {
	require.False(t, shouldBlockRisk(riskLevelControversial, riskLevelUnsafe))
	require.True(t, shouldBlockRisk(riskLevelUnsafe, riskLevelUnsafe))
	require.True(t, shouldBlockRisk(riskLevelControversial, riskLevelControversial))
	require.False(t, shouldBlockRisk("unknown", riskLevelUnsafe))
}
