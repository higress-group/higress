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

package common

import (
	"testing"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/config"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/annotations"
	"github.com/stretchr/testify/assert"
)

func TestConstructRouteName(t *testing.T) {
	testCases := []struct {
		input  *WrapperHTTPRoute
		expect string
	}{
		{
			input: &WrapperHTTPRoute{
				Host:           "test.com",
				OriginPathType: Exact,
				OriginPath:     "/test",
				HTTPRoute:      &networking.HTTPRoute{},
			},
			expect: "test.com-exact-/test",
		},
		{
			input: &WrapperHTTPRoute{
				Host:           "*.test.com",
				OriginPathType: PrefixRegex,
				OriginPath:     "/test/(.*)/?[0-9]",
				HTTPRoute:      &networking.HTTPRoute{},
			},
			expect: "*.test.com-prefixRegex-/test/(.*)/?[0-9]",
		},
		{
			input: &WrapperHTTPRoute{
				Host:           "test.com",
				OriginPathType: Exact,
				OriginPath:     "/test",
				HTTPRoute: &networking.HTTPRoute{
					Match: []*networking.HTTPMatchRequest{
						{
							Headers: map[string]*networking.StringMatch{
								"b": {
									MatchType: &networking.StringMatch_Regex{
										Regex: "a?c.*",
									},
								},
								"a": {
									MatchType: &networking.StringMatch_Exact{
										Exact: "hello",
									},
								},
							},
						},
					},
				},
			},
			expect: "test.com-exact-/test-exact-a-hello-regex-b-a?c.*",
		},
		{
			input: &WrapperHTTPRoute{
				Host:           "test.com",
				OriginPathType: Prefix,
				OriginPath:     "/test",
				HTTPRoute: &networking.HTTPRoute{
					Match: []*networking.HTTPMatchRequest{
						{
							QueryParams: map[string]*networking.StringMatch{
								"b": {
									MatchType: &networking.StringMatch_Regex{
										Regex: "a?c.*",
									},
								},
								"a": {
									MatchType: &networking.StringMatch_Exact{
										Exact: "hello",
									},
								},
							},
						},
					},
				},
			},
			expect: "test.com-prefix-/test-exact:a:hello-regex:b:a?c.*",
		},
		{
			input: &WrapperHTTPRoute{
				Host:           "test.com",
				OriginPathType: Prefix,
				OriginPath:     "/test",
				HTTPRoute: &networking.HTTPRoute{
					Match: []*networking.HTTPMatchRequest{
						{
							Headers: map[string]*networking.StringMatch{
								"f": {
									MatchType: &networking.StringMatch_Regex{
										Regex: "abc?",
									},
								},
								"e": {
									MatchType: &networking.StringMatch_Exact{
										Exact: "bye",
									},
								},
							},
							QueryParams: map[string]*networking.StringMatch{
								"b": {
									MatchType: &networking.StringMatch_Regex{
										Regex: "a?c.*",
									},
								},
								"a": {
									MatchType: &networking.StringMatch_Exact{
										Exact: "hello",
									},
								},
							},
						},
					},
				},
			},
			expect: "test.com-prefix-/test-exact-e-bye-regex-f-abc?-exact:a:hello-regex:b:a?c.*",
		},
	}

	for _, c := range testCases {
		t.Run("", func(t *testing.T) {
			out := constructRouteName(c.input)
			if out != c.expect {
				t.Fatalf("Expect %s, but is %s", c.expect, out)
			}
		})
	}
}

