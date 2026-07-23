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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/cert"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/kubernetes"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(HTTPHttpsWithoutSni)
}

var HTTPHttpsWithoutSni = suite.ConformanceTest{
	ShortName:   "HTTPHttpsWithoutSni",
	Description: "A single Ingress in the higress-conformance-infra namespace for https without sni.",
	Manifests:   []string{"tests/httproute-https-without-sni.yaml"},
	Features:    []suite.SupportedFeature{suite.HTTPConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		// Prepare secrets for testcases
		caCertOut, _, caCert, caKey := cert.MustGenerateCaCert(t)
		svcCertOut, svcKeyOut := cert.MustGenerateCertWithCA(t, cert.ServerCertType, caCert, caKey, []string{"foo.com"})
		fooSecret := kubernetes.ConstructTLSSecret("higress-conformance-infra", "foo-secret", svcCertOut.Bytes(), svcKeyOut.Bytes())
		suite.Applier.MustApplyObjectsWithCleanup(t, suite.Client, suite.TimeoutConfig, []client.Object{fooSecret}, suite.Cleanup)

		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: with sni",
					TargetBackend:   "infra-backend-v2",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo",
						Host: "foo.com",
						TLSConfig: &http.TLSConfig{
							SNI: "foo.com",
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo",
							Host: "foo.com",
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
					TestCaseName:    "case 1: without sni",
					TargetBackend:   "infra-backend-v2",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo",
						Host: "foo.com",
						TLSConfig: &http.TLSConfig{
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo",
							Host: "foo.com",
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

		t.Run("HTTPS without SNI", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
