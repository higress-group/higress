// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"testing"

	"github.com/alibaba/higress/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsAiTokenRateLimit)
}

var WasmPluginsAiTokenRateLimit = suite.ConformanceTest{
	ShortName:   "WasmPluginAiTokenRateLimit",
	Description: "The Ingress in the higress-conformance-infra namespace test the ai-token-ratelimit WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-token-ratelimit.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{{Meta: http.AssertionMeta{
			TestCaseName:    "case 1: request within token limit passes",
			TargetBackend:   "infra-backend-v1",
			TargetNamespace: "higress-conformance-infra",
		}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions?apikey=test-key-1",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"Hello"}],  
                                                        "stream": false  
                                                }`),
		}, ExpectedRequest: &http.ExpectedRequest{Request: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions?apikey=test-key-1",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                                "model": "gpt-4",  
                                                                "messages": [{"role":"user","content":"Hello"}],  
                                                                "stream": false  
                                                        }`),
		}}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode: 200,
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:  "case 2: request exceeding token limit blocked",
			CompareTarget: http.CompareTargetResponse}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions?apikey=test-key-2",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"Hello"}],  
                                                        "stream": false  
                                                }`),
		}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode:  429,
			ContentType: http.ContentTypeApplicationJson,
			Body:        []byte(`Too many requests`),
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:    "case 3: request with header-based rate limiting",
			TargetBackend:   "infra-backend-v1",
			TargetNamespace: "higress-conformance-infra",
		}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.custom.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			Headers:     map[string]string{"x-api-key": "header-key-1"},
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"Hello"}],  
                                                        "stream": false  
                                                }`),
		}, ExpectedRequest: &http.ExpectedRequest{Request: http.Request{
			Host:        "api.custom.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			Headers:     map[string]string{"x-api-key": "header-key-1"},
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                                "model": "gpt-4",  
                                                                "messages": [{"role":"user","content":"Hello"}],  
                                                                "stream": false  
                                                        }`),
		}}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode: 200,
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:  "case 4: IP-based rate limiting blocked",
			CompareTarget: http.CompareTargetResponse}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.ip-limit.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"Hello"}],  
                                                        "stream": false  
                                                }`),
		}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode:  429,
			ContentType: http.ContentTypeApplicationJson,
			Body:        []byte(`Too many requests`),
		}}}}
		t.Run("WasmPlugins ai-token-ratelimit", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)

			}

		})

	},
}