func TestWrapperConfigForMcpDestinationProtocol(t *testing.T) {
	base := &WrapperConfig{
		AnnotationsConfig: &annotations.Ingress{
			UpstreamTLS: &annotations.UpstreamTLSConfig{
				BackendProtocol: "HTTPS",
				EnableSNI:       true,
				SNI:             "example.com",
			},
			Destination: &annotations.DestinationConfig{
				Protocols: map[string]string{
					"plain.example.com:80":   "HTTP",
					"secure.example.com:443": "HTTPS",
				},
			},
		},
	}

	httpWrapper := WrapperConfigForMcpDestination(base, &networking.HTTPRouteDestination{
		Destination: &networking.Destination{
			Host: "plain.example.com",
			Port: &networking.PortSelector{Number: 80},
		},
	})
	assert.Nil(t, httpWrapper.AnnotationsConfig.UpstreamTLS)
	assert.NotSame(t, base, httpWrapper)
	assert.NotNil(t, base.AnnotationsConfig.UpstreamTLS)

	httpsWrapper := WrapperConfigForMcpDestination(base, &networking.HTTPRouteDestination{
		Destination: &networking.Destination{
			Host: "secure.example.com",
			Port: &networking.PortSelector{Number: 443},
		},
	})
	assert.NotSame(t, base, httpsWrapper)
	assert.Equal(t, "HTTPS", httpsWrapper.AnnotationsConfig.UpstreamTLS.BackendProtocol)
	assert.True(t, httpsWrapper.AnnotationsConfig.UpstreamTLS.EnableSNI)
	assert.Equal(t, "example.com", httpsWrapper.AnnotationsConfig.UpstreamTLS.SNI)

	plainWrapper := WrapperConfigForMcpDestination(base, &networking.HTTPRouteDestination{
		Destination: &networking.Destination{Host: "other.example.com"},
	})
	assert.Same(t, base, plainWrapper)
}

func TestGenerateUniqueRouteName(t *testing.T) {
	input := &WrapperHTTPRoute{
		WrapperConfig: &WrapperConfig{
			Config: &config.Config{
				Meta: config.Meta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
		Host:           "test.com",
		OriginPathType: Prefix,
		OriginPath:     "/test",
		ClusterId:      "cluster1",
		HTTPRoute: &networking.HTTPRoute{
			Match: []*networking.HTTPMatchRequest{
				{
					Headers: map[string]*networking.StringMatch{
						"f": {
							MatchType: &networking.StringMatch_Regex{
								Regex: "abc?",
							},
						},
						"e": {
							MatchType: &networking.StringMatch_Exact{
								Exact: "bye",
							},
						},
					},
					QueryParams: map[string]*networking.StringMatch{
						"b": {
							MatchType: &networking.StringMatch_Regex{
								Regex: "a?c.*",
							},
						},
						"a": {
							MatchType: &networking.StringMatch_Exact{
								Exact: "hello",
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, "bar/foo", GenerateUniqueRouteName("xxx", input))
	assert.Equal(t, "foo", GenerateUniqueRouteName("bar", input))
}

func TestGetLbStatusList(t *testing.T) {
	clusterPrefix = "gw-123-"
	svcName := clusterPrefix
	aliyunHostname := "higress.cn-hangzhou.alb.aliyuncs.com"
	awsHostname := "k8s-kubeingr-higressg-1234567890.eu-north-1.elb.amazonaws.com"
	tencentHostname := "lb-12345678.clb.ap-guangzhou.tencentclb.com"
	svcList := []*v1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							IP: "2.2.2.2",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: awsHostname,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: aliyunHostname,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: tencentHostname,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: "1.1.1.1" + SvcHostNameSuffix,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: "4.4.4.4" + SvcHostNameSuffix,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							IP: "3.3.3.3",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeClusterIP,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							IP: "5.5.5.5",
						},
					},
				},
			},
		},
	}

	lbiList := GetLbStatusList(svcList)
	assert.Equal(t, []v1.LoadBalancerIngress{
		{IP: "1.1.1.1"},
		{IP: "2.2.2.2"},
		{IP: "3.3.3.3"},
		{IP: "4.4.4.4"},
		{Hostname: aliyunHostname},
		{Hostname: awsHostname},
		{Hostname: tencentHostname},
	}, lbiList)
}

func TestSortRoutes(t *testing.T) {
	input := []*WrapperHTTPRoute{
		{
			WrapperConfig: &WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				AnnotationsConfig: &annotations.Ingress{},
			},
			Host:           "test.com",
			OriginPathType: Prefix,
			OriginPath:     "/",
			ClusterId:      "cluster1",
			HTTPRoute: &networking.HTTPRoute{
				Name: "test-1",
			},
		},
		{
			WrapperConfig: &WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				AnnotationsConfig: &annotations.Ingress{},
			},
			Host:           "test.com",
			OriginPathType: Prefix,
			OriginPath:     "/a",
			ClusterId:      "cluster1",
			HTTPRoute: &networking.HTTPRoute{
				Name: "test-2",
			},
		},
		{
			WrapperConfig: &WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				AnnotationsConfig: &annotations.Ingress{},
			},
			HTTPRoute: &networking.HTTPRoute{
				Name: "test-3",
			},
			IsDefaultBackend: true,
		},
		{
			WrapperConfig: &WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				AnnotationsConfig: &annotations.Ingress{},
			},
			Host:           "test.com",
			OriginPathType: Exact,
			OriginPath:     "/b",
			ClusterId:      "cluster1",
			HTTPRoute: &networking.HTTPRoute{
				Name: "test-4",
			},
		},
		{
			WrapperConfig: &WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
				AnnotationsConfig: &annotations.Ingress{},
			},
			Host:           "test.com",
			OriginPathType: PrefixRegex,
			OriginPath:     "/d(.*)",
			ClusterId:      "cluster1",
			HTTPRoute: &networking.HTTPRoute{
				Name: "test-5",
			},
		},
	}

	SortHTTPRoutes(input)
	if (input[0].HTTPRoute.Name) != "test-4" {
		t.Fatal("should be test-4")
	}
	if (input[1].HTTPRoute.Name) != "test-2" {
		t.Fatal("should be test-2")
	}
	if (input[2].HTTPRoute.Name) != "test-5" {
		t.Fatal("should be test-5")
	}
	if (input[3].HTTPRoute.Name) != "test-1" {
		t.Fatal("should be test-1")
	}
	if (input[4].HTTPRoute.Name) != "test-3" {
		t.Fatal("should be test-3")
	}
}

