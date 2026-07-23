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
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/cert"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(ConfigmapHttps)
}

var ConfigmapHttps = suite.ConformanceTest{
	ShortName:   "ConfigmapHttps",
	Description: "The Ingress in the higress-conformance-infra namespace uses the configmap https.",
	Manifests:   []string{"tests/configmap-https.yaml"},
	Features:    []suite.SupportedFeature{suite.HTTPConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		fooCAOut, _, fooCA, fooCAKey := cert.MustGenerateCaCert(t)
		fooCertOut, fooKeyOut := cert.MustGenerateCertWithCA(t, cert.ServerCertType, fooCA, fooCAKey, []string{"foo.com", "*.foo.com"})
		barCAOut, _, barCA, barCAKey := cert.MustGenerateCaCert(t)
		barCertOut, barKeyOut := cert.MustGenerateCertWithCA(t, cert.ServerCertType, barCA, barCAKey, []string{"bar.com"})

		testContext := context.Background()
		for _, secretUpdate := range []struct {
			objectKey  client.ObjectKey
			cert       []byte
			privateKey []byte
		}{
			{
				objectKey:  client.ObjectKey{Namespace: "higress-system", Name: "foo-com-secret"},
				cert:       fooCertOut.Bytes(),
				privateKey: fooKeyOut.Bytes(),
			},
			{
				objectKey:  client.ObjectKey{Namespace: "higress-conformance-infra", Name: "bar-com-secret"},
				cert:       barCertOut.Bytes(),
				privateKey: barKeyOut.Bytes(),
			},
		} {
			secret := &corev1.Secret{}
			if err := suite.Client.Get(testContext, secretUpdate.objectKey, secret); err != nil {
				t.Fatalf("failed to get TLS secret %s: %v", secretUpdate.objectKey, err)
			}
			secret.Data[corev1.TLSCertKey] = secretUpdate.cert
			secret.Data[corev1.TLSPrivateKeyKey] = secretUpdate.privateKey
			if err := suite.Client.Update(testContext, secret); err != nil {
				t.Fatalf("failed to update TLS secret %s: %v", secretUpdate.objectKey, err)
			}
		}

		testCases := []struct {
			httpAssert http.Assertion
		}{
			{
				httpAssert: http.Assertion{
					Meta: http.AssertionMeta{
						TestCaseName:    "test configmap bar-com https",
						TargetBackend:   "infra-backend-v2",
						TargetNamespace: "higress-conformance-infra",
					},
					Request: http.AssertionRequest{
						ActualRequest: http.Request{
							Path: "/barhttps",
							Host: "bar.com",
							TLSConfig: &http.TLSConfig{
								SNI: "bar.com",
								Certificates: http.Certificates{
									CACerts: [][]byte{barCAOut.Bytes()},
								},
							},
						},
						ExpectedRequest: &http.ExpectedRequest{
							Request: http.Request{
								Path: "/barhttps",
								Host: "bar.com",
							},
						},
					},
					Response: http.AssertionResponse{
						ExpectedResponse: http.Response{
							StatusCode: 200,
						},
					},
				},
			},
			{
				httpAssert: http.Assertion{
					Meta: http.AssertionMeta{
						TestCaseName:    "test configmap a-foo-com https",
						TargetBackend:   "infra-backend-v2",
						TargetNamespace: "higress-conformance-infra",
					},
					Request: http.AssertionRequest{
						ActualRequest: http.Request{
							Path: "/afoohttps",
							Host: "a.foo.com",
							TLSConfig: &http.TLSConfig{
								SNI: "a.foo.com",
								Certificates: http.Certificates{
									CACerts: [][]byte{fooCAOut.Bytes()},
								},
							},
						},
						ExpectedRequest: &http.ExpectedRequest{
							Request: http.Request{
								Path: "/afoohttps",
								Host: "a.foo.com",
							},
						},
					},
					Response: http.AssertionResponse{
						ExpectedResponse: http.Response{
							StatusCode: 200,
						},
					},
				},
			},
			{
				httpAssert: http.Assertion{
					Meta: http.AssertionMeta{
						TestCaseName:    "test configmap b-foo-com https",
						TargetBackend:   "infra-backend-v2",
						TargetNamespace: "higress-conformance-infra",
					},
					Request: http.AssertionRequest{
						ActualRequest: http.Request{
							Path: "/bfoohttps",
							Host: "b.foo.com",
							TLSConfig: &http.TLSConfig{
								SNI: "b.foo.com",
								Certificates: http.Certificates{
									CACerts: [][]byte{fooCAOut.Bytes()},
								},
							},
						},
						ExpectedRequest: &http.ExpectedRequest{
							Request: http.Request{
								Path: "/bfoohttps",
								Host: "b.foo.com",
							},
						},
					},
					Response: http.AssertionResponse{
						ExpectedResponse: http.Response{
							StatusCode: 200,
						},
					},
				},
			},
		}
		t.Run("Configmap Https", func(t *testing.T) {
			for _, testcase := range testCases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase.httpAssert)
			}
		})
	},
}
