package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var emptyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{})
	return data
}()

func TestGetRequestID(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("prefers request-id header over x_request_id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"request-id", "biz-req-123"},
				{"x-request-id", "envoy-req-123"},
			}))
			require.Equal(t, "biz-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x_request_id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId("x-req-123"))
			require.Equal(t, "x-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to selected request-id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "envoy-req-123"},
			}))
			require.NoError(t, host.SetProperty([]string{"wasm.requestId"}, []byte("biz-req-123")))
			require.Equal(t, "biz-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x-request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "header-req-123"},
			}))
			require.NoError(t, host.SetRequestId("-"))
			require.Equal(t, "header-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x_request_id when request-id is dash", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"request-id", "-"},
				{"x-request-id", "envoy-req-456"},
			}))
			require.Equal(t, "envoy-req-456", getRequestID())
			host.CompleteHttp()
		})

		t.Run("returns empty for dash request ID", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId("-"))
			require.Empty(t, getRequestID())
			host.CompleteHttp()
		})

		t.Run("trims whitespace", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId(" x-req-456 "))
			require.Equal(t, "x-req-456", getRequestID())
			host.CompleteHttp()
		})
	})
}

func TestNormalizeRequestID(t *testing.T) {
	require.Equal(t, "req-123", normalizeRequestID("req-123"))
	require.Equal(t, "req-456", normalizeRequestID(" req-456 "))
	require.Empty(t, normalizeRequestID(""))
	require.Empty(t, normalizeRequestID("   "))
	require.Empty(t, normalizeRequestID("-"))
}

func TestIsPendingAsyncResponse(t *testing.T) {
	pending := `{"accepted":true,"request_id":"0601f3d3-304b-423c-b89b-5a5b1f1f8cb2","retryable":false,"status":"processing_or_existing"}`
	quoted, err := json.Marshal(pending)
	require.NoError(t, err)
	doubleQuoted, err := json.Marshal(string(quoted))
	require.NoError(t, err)
	for _, body := range []string{pending, string(quoted), string(doubleQuoted),
		`{"data":` + pending + `}`, `{"data":` + string(doubleQuoted) + `}`} {
		require.True(t, isPendingAsyncResponse([]byte(body)), body)
	}
	for _, body := range []string{``, `invalid JSON`, `null`,
		`{"accepted":false,"status":"processing_or_existing"}`,
		`{"accepted":true,"status":"completed","data":[{"url":"image.png"}]}`,
		`{"data":[{"url":"image.png"}]}`,
		`{"choices":[{"message":{"content":` + string(quoted) + `}}]}`} {
		require.False(t, isPendingAsyncResponse([]byte(body)), body)
	}
}

func TestTokenReportResponseFiltering(t *testing.T) {
	pending := `{"accepted":true,"request_id":"0601f3d3-304b-423c-b89b-5a5b1f1f8cb2","retryable":false,"status":"processing_or_existing"}`
	quoted, err := json.Marshal(pending)
	require.NoError(t, err)
	doubleQuoted, err := json.Marshal(string(quoted))
	require.NoError(t, err)
	usage := `{"model":"test-model","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	config := json.RawMessage(`{"reportApiUrl":"http://report.example.com/usage"}`)
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			status        string
			currentStatus string
			body          string
			wantReports   int
			wantTokens    int64
		}{
			{name: "201 acknowledgement", status: "201", body: pending},
			{name: "201 image result", status: "201", body: `{"data":[{"url":"image.png"}]}`},
			{name: "201 usage", status: "201", body: usage},
			{name: "202 usage", status: "202", body: usage},
			{name: "500 usage", status: "500", body: usage},
			{name: "status changed after headers", status: "200", currentStatus: "201", body: usage},
			{name: "200 acknowledgement", status: "200", body: pending},
			{name: "200 wrapped acknowledgement", status: "200", body: `{"data":` + pending + `}`},
			{name: "200 escaped acknowledgement", status: "200", body: `{"data":` + string(doubleQuoted) + `}`},
			{name: "200 acknowledgement with usage", status: "200", body: `{"accepted":true,"status":"processing_or_existing","usage":{"total_tokens":5}}`},
			{name: "200 token usage", status: "200", body: usage, wantReports: 1, wantTokens: 5},
			{name: "200 completed image", status: "200", body: `{"data":[{"url":"image.png"}]}`, wantReports: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(config)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				require.NoError(t, host.SetProperty([]string{"wasm.providerType"}, []byte("openai")))
				require.NoError(t, host.SetProperty([]string{"wasm.requestModel"}, []byte("test-model")))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"}, {":path", "/v1/images/generations"},
					{":method", "POST"}, {"content-type", "application/json"},
					{OriginalAPIKey, "test-key"},
				}))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"model":"test-model"}`)))
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{
					{":status", tc.status}, {"content-type", "application/json"},
				}))
				if tc.currentStatus != "" {
					require.NoError(t, proxywasm.ReplaceHttpResponseHeader(":status", tc.currentStatus))
				}
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(tc.body)))
				require.Equal(t, tc.body, string(host.GetResponseBody()))
				reports := host.GetHttpCalloutAttributes()
				require.Len(t, reports, tc.wantReports)
				if tc.wantReports > 0 {
					var report TokenUsageReportRequest
					require.NoError(t, json.Unmarshal(reports[0].Body, &report))
					require.Equal(t, tc.wantTokens, report.TotalToken)
				}
				host.CompleteHttp()
			})
		}
	})
}