// TestSortHTTPRoutesWithMoreRules include headers, query params, methods
func TestSortHTTPRoutesWithMoreRules(t *testing.T) {
	input := []struct {
		order      string
		pathType   PathType
		path       string
		method     *networking.StringMatch
		header     map[string]*networking.StringMatch
		queryParam map[string]*networking.StringMatch
	}{
		{
			order:    "1",
			pathType: Exact,
			path:     "/bar",
		},
		{
			order:    "2",
			pathType: Prefix,
			path:     "/bar",
		},
		{
			order:    "3",
			pathType: Prefix,
			path:     "/bar",
			method: &networking.StringMatch{
				MatchType: &networking.StringMatch_Regex{Regex: "GET|PUT"},
			},
		},
		{
			order:    "4",
			pathType: Prefix,
			path:     "/bar",
			method: &networking.StringMatch{
				MatchType: &networking.StringMatch_Regex{Regex: "GET"},
			},
		},
		{
			order:    "5",
			pathType: Prefix,
			path:     "/bar",
			header: map[string]*networking.StringMatch{
				"foo": {
					MatchType: &networking.StringMatch_Exact{Exact: "bar"},
				},
			},
		},
		{
			order:    "6",
			pathType: Prefix,
			path:     "/bar",
			header: map[string]*networking.StringMatch{
				"foo": {
					MatchType: &networking.StringMatch_Exact{Exact: "bar"},
				},
				"bar": {
					MatchType: &networking.StringMatch_Exact{Exact: "foo"},
				},
			},
		},
		{
			order:    "7",
			pathType: Prefix,
			path:     "/bar",
			queryParam: map[string]*networking.StringMatch{
				"foo": {
					MatchType: &networking.StringMatch_Exact{Exact: "bar"},
				},
			},
		},
		{
			order:    "8",
			pathType: Prefix,
			path:     "/bar",
			queryParam: map[string]*networking.StringMatch{
				"foo": {
					MatchType: &networking.StringMatch_Exact{Exact: "bar"},
				},
				"bar": {
					MatchType: &networking.StringMatch_Exact{Exact: "foo"},
				},
			},
		},
		{
			order:    "9",
			pathType: Prefix,
			path:     "/bar",
			method: &networking.StringMatch{
				MatchType: &networking.StringMatch_Regex{Regex: "GET"},
			},
			queryParam: map[string]*networking.StringMatch{
				"foo": {
					MatchType: &networking.StringMatch_Exact{Exact: "bar"},
				},
			},
		},
		{
			order:    "10",
			pathType: Prefix,
			path:     "/bar",
			method: &networking.StringMatch{
				MatchType: &networking.StringMatch_Regex{Regex: "GET"},
			},
			queryParam: map[string]*networking.StringMatch{
				"bar": {
					MatchType: &networking.StringMatch_Exact{Exact: "foo"},
				},
			},
		},
	}

	origin := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	expect := []string{"1", "9", "10", "4", "3", "6", "5", "8", "7", "2"}

	var list []*WrapperHTTPRoute
	for idx, val := range input {
		list = append(list, &WrapperHTTPRoute{
			OriginPath:     val.path,
			OriginPathType: val.pathType,
			HTTPRoute: &networking.HTTPRoute{
				Name: origin[idx],
				Match: []*networking.HTTPMatchRequest{
					{
						Method:      val.method,
						Headers:     val.header,
						QueryParams: val.queryParam,
					},
				},
			},
		})
	}

	SortHTTPRoutes(list)

	for idx, val := range list {
		if val.HTTPRoute.Name != expect[idx] {
			t.Fatalf("should be %s, but got %s", expect[idx], val.HTTPRoute.Name)
		}
	}
}

