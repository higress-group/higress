package tests

import (
	"testing"

	"github.com/alibaba/higress/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsModelMapper)
}

var WasmPluginsModelMapper = suite.ConformanceTest{
	ShortName:   "WasmPluginModelMapper",
	Description: "The Ingress in the higress-conformance-ai-backend namespace tests the model-mapper WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMCPPConformanceFeature},
	Manifests:   []string{"tests/cpp-wasm-model-mapper.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				// 测试精确匹配: gpt-4o -> qwen-vl-plus
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 1: exact match",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"qwen-vl-plus","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试前缀匹配: gpt-4-1106-preview -> qwen-max
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 2: prefix match",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4-1106-preview","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"qwen-max","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试默认匹配: claude-2 -> qwen-turbo
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 3: default match",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"claude-2","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"qwen-turbo","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试保留原值: text-embedding-v1 -> text-embedding-v1 (不改变)
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 4: keep original value",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"text-embedding-v1","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"text-embedding-v1","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试自定义 modelKey 配置: engine -> qwen-turbo
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 5: custom model key",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"engine":"davinci","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"engine":"qwen-turbo","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试路径匹配: 非配置的路径不进行处理
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 6: path not match",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/embeddings",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
			{
				// 测试路径自定义匹配：使用自定义路径后缀
				Meta: http.AssertionMeta{
					TestCaseName:  "model mapper case 7: custom path match",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/api/custom/endpoint",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"qwen-vl-plus","messages":[{"role":"user","content":"测试消息"}]}`),
					},
				},
			},
		}
		t.Run("WasmPlugins model-mapper", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
