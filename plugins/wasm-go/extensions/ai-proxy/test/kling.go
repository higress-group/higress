package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var klingOfficialConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                   "kling",
			"klingAccessKey":         "kling-ak-test",
			"klingSecretKey":         "kling-sk-test",
			"klingTokenRefreshAhead": 60,
			"modelMapping": map[string]string{
				"client-video": "kling-v2-1",
			},
		},
	})
	return data
}()

var klingGatewayConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":             "kling",
			"apiTokens":        []string{"gateway-token"},
			"providerDomain":   "api.302.ai",
			"providerBasePath": "/klingai",
			"modelMapping": map[string]string{
				"client-video": "kling-v2-1",
			},
		},
	})
	return data
}()

var klingGatewayCustomImageRetrieveConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":             "kling",
			"apiTokens":        []string{"gateway-token"},
			"providerDomain":   "api.302.ai",
			"providerBasePath": "/klingai",
			"modelMapping": map[string]string{
				"client-video": "kling-v2-1",
			},
			"capabilities": map[string]string{
				"openai/v1/videos":            "/gateway/text2video?mode=text",
				"kling/v1/image2video":        "/gateway/image2video?mode=image",
				"kling/v1/retrieveimagevideo": "/gateway/image-tasks/{video_id}?version=1",
			},
		},
	})
	return data
}()

var klingOriginalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":             "kling",
			"apiTokens":        []string{"gateway-token"},
			"providerDomain":   "api.302.ai",
			"providerBasePath": "/klingai",
			"protocol":         "original",
		},
	})
	return data
}()

func RunKlingParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("kling official config", func(t *testing.T) {
			host, status := test.NewTestHost(klingOfficialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("kling gateway config", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func RunKlingOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("official mode sets jwt bearer and default host", func(t *testing.T) {
			host, status := test.NewTestHost(klingOfficialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "api-singapore.klingai.com", hostValue)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth)
			require.True(t, strings.HasPrefix(authValue, "Bearer "))
			require.Len(t, strings.Split(strings.TrimPrefix(authValue, "Bearer "), "."), 3)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/v1/videos/text2video", pathValue)
		})

		t.Run("providerDomain and providerBasePath apply to gateway mode", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "api.302.ai", hostValue)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth)
			require.Equal(t, "Bearer gateway-token", authValue)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/text2video", pathValue)
		})

		t.Run("retrieve video query path is mapped under providerBasePath", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/task-123?with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/text2video/task-123?with_status=true", pathValue)
		})

		t.Run("prefixed image task query path is mapped to image endpoint", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/kling-i2v-task-123?with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/image2video/task-123?with_status=true", pathValue)
		})

		t.Run("prefixed image task query strips task type hint", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/kling-i2v-task-123?kling_task_type=image2video&with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/image2video/task-123?with_status=true", pathValue)
		})

		t.Run("raw image task query path uses explicit task type hint", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/raw-task-123?kling_task_type=image2video&with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/image2video/raw-task-123?with_status=true", pathValue)
		})

		t.Run("raw retrieve strips unknown task type hint before fallback mapping", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/raw-task-123?kling_task_type=bad&with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/text2video/raw-task-123?with_status=true", pathValue)
		})

		t.Run("image retrieve path uses configured image capability", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayCustomImageRetrieveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/kling-i2v-task-123?with_status=true"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/gateway/image-tasks/task-123?version=1&with_status=true", pathValue)
		})
	})
}

func RunKlingOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("text to video keeps text endpoint and maps model_name", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos?gateway_param=1"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			action := host.CallOnHttpRequestBody([]byte(`{"model":"client-video","prompt":"sunrise","duration":"5"}`))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/text2video?gateway_param=1", pathValue)

			processedBody := host.GetRequestBody()
			require.Equal(t, "kling-v2-1", gjson.GetBytes(processedBody, "model_name").String())
			require.False(t, gjson.GetBytes(processedBody, "model").Exists())
			require.Equal(t, "sunrise", gjson.GetBytes(processedBody, "prompt").String())
		})

		t.Run("image to video switches endpoint after body inspection", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos?gateway_param=1"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			action := host.CallOnHttpRequestBody([]byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/image2video?gateway_param=1", pathValue)

			processedBody := host.GetRequestBody()
			require.Equal(t, "kling-v2-1", gjson.GetBytes(processedBody, "model_name").String())
			require.Equal(t, "https://example.com/a.png", gjson.GetBytes(processedBody, "image").String())
		})

		t.Run("image to video uses configured image capability and merges query", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayCustomImageRetrieveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos?gateway_param=1"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			action := host.CallOnHttpRequestBody([]byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/gateway/image2video?mode=image&gateway_param=1", pathValue)

			processedBody := host.GetRequestBody()
			require.Equal(t, "kling-v2-1", gjson.GetBytes(processedBody, "model_name").String())
		})

		t.Run("original protocol does not expose request body handler", func(t *testing.T) {
			host, status := test.NewTestHost(klingOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/image2video"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"Content-Length", "64"},
			})
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/image2video", pathValue)
			contentLengthValue, hasContentLength := test.GetHeaderValue(requestHeaders, "Content-Length")
			require.True(t, hasContentLength)
			require.Equal(t, "64", contentLengthValue)
		})

		t.Run("original protocol recognizes native retrieve video path", func(t *testing.T) {
			host, status := test.NewTestHost(klingOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos/text2video/task-123"},
				{":method", "GET"},
			})
			require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/klingai/v1/videos/text2video/task-123", pathValue)
		})
	})
}

func RunKlingOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("image creation response prefixes task id", func(t *testing.T) {
			host, status := test.NewTestHost(klingGatewayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/videos"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			action := host.CallOnHttpRequestBody([]byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`))
			require.Equal(t, types.ActionContinue, action)

			require.NoError(t, host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream")))
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpResponseBody([]byte(`{"id":"root-task","data":{"task_id":"task-123"}}`))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetResponseBody()
			require.Equal(t, "root-task", gjson.GetBytes(processedBody, "id").String())
			require.Equal(t, "kling-i2v-task-123", gjson.GetBytes(processedBody, "data.task_id").String())
		})
	})
}