func TestValidateBackendResource(t *testing.T) {
	groupStr := "networking.higress.io"
	testCases := []struct {
		name     string
		resource *v1.TypedLocalObjectReference
		expected bool
	}{
		{
			name:     "nil resource",
			resource: nil,
			expected: false,
		},
		{
			name: "nil APIGroup",
			resource: &v1.TypedLocalObjectReference{
				APIGroup: nil,
				Kind:     "McpBridge",
				Name:     "default",
			},
			expected: false,
		},
		{
			name: "wrong APIGroup",
			resource: &v1.TypedLocalObjectReference{
				APIGroup: &groupStr,
				Kind:     "McpBridge",
				Name:     "wrong-name",
			},
			expected: false,
		},
		{
			name: "wrong Kind",
			resource: &v1.TypedLocalObjectReference{
				APIGroup: &groupStr,
				Kind:     "WrongKind",
				Name:     "default",
			},
			expected: false,
		},
		{
			name: "valid resource",
			resource: &v1.TypedLocalObjectReference{
				APIGroup: &groupStr,
				Kind:     "McpBridge",
				Name:     "default",
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateBackendResource(tc.resource)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCreateOrUpdateAnnotations(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		options     Options
		expected    map[string]string
	}{
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			options: Options{
				ClusterId:    "test-cluster",
				RawClusterId: "raw-test-cluster",
			},
			expected: map[string]string{
				ClusterIdAnnotation:    "test-cluster",
				RawClusterIdAnnotation: "raw-test-cluster",
			},
		},
		{
			name: "existing annotations",
			annotations: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			options: Options{
				ClusterId:    "test-cluster",
				RawClusterId: "raw-test-cluster",
			},
			expected: map[string]string{
				"key1":                 "value1",
				"key2":                 "value2",
				ClusterIdAnnotation:    "test-cluster",
				RawClusterIdAnnotation: "raw-test-cluster",
			},
		},
		{
			name: "overwrite existing cluster annotations",
			annotations: map[string]string{
				ClusterIdAnnotation:    "old-cluster",
				RawClusterIdAnnotation: "old-raw-cluster",
				"key1":                 "value1",
			},
			options: Options{
				ClusterId:    "new-cluster",
				RawClusterId: "new-raw-cluster",
			},
			expected: map[string]string{
				ClusterIdAnnotation:    "new-cluster",
				RawClusterIdAnnotation: "new-raw-cluster",
				"key1":                 "value1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CreateOrUpdateAnnotations(tc.annotations, tc.options)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetClusterId(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		expected    string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    "",
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    "",
		},
		{
			name: "with cluster id",
			annotations: map[string]string{
				ClusterIdAnnotation: "test-cluster",
			},
			expected: "test-cluster",
		},
		{
			name: "with other annotations",
			annotations: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetClusterId(tc.annotations)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestConvertToDNSLabelValidAndCleanHost(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "simple host",
			input: "example.com",
		},
		{
			name:  "wildcard host",
			input: "*.example.com",
		},
		{
			name:  "long host",
			input: "very-long-subdomain.example-service.my-namespace.svc.cluster.local",
		},
		{
			name:  "empty host",
			input: "",
		},
		{
			name:  "ip address",
			input: "192.168.1.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test internal convertToDNSLabelValid function (through CleanHost)
			result := CleanHost(tc.input)

			// Validate result
			assert.NotEmpty(t, result)
			assert.Equal(t, 16, len(result)) // MD5 hash format is fixed length of 16 bytes

			// Consistency check - same input should produce same output
			result2 := CleanHost(tc.input)
			assert.Equal(t, result, result2)
		})
	}
}

func TestSplitServiceFQDN(t *testing.T) {
	testCases := []struct {
		name          string
		fqdn          string
		expectedSvc   string
		expectedNs    string
		expectedValid bool
	}{
		{
			name:          "simple fqdn",
			fqdn:          "service.namespace",
			expectedSvc:   "service",
			expectedNs:    "namespace",
			expectedValid: true,
		},
		{
			name:          "full k8s fqdn",
			fqdn:          "service.namespace.svc.cluster.local",
			expectedSvc:   "service",
			expectedNs:    "namespace",
			expectedValid: true,
		},
		{
			name:          "just service name",
			fqdn:          "service",
			expectedSvc:   "",
			expectedNs:    "",
			expectedValid: false,
		},
		{
			name:          "empty string",
			fqdn:          "",
			expectedSvc:   "",
			expectedNs:    "",
			expectedValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, ns, valid := SplitServiceFQDN(tc.fqdn)
			assert.Equal(t, tc.expectedSvc, svc)
			assert.Equal(t, tc.expectedNs, ns)
			assert.Equal(t, tc.expectedValid, valid)
		})
	}
}

func TestConvertBackendService(t *testing.T) {
	testCases := []struct {
		name     string
		dest     *networking.HTTPRouteDestination
		expected model.BackendService
	}{
		{
			name: "simple service",
			dest: &networking.HTTPRouteDestination{
				Destination: &networking.Destination{
					Host: "service.namespace",
					Port: &networking.PortSelector{
						Number: 80,
					},
				},
				Weight: 100,
			},
			expected: model.BackendService{
				Name:      "service",
				Namespace: "namespace",
				Port:      80,
				Weight:    100,
			},
		},
		{
			name: "full k8s FQDN",
			dest: &networking.HTTPRouteDestination{
				Destination: &networking.Destination{
					Host: "service.namespace.svc.cluster.local",
					Port: &networking.PortSelector{
						Number: 8080,
					},
				},
				Weight: 50,
			},
			expected: model.BackendService{
				Name:      "service",
				Namespace: "namespace",
				Port:      8080,
				Weight:    50,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertBackendService(tc.dest)
			assert.Equal(t, tc.expected.Name, result.Name)
			assert.Equal(t, tc.expected.Namespace, result.Namespace)
			assert.Equal(t, tc.expected.Port, result.Port)
			assert.Equal(t, tc.expected.Weight, result.Weight)
		})
	}
}

func TestCreateConvertedName(t *testing.T) {
	testCases := []struct {
		name     string
		items    []string
		expected string
	}{
		{
			name:     "empty slice",
			items:    []string{},
			expected: "",
		},
		{
			name:     "single item",
			items:    []string{"example"},
			expected: "example",
		},
		{
			name:     "multiple items",
			items:    []string{"part1", "part2", "part3"},
			expected: "part1-part2-part3",
		},
		{
			name:     "with empty strings",
			items:    []string{"part1", "", "part3"},
			expected: "part1-part3",
		},
		{
			name:     "all empty strings",
			items:    []string{"", "", ""},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CreateConvertedName(tc.items...)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSortIngressByCreationTime(t *testing.T) {
	configs := []config.Config{
		{
			Meta: config.Meta{
				Name:      "c-ingress",
				Namespace: "ns1",
			},
		},
		{
			Meta: config.Meta{
				Name:      "a-ingress",
				Namespace: "ns1",
			},
		},
		{
			Meta: config.Meta{
				Name:      "b-ingress",
				Namespace: "ns1",
			},
		},
	}

	expected := []string{"a-ingress", "b-ingress", "c-ingress"}

	SortIngressByCreationTime(configs)

	var actual []string
	for _, cfg := range configs {
		actual = append(actual, cfg.Name)
	}

	assert.Equal(t, expected, actual, "When the timestamps are the same, the configuration should be sorted by name")

	sameNamespaceConfigs := []config.Config{
		{
			Meta: config.Meta{
				Name:      "same-name",
				Namespace: "c-ns",
			},
		},
		{
			Meta: config.Meta{
				Name:      "same-name",
				Namespace: "a-ns",
			},
		},
		{
			Meta: config.Meta{
				Name:      "same-name",
				Namespace: "b-ns",
			},
		},
	}

	expectedNamespace := []string{"a-ns", "b-ns", "c-ns"}

	SortIngressByCreationTime(sameNamespaceConfigs)

	var actualNamespace []string
	for _, cfg := range sameNamespaceConfigs {
		actualNamespace = append(actualNamespace, cfg.Namespace)
	}

	assert.Equal(t, expectedNamespace, actualNamespace, "When the names are the same, the configuration should be sorted by namespace")

	mixedConfigs := []config.Config{
		{
			Meta: config.Meta{
				Name:      "z-ingress",
				Namespace: "a-ns",
			},
		},
		{
			Meta: config.Meta{
				Name:      "a-ingress",
				Namespace: "b-ns",
			},
		},
	}

	expectedNamespacedNames := []string{"a-ns/z-ingress", "b-ns/a-ingress"}

	SortIngressByCreationTime(mixedConfigs)

	var actualNamespacedNames []string
	for _, cfg := range mixedConfigs {
		actualNamespacedNames = append(actualNamespacedNames, cfg.Namespace+"/"+cfg.Name)
	}

	assert.Equal(t, expectedNamespacedNames, actualNamespacedNames, "When timestamps are the same, configurations should be sorted by namespace before name")

	// When timestamps are equal and canary names share the base name as a
	// prefix (e.g. "xx-hg" and "xx-hg-canary-*"), namespace-then-name ordering
	// places the base ingress first. This is a lexicographic consequence, not
	// active canary detection (see the non-prefixed counter-example below).
	prefixedCanaryConfigs := []config.Config{
		{
			Meta: config.Meta{
				Name:      "xx-hg-canary-by-header",
				Namespace: "default",
			},
		},
		{
			Meta: config.Meta{
				Name:      "xx-hg",
				Namespace: "default",
			},
		},
		{
			Meta: config.Meta{
				Name:      "xx-hg-canary-by-weight",
				Namespace: "default",
			},
		},
	}

	expectedPrefixedCanary := []string{"xx-hg", "xx-hg-canary-by-header", "xx-hg-canary-by-weight"}

	SortIngressByCreationTime(prefixedCanaryConfigs)

	var actualPrefixedCanary []string
	for _, cfg := range prefixedCanaryConfigs {
		actualPrefixedCanary = append(actualPrefixedCanary, cfg.Name)
	}

	assert.Equal(t, expectedPrefixedCanary, actualPrefixedCanary, "When canary names share the base prefix, the base ingress sorts first as a lexicographic consequence (not active canary detection)")

	// Counter-example: the sorter does NOT actively identify canary ingresses.
	// Canary status is determined by annotations, not by name. When a canary
	// ingress is named without the base name as a prefix and its name sorts
	// before the base lexicographically, it will NOT be ordered after the base.
	nonPrefixedCanaryConfigs := []config.Config{
		{
			Meta: config.Meta{
				Name:      "xx-hg",
				Namespace: "default",
			},
		},
		{
			Meta: config.Meta{
				Name:      "canary-xx-hg-by-header",
				Namespace: "default",
			},
		},
		{
			Meta: config.Meta{
				Name:      "canary-xx-hg-by-weight",
				Namespace: "default",
			},
		},
	}

	expectedNonPrefixedCanary := []string{"canary-xx-hg-by-header", "canary-xx-hg-by-weight", "xx-hg"}

	SortIngressByCreationTime(nonPrefixedCanaryConfigs)

	var actualNonPrefixedCanary []string
	for _, cfg := range nonPrefixedCanaryConfigs {
		actualNonPrefixedCanary = append(actualNonPrefixedCanary, cfg.Name)
	}

	assert.Equal(t, expectedNonPrefixedCanary, actualNonPrefixedCanary, "Without a shared name prefix, canary ingresses are not recognized and may sort before the base")
}

func TestPartMd5(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		length int
	}{
		{
			name:   "empty string",
			input:  "",
			length: 8,
		},
		{
			name:   "simple string",
			input:  "test",
			length: 8,
		},
		{
			name:   "complex string",
			input:  "this-is-a-long-string-with-special-chars-!@#$%^&*()",
			length: 8,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := partMd5(tc.input)

			// Check result format
			assert.Equal(t, tc.length, len(result), "MD5 hash excerpt should be 8 characters")

			// Run twice to ensure deterministic output
			result2 := partMd5(tc.input)
			assert.Equal(t, result, result2, "partMd5 function should be deterministic")
		})
	}
}

func TestGetLbStatusListV1AndV1Beta1(t *testing.T) {
	clusterPrefix = "gw-123-"
	svcName := clusterPrefix
	aliyunHostname := "higress.cn-hangzhou.alb.aliyuncs.com"
	awsHostname := "k8s-kubeingr-higressg-1234567890.eu-north-1.elb.amazonaws.com"
	tencentHostname := "lb-12345678.clb.ap-guangzhou.tencentclb.com"
	svcList := []*v1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							IP: "2.2.2.2",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: "1.1.1.1" + SvcHostNameSuffix,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: awsHostname,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: aliyunHostname,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: svcName,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
			Status: v1.ServiceStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{
							Hostname: tencentHostname,
						},
					},
				},
			},
		},
	}

	// Test the V1 version
	t.Run("GetLbStatusListV1", func(t *testing.T) {
		lbiList := GetLbStatusListV1(svcList)

		assert.Equal(t, []networkingv1.IngressLoadBalancerIngress{
			{IP: "1.1.1.1"},
			{IP: "2.2.2.2"},
			{Hostname: aliyunHostname},
			{Hostname: awsHostname},
			{Hostname: tencentHostname},
		}, lbiList)
	})

	// Test the V1Beta1 version
	t.Run("GetLbStatusListV1Beta1", func(t *testing.T) {
		lbiList := GetLbStatusListV1Beta1(svcList)

		assert.Equal(t, []networkingv1beta1.IngressLoadBalancerIngress{
			{IP: "1.1.1.1"},
			{IP: "2.2.2.2"},
			{Hostname: aliyunHostname},
			{Hostname: awsHostname},
			{Hostname: tencentHostname},
		}, lbiList)
	})
}


