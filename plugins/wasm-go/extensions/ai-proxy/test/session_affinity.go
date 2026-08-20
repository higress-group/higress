package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var sessionAffinityFallbackConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"providers": []map[string]interface{}{
			{
				"id":        "primary",
				"type":      "openai",
				"apiTokens": []string{"sk-primary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o",
				},
			},
			{
				"id":        "secondary",
				"type":      "openai",
				"apiTokens": []string{"sk-secondary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o-mini",
				},
			},
		},
		"sessionAffinity": map[string]interface{}{
			"enabled":               true,
			"mode":                  "hash",
			"onProviderUnavailable": "fallbackWithoutUpdate",
			"unavailableStatus":     []string{"5.*"},
			"key": map[string]string{
				"source":   "body",
				"jsonPath": "$.callOptions.stickySessionId",
			},
		},
	})
	return data
}()

var sessionAffinityFallbackAndUpdateConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"providers": []map[string]interface{}{
			{
				"id":        "primary",
				"type":      "openai",
				"apiTokens": []string{"sk-primary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o",
				},
			},
			{
				"id":        "secondary",
				"type":      "openai",
				"apiTokens": []string{"sk-secondary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o-mini",
				},
			},
		},
		"sessionAffinity": map[string]interface{}{
			"enabled":               true,
			"mode":                  "persistent",
			"onProviderUnavailable": "fallbackAndUpdate",
			"unavailableStatus":     []string{"5.*"},
			"key": map[string]string{
				"source":   "body",
				"jsonPath": "$.callOptions.stickySessionId",
			},
		},
	})
	return data
}()

var sessionAffinityFailFastConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"providers": []map[string]interface{}{
			{
				"id":        "primary",
				"type":      "openai",
				"apiTokens": []string{"sk-primary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o",
				},
			},
			{
				"id":        "secondary",
				"type":      "openai",
				"apiTokens": []string{"sk-secondary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o-mini",
				},
			},
		},
		"sessionAffinity": map[string]interface{}{
			"enabled":               true,
			"mode":                  "hash",
			"onProviderUnavailable": "failFast",
			"unavailableStatus":     []string{"5.*"},
			"key": map[string]string{
				"source":   "body",
				"jsonPath": "$.callOptions.stickySessionId",
			},
		},
	})
	return data
}()

func sessionAffinitySelectionConfig(key map[string]interface{}) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"providers": []map[string]interface{}{
			{
				"id":        "primary",
				"type":      "openai",
				"apiTokens": []string{"sk-primary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o",
				},
			},
			{
				"id":        "secondary",
				"type":      "openai",
				"apiTokens": []string{"sk-secondary"},
				"modelMapping": map[string]string{
					"*": "gpt-4o-mini",
				},
			},
		},
		"sessionAffinity": map[string]interface{}{
			"enabled": true,
			"mode":    "hash",
			"key":     key,
		},
	})
	return data
}

func RunSessionAffinitySelectionTests(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("selects provider by header key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinitySelectionConfig(map[string]interface{}{
				"source": "header",
				"name":   "x-session-id",
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := callSessionAffinityRequestHeaders(host, [][2]string{
				{"x-session-id", "session-3840"},
			})
			require.Equal(t, types.HeaderStopIteration, action)
			requireSecondaryProviderSelected(t, host)
		})

		t.Run("selects provider by cookie key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinitySelectionConfig(map[string]interface{}{
				"source": "cookie",
				"name":   "llm_session",
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := callSessionAffinityRequestHeaders(host, [][2]string{
				{"cookie", "theme=dark; llm_session=session-3840"},
			})
			require.Equal(t, types.HeaderStopIteration, action)
			requireSecondaryProviderSelected(t, host)
		})

		t.Run("selects provider by metadata key", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinitySelectionConfig(map[string]interface{}{
				"source":       "metadata",
				"propertyPath": []string{"metadata", "sticky_session"},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			require.NoError(t, host.SetProperty([]string{"metadata", "sticky_session"}, []byte("session-3840")))

			action := callSessionAffinityRequestHeaders(host, nil)
			require.Equal(t, types.HeaderStopIteration, action)
			requireSecondaryProviderSelected(t, host)
		})
	})
}

func RunSessionAffinityFailFastTests(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("failFast returns 503 without fallback callout", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinityFailFastConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := callSessionAffinityRequestHeaders(host, nil)
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"callOptions":{"stickySessionId":"session-3840"}}`)
			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)
			requireRequestAuth(t, host, "Bearer sk-secondary")

			setUpstreamResponse(host)
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(503), localResponse.StatusCode)
			require.Equal(t, "selected provider unavailable", string(localResponse.Data))
		})
	})
}

func RunSessionAffinityPersistentFallbackTests(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("fallbackAndUpdate remaps subsequent session requests", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinityFallbackAndUpdateConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := callSessionAffinityRequestHeaders(host, nil)
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"callOptions":{"stickySessionId":"session-3840"}}`)
			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)
			requireRequestAuth(t, host, "Bearer sk-secondary")

			setUpstreamResponse(host)
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			fallbackAuth, ok := wasmtest.GetHeaderValue(callouts[0].Headers, "Authorization")
			require.True(t, ok, "fallback request should set Authorization")
			require.Equal(t, "Bearer sk-primary", fallbackAuth)

			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}, []byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"fallback ok"},"finish_reason":"stop"}]}`))
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(200), localResponse.StatusCode)

			host.CompleteHttp()
			host.InitHttp()

			action = callSessionAffinityRequestHeaders(host, nil)
			require.Equal(t, types.HeaderStopIteration, action)
			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)
			requireRequestAuth(t, host, "Bearer sk-primary")
			require.Contains(t, string(host.GetRequestBody()), `"model":"gpt-4o"`)
			require.NotContains(t, string(host.GetRequestBody()), `"model":"gpt-4o-mini"`)
		})
	})
}

func RunSessionAffinityFallbackTests(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("provider 5xx falls back to next selected provider", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(sessionAffinityFallbackConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"callOptions":{"stickySessionId":"session-3840"}}`)
			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			initialAuth, ok := wasmtest.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok, "initial provider should set Authorization")

			setUpstreamResponse(host)
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			fallbackAuth, ok := wasmtest.GetHeaderValue(callouts[0].Headers, "Authorization")
			require.True(t, ok, "fallback request should set Authorization")
			require.NotEqual(t, initialAuth, fallbackAuth, "fallback should use another provider")
			require.Contains(t, string(callouts[0].Body), `"model":"gpt-4o`)

			fallbackBody := []byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"fallback ok"},"finish_reason":"stop"}]}`)
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}, fallbackBody)
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(200), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "fallback ok")
		})
	})
}

func callSessionAffinityRequestHeaders(host wasmtest.TestHost, extraHeaders [][2]string) types.Action {
	headers := [][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"Content-Type", "application/json"},
	}
	headers = append(headers, extraHeaders...)
	return host.CallOnHttpRequestHeaders(headers)
}

func requireSecondaryProviderSelected(t *testing.T, host wasmtest.TestHost) {
	t.Helper()
	requireRequestAuth(t, host, "Bearer sk-secondary")

	action := host.CallOnHttpRequestBody([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	require.Equal(t, types.ActionContinue, action)
	require.Contains(t, string(host.GetRequestBody()), `"model":"gpt-4o-mini"`)
}

func requireRequestAuth(t *testing.T, host wasmtest.TestHost, want string) {
	t.Helper()
	auth, ok := wasmtest.GetHeaderValue(host.GetRequestHeaders(), "Authorization")
	require.True(t, ok, "selected provider should set Authorization")
	require.Equal(t, want, auth)
}
