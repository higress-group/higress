// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"testing"

	conformancehttp "github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsMCP20260728)
}

var WasmPluginsMCP20260728 = suite.ConformanceTest{
	ShortName:   "WasmPluginsMCP20260728",
	Description: "The mcp-server WASM plugin serves the 2026-07-28 tools baseline while preserving legacy and origin-security behavior.",
	Manifests:   []string{"tests/go-wasm-mcp-2026-07-28.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		modernMeta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"higress-e2e","version":"1.0.0"},"io.modelcontextprotocol/clientCapabilities":{}}`
		modernHeaders := func(method, name string) map[string]string {
			headers := map[string]string{
				"Accept":               "application/json, text/event-stream",
				"MCP-Protocol-Version": "2026-07-28",
				"Mcp-Method":           method,
			}
			if name != "" {
				headers["Mcp-Name"] = name
			}
			return headers
		}
		cases := []conformancehttp.Assertion{
			mcpResponseAssertion(
				"modern discovery", modernHeaders("server/discover", ""),
				[]byte(`{"jsonrpc":"2.0","id":"discover-e2e","method":"server/discover","params":{`+modernMeta+`}}`),
				[]byte(`{"jsonrpc":"2.0","id":"discover-e2e","result":{"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"e2e-mcp-2026","version":"1.0.0"}},"cacheScope":"private","capabilities":{"tools":{}},"resultType":"complete","supportedVersions":["2024-11-05","2025-03-26","2025-06-18","2026-07-28"],"ttlMs":0}}`),
			),
			mcpResponseAssertion(
				"modern tools/list", modernHeaders("tools/list", ""),
				[]byte(`{"jsonrpc":"2.0","id":"list-e2e","method":"tools/list","params":{`+modernMeta+`}}`),
				[]byte(`{"jsonrpc":"2.0","id":"list-e2e","result":{"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"e2e-mcp-2026","version":"1.0.0"}},"cacheScope":"private","resultType":"complete","tools":[{"description":"Return deterministic fixture weather","inputSchema":{"properties":{"location":{"description":"City name","type":"string"}},"required":["location"],"type":"object"},"name":"get_weather"}],"ttlMs":0}}`),
			),
			mcpResponseAssertion(
				"modern tools/call", modernHeaders("tools/call", "get_weather"),
				[]byte(`{"jsonrpc":"2.0","id":"call-e2e","method":"tools/call","params":{`+modernMeta+`,"name":"get_weather","arguments":{"location":"Shanghai"}}}`),
				[]byte(`{"jsonrpc":"2.0","id":"call-e2e","result":{"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"e2e-mcp-2026","version":"1.0.0"}},"content":[{"text":"weather for Shanghai","type":"text"}],"isError":false,"resultType":"complete"}}`),
			),
			mcpResponseAssertion(
				"legacy tools/list remains unshaped", nil,
				[]byte(`{"jsonrpc":"2.0","id":"legacy-list","method":"tools/list","params":{}}`),
				[]byte(`{"jsonrpc":"2.0","id":"legacy-list","result":{"tools":[{"description":"Return deterministic fixture weather","inputSchema":{"properties":{"location":{"description":"City name","type":"string"}},"required":["location"],"type":"object"},"name":"get_weather"}]}}`),
			),
			{
				Meta: conformancehttp.AssertionMeta{TestCaseName: "cross-origin request rejected", CompareTarget: conformancehttp.CompareTargetResponse},
				Request: conformancehttp.AssertionRequest{ActualRequest: conformancehttp.Request{
					Host: "mcp-2026.example.com", Path: "/mcp", Method: "POST",
					Headers: modernHeaders("tools/list", ""), ContentType: conformancehttp.ContentTypeApplicationJson,
					Body: []byte(`{"jsonrpc":"2.0","id":"origin-e2e","method":"tools/list","params":{` + modernMeta + `}}`),
				}},
				Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 403}},
			},
		}
		cases[len(cases)-1].Request.ActualRequest.Headers["Origin"] = "https://evil.example"
		for _, testcase := range cases {
			t.Run(testcase.Meta.TestCaseName, func(t *testing.T) {
				conformancehttp.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			})
		}
	},
}

func mcpResponseAssertion(name string, headers map[string]string, requestBody, responseBody []byte) conformancehttp.Assertion {
	return conformancehttp.Assertion{
		Meta: conformancehttp.AssertionMeta{TestCaseName: name, CompareTarget: conformancehttp.CompareTargetResponse},
		Request: conformancehttp.AssertionRequest{ActualRequest: conformancehttp.Request{
			Host: "mcp-2026.example.com", Path: "/mcp", Method: "POST", Headers: headers,
			ContentType: conformancehttp.ContentTypeApplicationJson, Body: requestBody,
		}},
		Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{
			StatusCode: 200, ContentType: conformancehttp.ContentTypeApplicationJson, Body: responseBody,
		}},
	}
}