func makeSortRoute(path string, pathType PathType, isDefault bool, match *networking.HTTPMatchRequest) *WrapperHTTPRoute {
	r := &WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Match: nil,
		},
		WrapperConfig: &WrapperConfig{
			Config: &config.Config{},
		},
		OriginPath:       path,
		OriginPathType:   pathType,
		IsDefaultBackend: isDefault,
	}
	if match != nil {
		r.HTTPRoute.Match = []*networking.HTTPMatchRequest{match}
	}
	return r
}

func TestSortHTTPRoutes_EmptyMatchSlice(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/api/v1", Exact, false, nil),
		makeSortRoute("/", Prefix, false, &networking.HTTPMatchRequest{}),
	}
	routes[0].HTTPRoute.Match = []*networking.HTTPMatchRequest{}

	SortHTTPRoutes(routes)

	if routes[0].OriginPath != "/api/v1" {
		t.Errorf("expected /api/v1 first, got %s", routes[0].OriginPath)
	}
}

func TestSortHTTPRoutes_NilMatchSlice(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/api/v1", Exact, false, &networking.HTTPMatchRequest{}),
		makeSortRoute("/", Prefix, false, nil),
	}

	SortHTTPRoutes(routes)

	if routes[0].OriginPath != "/api/v1" {
		t.Errorf("expected /api/v1 first, got %s", routes[0].OriginPath)
	}
}

