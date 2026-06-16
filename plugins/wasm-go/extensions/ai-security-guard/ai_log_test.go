package main

import (
	"encoding/json"
	"testing"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type aiLogSnapshot struct {
	SafecheckRequests   []cfg.GuardrailSubmissionEvent `json:"safecheck_requests"`
	SafecheckRequestIDs []string                       `json:"safecheck_request_ids"`
	SafecheckRequestID  string                         `json:"safecheck_request_id"`
	SafecheckStatus     string                         `json:"safecheck_status"`
}

func readAILogSnapshot(t *testing.T, host test.TestHost) (aiLogSnapshot, string) {
	t.Helper()
	raw, err := host.GetProperty([]string{wrapper.AILogKey})
	require.NoError(t, err)
	decoded := wrapper.UnmarshalStr(`"` + string(raw) + `"`)
	require.NotEmpty(t, decoded)

	var snapshot aiLogSnapshot
	require.NoError(t, json.Unmarshal([]byte(decoded), &snapshot))
	return snapshot, decoded
}

func requireAILogArraySchema(t *testing.T, raw string) {
	t.Helper()
	require.True(t, gjson.Get(raw, cfg.SafecheckRequestsKey).IsArray(), "safecheck_requests must be a JSON array")
	require.True(t, gjson.Get(raw, cfg.SafecheckRequestIDsKey).IsArray(), "safecheck_request_ids must be a JSON array")
}

func requireSafecheckEvent(t *testing.T, event cfg.GuardrailSubmissionEvent, phase, modality, result, requestID string) {
	t.Helper()
	require.Equal(t, phase, event.Phase)
	require.Equal(t, modality, event.Modality)
	require.Equal(t, result, event.Result)
	require.Equal(t, requestID, event.RequestID)
}

func TestGuardrailAILogRequestAndResponseEventSchema(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("request pass emits one structured text event", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "Hello"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-structured-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			snapshot, raw := readAILogSnapshot(t, host)
			requireAILogArraySchema(t, raw)
			require.Len(t, snapshot.SafecheckRequests, 1)
			requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText, cfg.GuardrailResultPass, "req-structured-pass")
			require.Equal(t, []string{"req-structured-pass"}, snapshot.SafecheckRequestIDs)
			require.Equal(t, "req-structured-pass", snapshot.SafecheckRequestID)
			require.Equal(t, "request pass", snapshot.SafecheckStatus)
		})

		t.Run("response deny emits one structured text event", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			body := `{"choices": [{"message": {"role": "assistant", "content": "bad response content"}}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpResponseBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-structured-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			snapshot, raw := readAILogSnapshot(t, host)
			requireAILogArraySchema(t, raw)
			require.Len(t, snapshot.SafecheckRequests, 1)
			requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseResponse, cfg.GuardrailModalityText, cfg.GuardrailResultDeny, "req-structured-deny")
			require.Equal(t, []string{"req-structured-deny"}, snapshot.SafecheckRequestIDs)
			require.Equal(t, "req-structured-deny", snapshot.SafecheckRequestID)
			require.Equal(t, "response deny", snapshot.SafecheckStatus)
		})
	})
}

func TestGuardrailAILogStreamingPassFlushesBeforeEOS(t *testing.T) {
	streamingFlushConfig := func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"serviceName":               "security-service",
			"servicePort":               8080,
			"serviceHost":               "security.example.com",
			"accessKey":                 "test-ak",
			"secretKey":                 "test-sk",
			"checkRequest":              false,
			"checkResponse":             true,
			"action":                    "MultiModalGuard",
			"apiType":                   "text_generation",
			"contentModerationLevelBar": "high",
			"promptAttackLevelBar":      "high",
			"sensitiveDataLevelBar":     "S3",
			"timeout":                   2000,
			"bufferLimit":               1,
		})
		return data
	}()

	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(streamingFlushConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/event-stream"},
		})

		chunk := []byte("data:{\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		host.CallOnHttpStreamingResponseBody(chunk, false)

		securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-stream-pass", "Data": {"RiskLevel": "low"}}`
		host.CallOnHttpCall([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		}, []byte(securityResponse))

		snapshot, raw := readAILogSnapshot(t, host)
		requireAILogArraySchema(t, raw)
		require.Len(t, snapshot.SafecheckRequests, 1)
		requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseResponse, cfg.GuardrailModalityText, cfg.GuardrailResultPass, "req-stream-pass")
		require.False(t, gjson.Get(raw, "safecheck_status").Exists(), "event-level flush should not wait for a terminal safecheck_status")
	})
}

