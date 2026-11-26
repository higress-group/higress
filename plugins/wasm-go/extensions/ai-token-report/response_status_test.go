package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestResponseStatusBillingBoundary(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, responseStatus := range []string{"200", "", "400", "401", "402", "403", "429", "502", "503"} {
			for _, stream := range []bool{false, true} {
				for _, released := range []bool{false, true} {
					t.Run(fmt.Sprintf("status=%s/stream=%v/released=%v", responseStatus, stream, released), func(t *testing.T) {
						host, status := test.NewTestHost(json.RawMessage(`{"reportApiUrl":"https://billing.test/report","reportServiceName":"billing"}`))
						defer host.Reset()
						require.Equal(t, types.OnPluginStartStatusOK, status)
						require.NoError(t, host.SetProperty([]string{"wasm.providerType"}, []byte("openai")))
						require.NoError(t, host.SetProperty([]string{"wasm.requestModel"}, []byte("test-model")))
						id := host.InitializeHttpContext()
						host.CallOnRequestHeaders(id, [][2]string{{":method", "POST"}, {":path", "/v1/chat/completions"}, {":authority", "model.test"}, {"x-original-api-key", "fixture-key"}, {"request-id", "fixture-request"}}, false)
						contentType := "application/json"
						if stream {
							contentType = "text/event-stream"
						}
						headers := [][2]string{{"content-type", contentType}}
						if responseStatus != "" {
							headers = append(headers, [2]string{":status", responseStatus})
						}
						host.CallOnResponseHeaders(id, headers, false)
						if released && responseStatus != "" {
							require.NoError(t, proxywasm.RemoveHttpResponseHeader(":status"))
						}
						body := []byte(`{"model":"test-model","usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`)
						if stream {
							host.CallOnResponseBody(id, append(append([]byte("data: "), body...), []byte("\n\n")...), false)
							require.Empty(t, host.GetCalloutAttributesFromContext(id))
							host.CallOnResponseBody(id, []byte("data: [DONE]\n\n"), true)
						} else {
							host.CallOnResponseBody(id, body, true)
						}
						calls := host.GetCalloutAttributesFromContext(id)
						if responseStatus == "200" {
							require.Len(t, calls, 1)
							var report TokenUsageReportRequest
							require.NoError(t, json.Unmarshal(calls[0].Body, &report))
							require.Equal(t, int64(14), report.TotalToken)
						} else {
							require.Empty(t, calls, "失败请求不得触发计费上报")
						}
						host.CompleteHttpContext(id)
					})
				}
			}
		}
	})
}

