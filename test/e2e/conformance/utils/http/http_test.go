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

package http

import (
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/stretchr/testify/require"
)

func TestCompareRequest(t *testing.T) {
	cases := []struct {
		caseName string
		errMsg   string
		req      *roundtripper.Request
		cReq     *roundtripper.CapturedRequest
		cRes     *roundtripper.CapturedResponse
		expected Assertion
	}{
		{
			caseName: "compare request header ok",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
				Body:      []byte(``),
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
				Body: []byte(``),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body:        []byte(``),
							ContentType: "",
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body ok",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: map[string]interface{}{
					"X-body-test1": []interface{}{"b1"},
					"X-body-test2": []interface{}{"b2", "b22"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body: []byte(`
							{
								"X-body-test1":["b1"],
								"X-body-test2":["b2","b22"]
							}`),
							ContentType: ContentTypeApplicationJson,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body fail, because headers not consistent",
			errMsg:   "expected X-header-test1 header to be set to h1, got h1,hn",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1", "hn"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: map[string]interface{}{
					"X-body-test1": []interface{}{"b1"},
					"X-body-test2": []interface{}{"b2", "b22"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body: []byte(`
							{
								"X-body-test1":["b1"],
								"X-body-test2":["b2","b22"]
							}`),
							ContentType: ContentTypeApplicationJson,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body fail, because body not consistent",
			errMsg:   "expected application/json body to be",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: map[string]interface{}{
					"X-body-test1": []interface{}{"b1", "bn"},
					"X-body-test2": []interface{}{"b2", "b22"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body: []byte(`
							{
								"X-body-test1":["b1"],
								"X-body-test2":["b2","b22"]
							}`),
							ContentType: ContentTypeApplicationJson,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body ok, body type is text/plain",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"text/plain"},
				},
				Body:      "hello higress",
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body:        []byte(`hello higress`),
							ContentType: ContentTypeTextPlain,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body fail, because body not consistent. body type is text/plain",
			errMsg:   "expected text/plain body to be",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"text/plain"},
				},
				Body:      "Hello Higress",
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body:        []byte(`hello higress`),
							ContentType: ContentTypeTextPlain,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body ok, body type is FormUrlencoded",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/x-www-form-urlencoded"},
				},
				Body: map[string][]string{
					"X-body-test1": {"b1"},
					"X-body-test2": {"b2", "b22"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body:        []byte(`X-body-test1=b1&X-body-test2=b2&X-body-test2=b22`),
							ContentType: ContentTypeFormUrlencoded,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body fail, because body not consistent, body type is FormUrlencoded",
			errMsg:   "expected application/x-www-form-urlencoded body to be",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/x-www-form-urlencoded"},
				},
				Body: map[string][]string{
					"X-body-test1": {"b1", "bn"},
					"X-body-test2": {"b2", "b22"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body:        []byte(`X-body-test1=b1&X-body-test2=b2&X-body-test2=b22`),
							ContentType: ContentTypeFormUrlencoded,
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body ok, body type is MultipartForm",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"multipart/form-data; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd"},
				},
				Body: map[string][]string{
					"name": {"denzel"},
					"flag": {"test"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body: []byte(
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
									"Content-Type: image/png\r\n\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
									"denzel\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
									"test\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
							ContentType: ContentTypeMultipartForm + "; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd",
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare request header&body fail, because body not consistent. body type is MultipartForm",
			errMsg:   "expected multipart/form-data body to be",
			req:      &roundtripper.Request{},
			cReq: &roundtripper.CapturedRequest{
				Path:   "/",
				Host:   "foo.com",
				Method: "GET",
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"multipart/form-data; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd"},
				},
				Body: map[string][]string{
					"name": {"higress"},
					"flag": {"test"},
				},
				Namespace: "",
				Pod:       "",
			},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetRequest},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request: Request{
							Host:   "foo.com",
							Method: "GET",
							Path:   "/",
							Headers: map[string]string{
								"X-header-test1": "h1",
								"X-header-test2": "h2,h22",
							},
							Body: []byte(
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
									"Content-Type: image/png\r\n\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
									"denzel\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
									"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
									"test\r\n" +
									"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
							ContentType: ContentTypeMultipartForm + "; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd",
						},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode:    200,
						Headers:       map[string]string{},
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			err := CompareRequest(c.req, c.cReq, c.cRes, c.expected)
			if c.errMsg != "" {
				require.ErrorContains(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCompareResponse(t *testing.T) {
	cases := []struct {
		caseName string
		errMsg   string
		req      *roundtripper.Request
		cReq     *roundtripper.CapturedRequest
		cRes     *roundtripper.CapturedResponse
		expected Assertion
	}{
		{
			caseName: "compare response header ok",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
				},
				Body: []byte(``),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body:          []byte(``),
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body ok",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: []byte(`
				{
					"X-body-test1":["b1"],
					"X-body-test2":["b2", "b22"]
				}`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body: []byte(`
						{
							"X-body-test1":["b1"],
							"X-body-test2":["b2","b22"]
						}`),
						ContentType:   ContentTypeApplicationJson,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body fail, because headers not consistent",
			errMsg:   "expected X-header-test1 header to be set to h1, got h1,hn",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1", "hn"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: []byte(`
				{
					"X-body-test1":["b1"],
					"X-body-test2":["b2", "b22"]
				}`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body: []byte(`
						{
							"X-body-test1":["b1"],
							"X-body-test2":["b2","b22"]
						}`),
						ContentType:   ContentTypeApplicationJson,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body fail, because body not consistent",
			errMsg:   "expected application/json body to be",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/json"},
				},
				Body: []byte(`
				{
					"X-body-test1":["b1", "bn"],
					"X-body-test2":["b2", "b22"]
				}`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body: []byte(`
						{
							"X-body-test1":["b1"],
							"X-body-test2":["b2","b22"]
						}`),
						ContentType:   ContentTypeApplicationJson,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body ok, body type is text/plain",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"text/plain"},
				},
				Body: []byte(`hello higress`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body:          []byte(`hello higress`),
						ContentType:   ContentTypeTextPlain,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body fail, because body not consistent. body type is text/plain",
			errMsg:   "expected text/plain body to be",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"text/plain"},
				},
				Body: []byte(`Hello Higress`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body:          []byte(`hello higress`),
						ContentType:   ContentTypeTextPlain,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body ok, body type is FormUrlencoded",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/x-www-form-urlencoded"},
				},
				Body: []byte(`X-body-test1=b1&X-body-test2=b2&X-body-test2=b22`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body:          []byte(`X-body-test1=b1&X-body-test2=b2&X-body-test2=b22`),
						ContentType:   ContentTypeFormUrlencoded,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body fail, because body not consistent, body type is FormUrlencoded",
			errMsg:   "expected application/x-www-form-urlencoded body to be",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"Content-Type":   {"application/x-www-form-urlencoded"},
				},
				Body: []byte(`X-body-test1=b1&X-body-test1=bn&X-body-test2=b2&X-body-test2=b22`),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body:          []byte(`X-body-test1=b1&X-body-test2=b2&X-body-test2=b22`),
						ContentType:   ContentTypeFormUrlencoded,
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body ok, body type is MultipartForm",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"ContentType":    {"multipart/form-data; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd"},
				},
				Body: []byte(
					"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
						"Content-Type: image/png\r\n\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
						"denzel\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
						"test\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body: []byte(
							"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
								"Content-Type: image/png\r\n\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
								"denzel\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
								"test\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
						ContentType:   ContentTypeMultipartForm + "; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd",
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
		{
			caseName: "compare response header&body fail, because body not consistent. body type is MultipartForm",
			errMsg:   "expected multipart/form-data body to be",
			req:      &roundtripper.Request{},
			cReq:     &roundtripper.CapturedRequest{},
			cRes: &roundtripper.CapturedResponse{
				StatusCode: 200,
				Headers: map[string][]string{
					"X-header-test1": {"h1"},
					"X-header-test2": {"h2", "h22"},
					"ContentType":    {"multipart/form-data; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd"},
				},
				Body: []byte(
					"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
						"Content-Type: image/png\r\n\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
						"higress\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
						"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
						"test\r\n" +
						"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
			},
			expected: Assertion{
				Meta: AssertionMeta{TestCaseName: "", TargetBackend: "", TargetNamespace: "", CompareTarget: CompareTargetResponse},
				Request: AssertionRequest{
					ActualRequest: Request{},
					ExpectedRequest: &ExpectedRequest{
						Request:       Request{},
						AbsentHeaders: []string{},
					},
				},
				Response: AssertionResponse{
					ExpectedResponse: Response{
						StatusCode: 200,
						Headers: map[string]string{
							"X-header-test1": "h1",
							"X-header-test2": "h2,h22",
						},
						Body: []byte(
							"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"file\"; filename=\"Screenshot.png\"\r\n" +
								"Content-Type: image/png\r\n\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"name\"\r\n\r\n" +
								"denzel\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd\r\n" +
								"Content-Disposition: form-data; name=\"flag\"\r\n\r\n" +
								"test\r\n" +
								"------WebKitFormBoundaryAnydWsQ1ajKuGoCd--\r\n"),
						ContentType:   ContentTypeMultipartForm + "; boundary=----WebKitFormBoundaryAnydWsQ1ajKuGoCd",
						AbsentHeaders: []string{},
					},
					AdditionalResponseHeaders: map[string]string{},
					ExpectedResponseNoRequest: false,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(t *testing.T) {
			err := CompareResponse(c.cRes, c.expected)
			if c.errMsg != "" {
				require.ErrorContains(t, err, c.errMsg)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCompareSSEEventsWithIgnoreFields(t *testing.T) {
	cases := []struct {
		name         string
		expected     string
		actual       string
		ignoreFields []string
		wantErr      bool
	}{
		{
			name:         "created differs but ignored, ends with DONE",
			expected:     "data: {\"id\":\"x\",\"created\":10,\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n",
			actual:       "data: {\"id\":\"x\",\"created\":9999,\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n",
			ignoreFields: []string{"created"},
			wantErr:      false,
		},
		{
			name:         "content mismatch is caught",
			expected:     "data: {\"created\":10,\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
			actual:       "data: {\"created\":11,\"choices\":[{\"delta\":{\"content\":\"bye\"}}]}\n\n",
			ignoreFields: []string{"created"},
			wantErr:      true,
		},
		{
			name:         "event count mismatch is caught",
			expected:     "data: {\"created\":10}\n\ndata: [DONE]\n\n",
			actual:       "data: {\"created\":10}\n\n",
			ignoreFields: []string{"created"},
			wantErr:      true,
		},
		{
			name:         "DONE vs non-DONE mismatch is caught",
			expected:     "data: [DONE]\n\n",
			actual:       "data: {\"created\":10}\n\n",
			ignoreFields: []string{"created"},
			wantErr:      true,
		},
		{
			name:     "malformed expected event errors",
			expected: "data: {not json\n\n",
			actual:   "data: {\"a\":1}\n\n",
			wantErr:  true,
		},
		{
			name:     "malformed actual event errors",
			expected: "data: {\"a\":1}\n\n",
			actual:   "data: {not json\n\n",
			wantErr:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CompareSSEEventsWithIgnoreFields([]byte(c.expected), []byte(c.actual), c.ignoreFields)
			if c.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