func TestGuardrailAILogErrorFlushAndOrderingForImageSubmissions(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		runCase := func(t *testing.T, firstHeaders [][2]string, firstResponse, firstRequestID string) {
			host, status := test.NewTestHost(multiModalGuardImageQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
			})

			body := `{"input": {"images": ["https://example.com/a.png", "https://example.com/b.png"]}}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			host.CallOnHttpCall(firstHeaders, []byte(firstResponse))
			snapshot, raw := readAILogSnapshot(t, host)
			requireAILogArraySchema(t, raw)
			require.Len(t, snapshot.SafecheckRequests, 1)
			requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityImage, cfg.GuardrailResultError, firstRequestID)
			require.Equal(t, []string{firstRequestID}, snapshot.SafecheckRequestIDs)

			secondResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-image-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(secondResponse))

			snapshot, raw = readAILogSnapshot(t, host)
			requireAILogArraySchema(t, raw)
			require.Len(t, snapshot.SafecheckRequests, 2)
			requireSafecheckEvent(t, snapshot.SafecheckRequests[1], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityImage, cfg.GuardrailResultPass, "req-image-pass")
			require.Equal(t, []string{firstRequestID, "req-image-pass"}, snapshot.SafecheckRequestIDs)
			require.Equal(t, "req-image-pass", snapshot.SafecheckRequestID)
		}

		t.Run("non-200 HTTP response flushes error before next image submission", func(t *testing.T) {
			runCase(t, [][2]string{
				{":status", "502"},
				{"content-type", "application/json"},
			}, `{"RequestId": "req-http-error"}`, "req-http-error")
		})

		t.Run("business failure flushes error before next image submission", func(t *testing.T) {
			runCase(t, [][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, `{"Code": 500, "Message": "Failed", "RequestId": "req-business-error"}`, "req-business-error")
		})
	})
}

func TestGuardrailAILogMalformedRequestIDsAreIgnored(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name           string
			response       string
			expectedResult string
		}{
			{
				name:           "missing",
				response:       `{"Code": 200, "Message": "Success", "Data": {"RiskLevel": "low"}}`,
				expectedResult: cfg.GuardrailResultPass,
			},
			{
				name:           "empty",
				response:       `{"Code": 200, "Message": "Success", "RequestId": "", "Data": {"RiskLevel": "low"}}`,
				expectedResult: cfg.GuardrailResultPass,
			},
			{
				name:           "whitespace",
				response:       `{"Code": 200, "Message": "Success", "RequestId": "   ", "Data": {"RiskLevel": "low"}}`,
				expectedResult: cfg.GuardrailResultPass,
			},
			{
				name:           "non-string",
				response:       `{"Code": 200, "Message": "Success", "RequestId": 123, "Data": {"RiskLevel": "low"}}`,
				expectedResult: cfg.GuardrailResultError,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(multiModalGuardTextConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/v1/chat/completions"},
					{":method", "POST"},
				})

				body := `{"messages": [{"role": "user", "content": "Hello"}]}`
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

				host.CallOnHttpCall([][2]string{
					{":status", "200"},
					{"content-type", "application/json"},
				}, []byte(tc.response))

				snapshot, raw := readAILogSnapshot(t, host)
				requireAILogArraySchema(t, raw)
				require.Len(t, snapshot.SafecheckRequests, 1)
				requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText, tc.expectedResult, "")
				require.Empty(t, snapshot.SafecheckRequestIDs)
				require.False(t, gjson.Get(raw, cfg.SafecheckRequestIDKey).Exists())
				require.False(t, gjson.Get(raw, cfg.SafecheckRequestsKey+".0.requestId").Exists())
			})
		}
	})
}

func TestGuardrailAILogMaskFallbackRecordsDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(maskConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})

		body := `{"messages": [{"role": "user", "content": "敏感内容"}]}`
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

		securityResponse := `{
			"Code": 200, "Message": "Success", "RequestId": "req-mask-fallback",
			"Data": {
				"RiskLevel": "none",
				"Detail": [{
					"Suggestion": "mask", "Type": "sensitiveData", "Level": "S3",
					"Result": [{"Label": "phone", "Confidence": 99.0,
						"Ext": {"Desensitization": ""}}]
				}]
			}
		}`
		host.CallOnHttpCall([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		}, []byte(securityResponse))

		snapshot, raw := readAILogSnapshot(t, host)
		requireAILogArraySchema(t, raw)
		require.Len(t, snapshot.SafecheckRequests, 1)
		requireSafecheckEvent(t, snapshot.SafecheckRequests[0], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText, cfg.GuardrailResultDeny, "req-mask-fallback")
		require.Equal(t, []string{"req-mask-fallback"}, snapshot.SafecheckRequestIDs)
	})
}

func TestGuardrailAILogDispatchFailureEmitsErrorEvent(t *testing.T) {
	ctx := newStubHTTPContext()
	eventIndex := cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText)
	cfg.CompleteGuardrailSubmissionEventWithRequestID(ctx, eventIndex, "", cfg.GuardrailResultError)
	cfg.WriteGuardrailLog(ctx)

	events, ok := ctx.GetUserAttribute(cfg.SafecheckRequestsKey).([]cfg.GuardrailSubmissionEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	requireSafecheckEvent(t, events[0], cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText, cfg.GuardrailResultError, "")

	requestIDs, ok := ctx.GetUserAttribute(cfg.SafecheckRequestIDsKey).([]string)
	require.True(t, ok)
	require.Empty(t, requestIDs)
	require.Nil(t, ctx.GetUserAttribute(cfg.SafecheckRequestIDKey))
	require.Equal(t, []string{wrapper.AILogKey}, ctx.writes)
}

type stubHTTPContext struct {
	userContext    map[string]interface{}
	userAttribute  map[string]interface{}
	bufferQueue    [][]byte
	writes         []string
	routeCallError error
}

func newStubHTTPContext() *stubHTTPContext {
	return &stubHTTPContext{
		userContext:   map[string]interface{}{},
		userAttribute: map[string]interface{}{},
	}
}

func (ctx *stubHTTPContext) Scheme() string { return "" }
func (ctx *stubHTTPContext) Host() string   { return "" }
func (ctx *stubHTTPContext) Path() string   { return "" }
func (ctx *stubHTTPContext) Method() string { return "" }

func (ctx *stubHTTPContext) SetContext(key string, value interface{}) {
	ctx.userContext[key] = value
}

func (ctx *stubHTTPContext) GetContext(key string) interface{} {
	return ctx.userContext[key]
}

func (ctx *stubHTTPContext) GetBoolContext(key string, defaultValue bool) bool {
	if value, ok := ctx.userContext[key].(bool); ok {
		return value
	}
	return defaultValue
}

func (ctx *stubHTTPContext) GetStringContext(key, defaultValue string) string {
	if value, ok := ctx.userContext[key].(string); ok {
		return value
	}
	return defaultValue
}

func (ctx *stubHTTPContext) GetByteSliceContext(key string, defaultValue []byte) []byte {
	if value, ok := ctx.userContext[key].([]byte); ok {
		return value
	}
	return defaultValue
}

func (ctx *stubHTTPContext) GetUserAttribute(key string) interface{} {
	return ctx.userAttribute[key]
}

func (ctx *stubHTTPContext) SetUserAttribute(key string, value interface{}) {
	ctx.userAttribute[key] = value
}

func (ctx *stubHTTPContext) SetUserAttributeMap(kvmap map[string]interface{}) {
	ctx.userAttribute = kvmap
}

func (ctx *stubHTTPContext) GetUserAttributeMap() map[string]interface{} {
	return ctx.userAttribute
}

func (ctx *stubHTTPContext) WriteUserAttributeToLog() error {
	return ctx.WriteUserAttributeToLogWithKey(wrapper.CustomLogKey)
}

func (ctx *stubHTTPContext) WriteUserAttributeToLogWithKey(key string) error {
	ctx.writes = append(ctx.writes, key)
	return nil
}

func (ctx *stubHTTPContext) WriteUserAttributeToTrace() error { return nil }
func (ctx *stubHTTPContext) DontReadRequestBody()             {}
func (ctx *stubHTTPContext) DontReadResponseBody()            {}
func (ctx *stubHTTPContext) BufferRequestBody()               {}
func (ctx *stubHTTPContext) BufferResponseBody()              {}
func (ctx *stubHTTPContext) NeedPauseStreamingResponse()      {}

func (ctx *stubHTTPContext) PushBuffer(buffer []byte) {
	ctx.bufferQueue = append(ctx.bufferQueue, buffer)
}

func (ctx *stubHTTPContext) PopBuffer() []byte {
	if len(ctx.bufferQueue) == 0 {
		return nil
	}
	buffer := ctx.bufferQueue[0]
	ctx.bufferQueue = ctx.bufferQueue[1:]
	return buffer
}

func (ctx *stubHTTPContext) BufferQueueSize() int { return len(ctx.bufferQueue) }
func (ctx *stubHTTPContext) DisableReroute()      {}
func (ctx *stubHTTPContext) SetRequestBodyBufferLimit(uint32) {
}
func (ctx *stubHTTPContext) SetResponseBodyBufferLimit(uint32) {
}

func (ctx *stubHTTPContext) RouteCall(string, string, [][2]string, []byte, iface.RouteResponseCallback) error {
	return ctx.routeCallError
}

func (ctx *stubHTTPContext) GetExecutionPhase() iface.HTTPExecutionPhase {
	return iface.DecodeData
}

func (ctx *stubHTTPContext) HasRequestBody() bool      { return true }
func (ctx *stubHTTPContext) HasResponseBody() bool     { return true }
func (ctx *stubHTTPContext) IsWebsocket() bool         { return false }
func (ctx *stubHTTPContext) IsBinaryRequestBody() bool { return false }
func (ctx *stubHTTPContext) IsBinaryResponseBody() bool {
	return false
}
