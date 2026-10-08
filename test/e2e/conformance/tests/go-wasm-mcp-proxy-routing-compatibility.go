// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package tests

import (
	"testing"

	conformancehttp "github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() { Register(GoWasmMcpProxyRoutingCompatibility) }

var GoWasmMcpProxyRoutingCompatibility = suite.ConformanceTest{
	ShortName:   "GoWasmMcpProxyRoutingCompatibility",
	Description: "MCP proxy final business calls inherit ordinary headers and execute original route header policies.",
	Manifests:   []string{"tests/go-wasm-mcp-proxy-routing-compatibility.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		for _, strategy := range []string{"legacy", "modern", "auto"} {
			t.Run(strategy, func(t *testing.T) {
				headers := map[string]string{"Cookie": "synthetic=compat", "X-Tenant": "synthetic-tenant", "Accept": "application/json,text/event-stream"}
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
					Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 200, Body: []byte(`{"jsonrpc":"2.0","id":"routing","result":` + result + `}`)}},
				}
				conformancehttp.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, assertion)
			})
		}
	},
}
