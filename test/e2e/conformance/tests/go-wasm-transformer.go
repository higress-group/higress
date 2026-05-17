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

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsTransformer)
}

// TODO(WeixinX): Request and response body conformance check is not supported now (except for multipart/form-data)
var WasmPluginsTransformer = suite.ConformanceTest{
	ShortName:   "WasmPluginTransformer",
	Description: "The Ingress in the higress-conformance-infra namespace test the transformer WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-transformer.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: request header&query transformer",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo1.com",
						Path: "/get?k1=v11&k1=v12&k2=v2",
						Headers: map[string]string{
							"X-remove":        "exist",
							"X-not-renamed":   "test",
							"X-replace":       "not-replaced",
							"X-dedupe-first":  "1,2,3",
							"X-dedupe-last":   "a,b,c",
							"X-dedupe-unique": "1,2,3,3,2,1",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo1.com",
							Path: "/get?k2-new=v2-new&k3=v31&k3=v32&k4=v31", // url.Value.Encode() is ordered by key
							Headers: map[string]string{
								"X-renamed":       "test",
								"X-replace":       "replaced",
								"X-add-append":    "add,append", // header with same name
								"X-map":           "add,append",
								"X-dedupe-first":  "1",
								"X-dedupe-last":   "c",
								"X-dedupe-unique": "1,2,3",
							},
						},
						AbsentHeaders: []string{"X-remove"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: response header&query transformer",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo2.com",
						Path: "/get/index.html",
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo2.com",
							Path: "/get/index.html",
						},
					},
				},
				Response: http.AssertionResponse{
					AdditionalResponseHeaders: map[string]string{
						"X-remove":      "exist",
						"X-not-renamed": "test",
						"X-replace":     "not-replaced",
					},
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-renamed":    "test",
							"X-replace":    "replace-get",           // regexp matches path and replace "replace-$1"
							"X-add-append": "add-foo2,append-index", // regexp matches host and replace "add-$1"
							"X-map":        "add-foo2,append-index",
						},
						AbsentHeaders: []string{"X-remove"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: req&resp bothway header&query transformer",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo3.com",
						Path: "/get/index.html?k1=v11&k1=v12&k2=v2",
						Headers: map[string]string{
							"X-remove":        "exist",
							"X-not-renamed":   "test",
							"X-replace":       "not-replaced",
							"X-dedupe-first":  "1,2,3",
							"X-dedupe-last":   "a,b,c",
							"X-dedupe-unique": "1,2,3,3,2,1",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo3.com",
							Path: "/get/index.html?k2-new=v2-new&k3=v31&k3=v32&k4=v31",
							Headers: map[string]string{
								"X-renamed":       "test",
								"X-replace":       "replaced",
								"X-add-append":    "add,append", // header with same name
								"X-map":           "add,append",
								"X-dedupe-first":  "1",
								"X-dedupe-last":   "c",
								"X-dedupe-unique": "1,2,3",
							},
						},
						AbsentHeaders: []string{"X-remove"},
					},
				},
				Response: http.AssertionResponse{
					AdditionalResponseHeaders: map[string]string{
						"X-remove":      "exist",
						"X-not-renamed": "test",
						"X-replace":     "not-replaced",
					},
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-renamed":    "test",
							"X-replace":    "replace-get",           // regexp matches path and replace "replace-$1"
							"X-add-append": "add-foo3,append-index", // regexp matches host and replace "add-$1"
							"X-map":        "add-foo3,append-index",
						},
						AbsentHeaders: []string{"X-remove"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: request transformer with arbitrary order",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo4.com",
						Path: "/get?k1=v11&k1=v12",
						Headers: map[string]string{
							"X-dedupe-first":  "1,2,3",
							"X-dedupe-last":   "a,b,c",
							"X-dedupe-unique": "1,2,3,3,2,1",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo4.com",
							Path: "/get?k2=v11&k2=v22&k3-new=v31",
							Headers: map[string]string{
								"X-add-append":            "add",
								"X-map-dedupe-first":      "1,append",
								"X-dedupe-last":           "X-dedupe-last-replaced",
								"X-dedupe-unique-renamed": "1,2,3",
							},
						},
						AbsentHeaders: []string{"X-dedupe-first"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},

			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 5: response transformer with arbitrary order",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo5.com",
						Path: "/get/index.html",
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo5.com",
							Path: "/get/index.html",
						},
					},
				},
				Response: http.AssertionResponse{
					AdditionalResponseHeaders: map[string]string{
						"X-remove":      "exist",
						"X-not-renamed": "test",
						"X-replace":     "not-replaced",
					},
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-renamed":    "test",
							"X-replace":    "replace-get",           // regexp matches path and replace "replace-$1"
							"X-add-append": "add-foo5,append-index", // regexp matches host and replace "add-$1"
							"X-map":        "add-foo5",
						},
						AbsentHeaders: []string{"X-remove"},
					},
				},
			},

			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 6: request transformer, map from query to header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo6.com",
						Path:    "/get?kmap=vmap",
						Headers: map[string]string{},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo6.com",
							Path: "/get?kmap=vmap",
							Headers: map[string]string{
								"X-map": "vmap",
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 7: request transformer, map from header to query",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo7.com",
						Path: "/get",
						Headers: map[string]string{
							"X-map": "vmap",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo7.com",
							Path: "/get?kmap=vmap",
							Headers: map[string]string{
								"X-map": "vmap",
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 8: request body transformer",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo8.com",
						Path: "/post",
						// TODO(Uncle-Justice) dedupe, replace的body插件逻辑有问题，暂跳过测试
						Method: "POST",
						Body: []byte(`
						{
							"X-removed":["v1", "v2"],
							"X-not-renamed":["v1"],
							"X-to-be-mapped":["v1", "v2"],
							"X-replace": "not-replaced"
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo8.com",
							Path: "/post",
							// TODO(Uncle-Justice) dedupe, replace的body插件逻辑有问题，暂跳过测试
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body: []byte(`
							{
								"X-renamed":["v1"],
								"X-add-append":["add","append"],
								"X-to-be-mapped":["v1", "v2"],
								"X-map":["v1", "v2"],
								"X-replace": "replaced"
							}
						`),
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 9: response json body transformer",
					TargetBackend:   "infra-backend-echo-body-v1",
					TargetNamespace: "higress-conformance-infra",
					CompareTarget:   http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo9.com",
						Path: "/post",
						// TODO(Uncle-Justice) dedupe, replace的body插件逻辑有问题，暂跳过测试
						Method: "POST",
						Body: []byte(`
						{
							"X-removed":["v1", "v2"],
							"X-not-renamed":["v1"],
							"X-to-be-mapped":["v1", "v2"]
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`
						{
							"X-renamed":["v1"],
							"X-add-append":["add","append"],
							"X-to-be-mapped":["v1", "v2"],
							"X-map":["v1", "v2"],
							"X-replace":"replaced"
						}
						`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 10: map from headers to body",
					TargetBackend:   "infra-backend-echo-body-v1",
					TargetNamespace: "higress-conformance-infra",
					CompareTarget:   http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo10.com",
						Path:    "/post",
						Method:  "POST",
						Headers: map[string]string{"X-map": "higress"},
						Body: []byte(`
						{
							"X-hello":"world"
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`
						{
							"X-hello":"world",
							"kmap":["higress"]
						}
						`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 11: map from querys to body",
					TargetBackend:   "infra-backend-echo-body-v1",
					TargetNamespace: "higress-conformance-infra",
					CompareTarget:   http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo11.com",
						Path:   "/post?X-map=higress",
						Method: "POST",
						Body: []byte(`
						{
							"X-hello": "world"
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`
						{
							"X-hello": "world",
							"test": {
								"kmap": ["higress"]
							}
						}
						`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 12: map from body to headers",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo12.com",
						Path:   "/post",
						Method: "POST",
						Body: []byte(`
						{
							"test": {
								"kmap": "higress"
							}
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:    "foo12.com",
							Path:    "/post",
							Method:  "POST",
							Headers: map[string]string{"X-map": "higress"},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 13: map from body to querys",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo13.com",
						Path:   "/post",
						Method: "POST",
						Body: []byte(`
						{
							"test": {
								"kmap": "higress"
							}
						}
						`),
						ContentType: http.ContentTypeApplicationJson,
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:   "foo13.com",
							Path:   "/post?X-map=higress",
							Method: "POST",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 14: headers & querys, when replace key is not exist, it is equivalent to app",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo14.com",
						Path: "/get?X-replace-querys=hello",
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:    "foo14.com",
							Path:    "/get?X-replace-querys=exist-querys",
							Headers: map[string]string{"X-replace-headers": "exist-headers"},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 15: body, when replace key is not exist, it is equivalent to add",
					TargetBackend:   "infra-backend-echo-body-v1",
					TargetNamespace: "higress-conformance-infra",
					CompareTarget:   http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo15.com",
						Path:        "/post",
						Method:      "POST",
						Body:        []byte(`{}`),
						ContentType: http.ContentTypeApplicationJson,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`
						{
							"X-replace-body": "exist-body"
						}
						`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 16: request reroute",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo16.com",
						Path: "/get",
						Headers: map[string]string{
							"reroute": "false",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:    "foo16.reroute.com",
							Path:    "/get",
							Headers: map[string]string{"reroute": "true"},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 17: request non reroute",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo17.com",
						Path: "/get",
						Headers: map[string]string{
							"reroute": "false",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo17.non-reroute.com",
							Path: "/get",
							// although the header was replaced, it was not rerouted
							Headers: map[string]string{"reroute": "true"},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 18: request header transformer with split",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo18.com",
						Path: "/get",
						RawHeaders: map[string][]string{
							"X-split-dedupe-first": {"1,2,3"},
							"X-split-dedupe-last":  {"a,b,c"},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "foo18.com",
							Path: "/get",
							Headers: map[string]string{
								"X-split-dedupe-first": "1",
								"X-split-dedupe-last":  "c",
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 19: request multipart/form-data body with file",
					TargetBackend:   "infra-transformer-echo",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo19.com",
						Path:        "/post",
						Method:      "POST",
						ContentType: "multipart/form-data; boundary=--------------------------180978275079165582161528",
						Body: []byte(`
----------------------------180978275079165582161528
Content-Disposition: form-data; name="file1"; filename="test.txt"
Content-Type: text/plain

这是一个txt文件
----------------------------180978275079165582161528
Content-Disposition: form-data; name="file2"; filename="test.md"
Content-Type: text/plain

这是一个md文件
----------------------------180978275079165582161528
Content-Disposition: form-data; name="x-process"

wasm
----------------------------180978275079165582161528--
`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo19.com",
							Path:        "/post",
							Method:      "POST",
							ContentType: "multipart/form-data; boundary=--------------------------180978275079165582161528",
							Body: []byte(`
----------------------------180978275079165582161528
Content-Disposition: form-data; name="file1"; filename="test.txt"
Content-Type: text/plain

这是一个txt文件
----------------------------180978275079165582161528
Content-Disposition: form-data; name="file2"; filename="test.md"
Content-Type: text/plain

这是一个md文件
----------------------------180978275079165582161528
Content-Disposition: form-data; name="x-process"

wasm
----------------------------180978275079165582161528--
`,
							),
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
		}
		t.Run("WasmPlugin transformer", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
