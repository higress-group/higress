// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	conformancehttp "github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() { Register(GoWasmMcpProxyRoutingCompatibility) }

var GoWasmMcpProxyRoutingCompatibility = suite.ConformanceTest{
	ShortName:   "GoWasmMcpProxyRoutingCompatibility",
	Description: "MCP proxy final business calls inherit ordinary headers and execute original route header and timeout policies.",
	Manifests:   []string{"tests/go-wasm-mcp-proxy-routing-compatibility.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		for _, strategy := range []string{"legacy", "modern", "auto"} {
			t.Run(strategy, func(t *testing.T) {
				headers := map[string]string{"Cookie": "synthetic=compat", "X-Tenant": "synthetic-tenant", "Proxy-Authorization": "synthetic-proxy-credential", "Accept": "application/json,text/event-stream"}
				params := `"name":"echo","arguments":{}`
				if strategy != "legacy" {
					headers["MCP-Protocol-Version"] = "2026-07-28"
					headers["Mcp-Method"] = "tools/call"
					headers["Mcp-Name"] = "echo"
					params += `,"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
				}
				result := `{"content":[{"type":"text","text":"headers-and-route-ok"}],"isError":false}`
				if strategy != "legacy" {
					result = `{"content":[{"type":"text","text":"headers-and-route-ok"}],"isError":false,"resultType":"complete"}`
				}
				assertion := conformancehttp.Assertion{
					Meta:     conformancehttp.AssertionMeta{TestCaseName: strategy, CompareTarget: conformancehttp.CompareTargetResponse},
					Request:  conformancehttp.AssertionRequest{ActualRequest: conformancehttp.Request{Host: "mcp-routing-" + strategy + ".example.com", Path: "/mcp", Method: "POST", Headers: headers, ContentType: conformancehttp.ContentTypeApplicationJson, Body: []byte(`{"jsonrpc":"2.0","id":"routing","method":"tools/call","params":{` + params + `}}`)}},
					Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 200, ContentType: conformancehttp.ContentTypeApplicationJson, Body: []byte(`{"jsonrpc":"2.0","id":"routing","result":` + result + `}`)}},
				}
				conformancehttp.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, assertion)
				// Wait for both routes to converge before measuring individual requests.
				assertion.Request.ActualRequest.Path = "/timeout"
				conformancehttp.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, assertion)
				for _, path := range []string{"/mcp", "/timeout"} {
					t.Run(path, func(t *testing.T) {
						request := roundtripper.Request{
							URL:  url.URL{Scheme: "http", Host: suite.GatewayAddress, Path: path},
							Host: assertion.Request.ActualRequest.Host, Protocol: "HTTP/1.1", Method: "POST",
							Headers: map[string][]string{}, ContentType: conformancehttp.ContentTypeApplicationJson,
							Body: []byte(strings.Replace(string(assertion.Request.ActualRequest.Body), `"arguments":{}`, `"arguments":{"delayMs":3000}`, 1)),
						}
						for name, value := range headers {
							request.Headers[name] = []string{value}
						}
						begin := time.Now()
						_, response, err := suite.RoundTripper.CaptureRoundTrip(request)
						elapsed := time.Since(begin)
						require.NoError(t, err)
						require.Equal(t, "routing", gjson.GetBytes(response.Body, "id").String())
						if path == "/mcp" {
							require.Equal(t, 200, response.StatusCode)
							require.JSONEq(t, string(assertion.Response.ExpectedResponse.Body), string(response.Body))
							require.GreaterOrEqual(t, elapsed, 2800*time.Millisecond, "the control backend must actually sleep")
						} else {
							require.False(t, gjson.GetBytes(response.Body, "result").Exists(), string(response.Body))
							require.Equal(t, int64(-32603), gjson.GetBytes(response.Body, "error.code").Int(), string(response.Body))
							if strategy == "auto" {
								require.Equal(t, 200, response.StatusCode)
								require.Contains(t, gjson.GetBytes(response.Body, "error.message").String(), "backend tools/call failed")
							} else {
								require.Equal(t, 200, response.StatusCode, "explicit profiles retain their JSON-RPC error contract")
								require.Equal(t, "backend tools/call failed", gjson.GetBytes(response.Body, "error.message").String())
							}
							require.GreaterOrEqual(t, elapsed, 800*time.Millisecond)
							require.Less(t, elapsed, 2800*time.Millisecond, "the 1s route deadline must beat the 3s backend and 5s callout deadline")
						}
					})
				}
			})
		}
	},
}