func TestSuccessfulZeroTokenImageStillReported(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(json.RawMessage(`{"reportApiUrl":"https://billing.test/report","reportServiceName":"billing"}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetProperty([]string{"wasm.providerType"}, []byte("openai")))
		require.NoError(t, host.SetProperty([]string{"wasm.requestModel"}, []byte("image-model")))
		id := host.InitializeHttpContext()
		host.CallOnRequestHeaders(id, [][2]string{{":method", "POST"}, {":path", "/v1/images/generations"}, {":authority", "model.test"}, {"x-original-api-key", "fixture-key"}}, false)
		host.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "application/json"}}, false)
		require.NoError(t, proxywasm.RemoveHttpResponseHeader(":status"))
		host.CallOnResponseBody(id, []byte(`{"data":[{"url":"image.png"}]}`), true)
		calls := host.GetCalloutAttributesFromContext(id)
		require.Len(t, calls, 1)
		var report TokenUsageReportRequest
		require.NoError(t, json.Unmarshal(calls[0].Body, &report))
		require.Zero(t, report.TotalToken)
		host.CompleteHttpContext(id)
	})
}

func TestRequestBillingPolicy(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		const team = `,"requireTeamId":true,"teamReportService":"team.internal.dns"`
		const previous = `,"teamUnlistedPathConfig":{"reportApiUrl":"https://billing.test/previous","reportServiceName":"old"}`
		for _, tc := range []struct {
			name, extra, version, id, wantPath                              string
			unlisted, lateHeader, personalOwner, missingProvider, forwarded bool
			status                                                          uint32
		}{
			{name: "legacy reports once", wantPath: "/report"},
			{name: "legacy rejects unsupported versioned billing before upstream", version: "2", status: 503},
			{name: "invalid version", version: "unknown", status: 400},
			{name: "later header changes preserve successful legacy usage", lateHeader: true, wantPath: "/report"},
			{name: "missing trusted admission", extra: team, version: "2", status: 503},
			{name: "non-numeric admission", extra: team, version: "2", id: "invalid", status: 503},
			{name: "personal wallet admission", personalOwner: true, extra: team, version: "2", id: "42", status: 503},
			{name: "forwarding is settled by execution", version: "2", id: "42", missingProvider: true, forwarded: true},
			{name: "missing usage provider", extra: team, version: "2", id: "42", missingProvider: true, status: 503},
			{name: "valid admission", extra: team, version: "2", id: "42", wantPath: "/internal/model/usage/team-report"},
			{name: "unlisted uses original report", extra: team + previous, unlisted: true, wantPath: "/previous"},
			{name: "original disabled report", extra: team + `,"teamUnlistedPathConfig":{}`, unlisted: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := `{"reportApiUrl":"https://billing.test/report","reportServiceName":"billing"` + tc.extra + "}"
				host, start := test.NewTestHost([]byte(cfg))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, start)
				id := host.InitializeHttpContext()
				if !tc.missingProvider {
					require.NoError(t, host.SetProperty([]string{"wasm.providerType"}, []byte("openai")))
				}
				require.NoError(t, host.SetProperty([]string{"wasm.requestModel"}, []byte("test-model")))
				if tc.id != "" {
					require.NoError(t, host.SetProperty([]string{"wasm.teamId"}, []byte(tc.id)))
				}
				if tc.forwarded {
					require.NoError(t, host.SetProperty([]string{"wasm.teamBillingSettlement"}, []byte("forward")))
				}
				if tc.personalOwner {
					require.NoError(t, host.SetProperty([]string{"wasm.teamPersonalOwnerUid"}, []byte("7")))
				}
				if tc.unlisted {
					require.NoError(t, host.SetProperty([]string{"wasm.teamBillingUnlistedPath"}, []byte("1")))
				}
				headers := [][2]string{{":method", "POST"}, {":path", "/v1/test"}, {":authority", "model.test"}, {OriginalAPIKey, "fixture"}}
				if tc.version != "" {
					headers = append(headers, [2]string{"X-Wujie-Billing-Version", tc.version})
				}
				action := host.CallOnRequestHeaders(id, headers, false)
				if tc.status != 0 {
					require.NotEqual(t, types.ActionContinue, action)
					response := host.GetSentLocalResponse(id)
					require.NotNil(t, response)
					require.Equal(t, tc.status, response.StatusCode)
					require.Empty(t, host.GetCalloutAttributesFromContext(id))
					return
				}
				require.Equal(t, types.ActionContinue, action)
				if tc.id != "" {
					require.NoError(t, host.SetProperty([]string{"wasm.teamId"}, []byte("99")))
				}
				if tc.lateHeader {
					host.SetHttpRequestHeaders(id, append(headers, [2]string{"X-Wujie-Billing-Version", "2"}))
				}
				host.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "application/json"}}, false)
				host.CallOnResponseBody(id, []byte(`{"model":"test-model","usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`), true)
				calls := host.GetCalloutAttributesFromContext(id)
				if tc.wantPath == "" {
					require.Empty(t, calls)
					return
				}
				require.Len(t, calls, 1)
				require.Contains(t, calls[0].Headers, [2]string{":path", tc.wantPath})
				var report TokenUsageReportRequest
				require.NoError(t, json.Unmarshal(calls[0].Body, &report))
				require.Equal(t, int64(14), report.TotalToken)
				if tc.id != "" {
					require.Equal(t, int64(42), report.TeamID)
				} else {
					require.Zero(t, report.TeamID)
				}
			})
		}
	})
}
