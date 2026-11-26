package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSourceConfig(t *testing.T) {
	c, err := parseSourceConfig(gjson.Parse(`{}`))
	require.NoError(t, err)
	require.Equal(t, "x-wujie-source", c.headerName)
	require.Equal(t, "unknown", c.normalize("wujieai")) // no product hardcoded into the plugin
	c, err = parseSourceConfig(gjson.Parse(`{"headerName":"X-Product","allowedValues":["desktop"],"fallbackValue":"other"}`))
	require.NoError(t, err)
	require.Equal(t, "x-product", c.headerName)
	require.Equal(t, "desktop", c.normalize("desktop"))
	require.Equal(t, "other", c.normalize(""))
	require.Equal(t, "other", c.normalize("arbitrary-request-value"))
	for _, raw := range []string{`[]`, `{"headerName":":path"}`, `{"headerName":1}`, `{"allowedValues":"desktop"}`, `{"allowedValues":[false]}`, `{"allowedValues":["with.dots"]}`, `{"fallbackValue":""}`} {
		_, err := parseSourceConfig(gjson.Parse(raw))
		require.Error(t, err, raw)
	}
}

func TestReportSourceSnapshot(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name, sourceConfig, header, value, expected                      string
			stream, teamRoute, hasAdmission, mixedRoute, rejected, asyncTask bool
		}{
			{name: "async creation has its own settlement", teamRoute: true, hasAdmission: true, asyncTask: true},
			{name: "team usage SSE", teamRoute: true, hasAdmission: true, expected: "unknown", stream: true},
			{name: "team usage without admission", teamRoute: true, expected: "unknown"},
			{name: "mixed route with team admission", mixedRoute: true, hasAdmission: true, expected: "unknown"},
			{name: "mixed route with legacy usage", mixedRoute: true, expected: "unknown"},
			{name: "mixed route rejects team without personal billing", mixedRoute: true, rejected: true},
			{name: "old config remains valid", header: "x-wujie-source", value: "wujieai", expected: "unknown"},
			{name: "configured source JSON", sourceConfig: `,"source":{"allowedValues":["wujieai"]}`, header: "x-wujie-source", value: "wujieai", expected: "wujieai"},
			{name: "configured source SSE", sourceConfig: `,"source":{"allowedValues":["wujieai"]}`, header: "x-wujie-source", value: "wujieai", expected: "wujieai", stream: true},
			{name: "missing source", sourceConfig: `,"source":{"allowedValues":["wujieai"]}`, header: "x-wujie-source", expected: "unknown", stream: true},
			{name: "unknown source", sourceConfig: `,"source":{"allowedValues":["wujieai"]}`, header: "x-wujie-source", value: "other-product", expected: "unknown"},
			{name: "custom contract", sourceConfig: `,"source":{"headerName":"X-Product","allowedValues":["desktop"],"fallbackValue":"other"}`, header: "x-product", value: "desktop", expected: "desktop", stream: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := `{"reportApiUrl":"https://billing.test/model/usage/report","reportServiceName":"billing"` + tc.sourceConfig
				if tc.teamRoute {
					config += `,"requireTeamId":true`
				}
				if tc.teamRoute || tc.mixedRoute {
					config += `,"teamReportService":"onepiece-open-platform-internal.dns"`
				}
				hostConfig := json.RawMessage(config + `}`)
				host, status := test.NewTestHost(hostConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				require.NoError(t, host.SetProperty([]string{"wasm.providerType"}, []byte("openai")))
				require.NoError(t, host.SetProperty([]string{"wasm.requestModel"}, []byte("gpt-4")))
				id := host.InitializeHttpContext()
				if tc.asyncTask {
					require.NoError(t, host.SetProperty([]string{"wasm.teamBillingSettlement"}, []byte("task_create")))
				}
				if tc.hasAdmission {
					require.NoError(t, host.SetProperty([]string{"wasm.teamId"}, []byte("57")))
					require.NoError(t, host.SetProperty([]string{"wasm.teamPersonalOwnerUid"}, []byte("17")))
				}
				headers := [][2]string{{":authority", "model.test"}, {":method", "POST"}, {":path", "/v1/chat/completions"}, {"x-original-api-key", "test-user-key"}, {"request-id", "business-request"}, {"x-request-id", "envoy-request"}, {"wujie_task_id", "999"}}
				if tc.rejected {
					headers = append(headers, [2]string{teamUUIDHeader, "9943d4d9-d20b-49db-809d-88586f950fc7"})
				}
				original := append(append([][2]string{}, headers...), [2]string{tc.header, tc.value})
				require.Equal(t, types.ActionContinue, host.CallOnRequestHeaders(id, original, false))
				// Later routing/provider/header plugins rewrite path/host and source.
				// Reporting must retain the value captured by this decode callback.
				headers[0][1] = "upstream.test"
				headers[2][1] = "/rewritten/provider/path"
				host.SetHttpRequestHeaders(id, append(headers, [2]string{tc.header, "rewritten-source"}))
				require.Empty(t, host.GetCalloutAttributesFromContext(id))
				body := []byte(`{"model":"gpt-4","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
				contentType := "application/json"
				if tc.stream {
					contentType = "text/event-stream"
				}
				responseStatus := "200"
				if tc.rejected {
					responseStatus = "403"
				}
				host.CallOnResponseHeaders(id, [][2]string{{":status", responseStatus}, {"content-type", contentType}}, false)
				if tc.stream {
					host.CallOnResponseBody(id, append(append([]byte("data: "), body...), []byte("\n\n")...), false)
					require.Empty(t, host.GetCalloutAttributesFromContext(id), "must not report before original end-of-stream timing")
					host.CallOnResponseBody(id, []byte("data: [DONE]\n\n"), true)
				} else {
					host.CallOnResponseBody(id, body, true)
				}
				calls := host.GetCalloutAttributesFromContext(id)
				if tc.asyncTask || tc.rejected || (tc.teamRoute && !tc.hasAdmission) {
					require.Empty(t, calls, "missing team attribution must never fall back to personal billing")
					host.CompleteHttpContext(id)
					return
				}
				require.Len(t, calls, 1)
				var report map[string]any
				require.NoError(t, json.Unmarshal(calls[0].Body, &report))
				require.Equal(t, tc.expected, report["productSource"])
				require.Equal(t, "business-request", report["requestId"])
				require.Equal(t, "gpt-4", report["model"])
				require.Equal(t, "test-user-key", report["userKey"])
				require.Equal(t, "999", report["taskId"])
				require.Equal(t, float64(10), report["inputToken"])
				require.Equal(t, float64(5), report["outputToken"])
				require.Equal(t, float64(15), report["totalToken"])
				if tc.hasAdmission {
					require.Equal(t, "outbound|8080||onepiece-open-platform-internal.dns", calls[0].Upstream)
					require.Contains(t, calls[0].Headers, [2]string{":path", "/internal/model/usage/team-report"})
				} else {
					require.Equal(t, "outbound|443||billing.dns", calls[0].Upstream)
				}
				for _, field := range []string{"duration", "timestamp", "startTime", "endTime"} {
					require.IsType(t, float64(0), report[field])
					delete(report, field)
				}
				var usage map[string]any
				require.NoError(t, json.Unmarshal([]byte(report["tokenUsage"].(string)), &usage))
				require.Equal(t, float64(10), usage["input_token"])
				require.Equal(t, float64(5), usage["output_token"])
				require.Equal(t, float64(15), usage["total_token"])
				if tc.hasAdmission {
					require.Equal(t, float64(57), report["teamId"])
					require.Equal(t, float64(17), report["personalOwnerUid"])
					delete(report, "teamId")
					delete(report, "personalOwnerUid")
				}
				require.Len(t, report, 9, "legacy report shape is preserved")
				// The serialized asynchronous callout survives stream cleanup.
				host.CompleteHttpContext(id)
				require.Equal(t, tc.expected, gjson.GetBytes(calls[0].Body, "productSource").String())
			})
		}
	})
}

func TestPerCallSettlement(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name, status, body, priceModel string
			wantReport                     bool
		}{
			{"per-call image without tokens", "200", `{"data":[{"url":"https://example.test/output.png"}]}`, "gemini-3.1-flash-image-preview-times", true},
			{"upstream name is not the invoiced model", "200", `{"model":"gemini-3.1-flash-image-preview","usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11},"data":[{"b64_json":"AQ=="}]}`, "gemini-3.1-flash-image-preview-times", true},
			{"unsuccessful provider response", "503", `{"error":{"message":"unavailable"}}`, "gemini-3.1-flash-image-preview-times", false},
			{"zero search matches", "200", `{"results":[]}`, "deepseek-search", true},
			{"successful search", "200", `{"results":[{"url":"https://example.test/result"}]}`, "deepseek-search", true},
			{"invalid search response", "200", `{"error":"disabled"}`, "deepseek-search", false},
			{"empty result", "200", `{"data":[]}`, "gemini-3.1-flash-image-preview-times", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := json.RawMessage(`{"requireTeamId":true,"reportApiUrl":"https://billing.test/model/usage/report","reportServiceName":"billing","teamReportService":"onepiece-open-platform-internal.dns"}`)
				host, start := test.NewTestHost(cfg)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, start)
				id := host.InitializeHttpContext()
				priceModel := tc.priceModel
				require.NoError(t, host.SetProperty([]string{"wasm.teamId"}, []byte("57")))
				require.NoError(t, host.SetProperty([]string{"wasm.teamBillingPerCallModel"}, []byte(priceModel)))
				host.CallOnRequestHeaders(id, [][2]string{{":authority", "model.test"}, {":method", "POST"}, {":path", "/team/v1/images/generations"}, {"request-id", "wjat-fixture"}, {"x-original-api-key", "fixture-key"}}, false)
				host.CallOnResponseHeaders(id, [][2]string{{":status", tc.status}, {"content-type", "application/json"}}, false)
				require.NoError(t, proxywasm.RemoveHttpResponseHeader(":status"))
				host.CallOnResponseBody(id, []byte(tc.body), true)
				calls := host.GetCalloutAttributesFromContext(id)
				if tc.wantReport {
					require.Len(t, calls, 1)
					require.Contains(t, calls[0].Headers, [2]string{":path", "/internal/model/usage/team-report"})
					var report TokenUsageReportRequest
					require.NoError(t, json.Unmarshal(calls[0].Body, &report))
					require.Equal(t, priceModel, report.Model)
					require.Equal(t, "wjat-fixture", report.RequestId)
					require.EqualValues(t, 57, report.TeamID)
					require.Zero(t, report.PersonalOwnerUID)
				} else {
					require.Empty(t, calls)
				}
				host.CompleteHttpContext(id)
			})
		}
	})
}