func TestSortHTTPRoutes_DefaultBackendLast(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("", "", true, nil),
		makeSortRoute("/api", Prefix, false, &networking.HTTPMatchRequest{}),
	}

	SortHTTPRoutes(routes)

	if !routes[1].IsDefaultBackend {
		t.Errorf("expected default backend last")
	}
}

func TestSortHTTPRoutes_AllCatchLast(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/", Prefix, false, nil),
		makeSortRoute("/api/v1", Prefix, false, &networking.HTTPMatchRequest{}),
	}

	SortHTTPRoutes(routes)

	if routes[0].OriginPath != "/api/v1" {
		t.Errorf("expected /api/v1 first, got %s", routes[0].OriginPath)
	}
}

func TestSortHTTPRoutes_ExactBeforePrefix(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/api", Prefix, false, &networking.HTTPMatchRequest{}),
		makeSortRoute("/api", Exact, false, &networking.HTTPMatchRequest{}),
	}

	SortHTTPRoutes(routes)

	if routes[0].OriginPathType != Exact {
		t.Errorf("expected Exact first, got %s", routes[0].OriginPathType)
	}
}

func TestSortHTTPRoutes_LongerPathFirst(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/api", Prefix, false, &networking.HTTPMatchRequest{}),
		makeSortRoute("/api/v1/users", Prefix, false, &networking.HTTPMatchRequest{}),
	}

	SortHTTPRoutes(routes)

	if routes[0].OriginPath != "/api/v1/users" {
		t.Errorf("expected /api/v1/users first, got %s", routes[0].OriginPath)
	}
}

func TestSortHTTPRoutes_EmptyVsNonEmptyMatch(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("/api", Prefix, false, nil),
		makeSortRoute("/api", Prefix, false, &networking.HTTPMatchRequest{}),
	}
	routes[0].HTTPRoute.Match = []*networking.HTTPMatchRequest{}

	SortHTTPRoutes(routes)

	// Route with match should come before route with empty match
	if len(routes[0].HTTPRoute.Match) == 0 {
		t.Errorf("expected route with match first")
	}
}

func TestSortHTTPRoutes_NoPanic(t *testing.T) {
	routes := []*WrapperHTTPRoute{
		makeSortRoute("", "", true, nil),
		makeSortRoute("/", Prefix, false, nil),
		makeSortRoute("/api", Prefix, false, &networking.HTTPMatchRequest{}),
		makeSortRoute("/api/exact", Exact, false, &networking.HTTPMatchRequest{}),
	}
	routes[1].HTTPRoute.Match = []*networking.HTTPMatchRequest{}

	SortHTTPRoutes(routes)

	if !routes[len(routes)-1].IsDefaultBackend {
		t.Errorf("expected default backend last")
	}
}
