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
	"crypto/tls"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/cert"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/kubernetes"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(HTTPRouteDownstreamEncryption)
}

var HTTPRouteDownstreamEncryption = suite.ConformanceTest{
	ShortName:   "HTTPRouteDownstreamEncryption",
	Description: "A single Ingress in the higress-conformance-infra namespace for downstream encryption.",
	Manifests:   []string{"tests/httproute-downstream-encryption.yaml"},
	Features:    []suite.SupportedFeature{suite.HTTPConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		// Prepare certificates and secrets for testcases
		caCertOut, _, caCert, caKey := cert.MustGenerateCaCert(t)
		svcCertOut, svcKeyOut := cert.MustGenerateCertWithCA(t, cert.ServerCertType, caCert, caKey, []string{
			"foo.com",
			"foo1.com",
			"foo2.com",
			"foo3.com",
		})
		cliCertOut, cliKeyOut := cert.MustGenerateCertWithCA(t, cert.ClientCertType, caCert, caKey, nil)
		fooSecret := kubernetes.ConstructTLSSecret("higress-conformance-infra", "foo-secret", svcCertOut.Bytes(), svcKeyOut.Bytes())
		fooSecretCACert := kubernetes.ConstructCASecret("higress-conformance-infra", "foo-secret-cacert", caCertOut.Bytes())
		suite.Applier.MustApplyObjectsWithCleanup(t, suite.Client, suite.TimeoutConfig, []client.Object{fooSecret, fooSecretCACert}, suite.Cleanup)

		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: auth-tls-secret annotation",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},

				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo1",
						Host: "foo1.com",
						TLSConfig: &http.TLSConfig{
							SNI: "foo1.com",
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
								ClientKeyPairs: []http.ClientKeyPair{
									{
										ClientCert: cliCertOut.Bytes(),
										ClientKey:  cliKeyOut.Bytes(),
									},
								},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo1",
							Host: "foo1.com",
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
					TestCaseName:    "case 2: ssl-cipher annotation, ingress of one cipher suite",
					TargetBackend:   "infra-backend-v2",
					TargetNamespace: "higress-conformance-infra",
				},

				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo2",
						Host: "foo2.com",
						TLSConfig: &http.TLSConfig{
							SNI:          "foo2.com",
							MaxVersion:   tls.VersionTLS12,
							CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA},
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
								ClientKeyPairs: []http.ClientKeyPair{
									{
										ClientCert: cliCertOut.Bytes(),
										ClientKey:  cliKeyOut.Bytes(),
									},
								},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo2",
							Host: "foo2.com",
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
					TestCaseName:    "case 3: ssl-cipher annotation, ingress of multiple cipher suites",
					TargetBackend:   "infra-backend-v3",
					TargetNamespace: "higress-conformance-infra",
				},

				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo3",
						Host: "foo3.com",
						TLSConfig: &http.TLSConfig{
							SNI:          "foo3.com",
							MaxVersion:   tls.VersionTLS12,
							CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305},
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
								ClientKeyPairs: []http.ClientKeyPair{
									{
										ClientCert: cliCertOut.Bytes(),
										ClientKey:  cliKeyOut.Bytes(),
									},
								},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo3",
							Host: "foo3.com",
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
					TestCaseName:    "case 4: ssl-cipher annotation, TLSv1.2 cipher suites are invalid in TLSv1.3",
					TargetBackend:   "infra-backend-v3",
					TargetNamespace: "higress-conformance-infra",
				},

				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Path: "/foo3",
						Host: "foo3.com",
						TLSConfig: &http.TLSConfig{
							SNI:          "foo3.com",
							MinVersion:   tls.VersionTLS13,
							MaxVersion:   tls.VersionTLS13,
							CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
							Certificates: http.Certificates{
								CACerts: [][]byte{caCertOut.Bytes()},
								ClientKeyPairs: []http.ClientKeyPair{
									{
										ClientCert: cliCertOut.Bytes(),
										ClientKey:  cliKeyOut.Bytes(),
									},
								},
							},
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/foo3",
							Host: "foo3.com",
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

		t.Run("Downstream encryption", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
