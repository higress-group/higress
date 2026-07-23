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

	"github.com/alibaba/higress/v2/pkg/ingress/kube/configmap"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/envoy"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/kubernetes"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

var ConfigmapTracing = suite.ConformanceTest{
	ShortName:   "ConfigmapTracing",
	Description: "The Ingress in the higress-conformance-infra namespace uses the configmap tracing.",
	Manifests:   []string{"tests/configmap-tracing.yaml"},
	Features:    []suite.SupportedFeature{suite.EnvoyConfigConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		t.Run("Configmap Tracing", func(t *testing.T) {
			for _, testcase := range tracingTestCases {
				err := kubernetes.ApplyConfigmapDataWithYaml(t, suite.Client, "higress-system", "higress-config", "higress", testcase.higressConfig)
				if err != nil {
					t.Fatalf("can't apply configmap %s in namespace %s for data key %s", "higress-config", "higress-system", "higress")
				}
				envoy.AssertEnvoyConfig(t, suite.TimeoutConfig, testcase.envoyAssertion)
			}
		})
	},
}

var tracingTestCases = []struct {
	name           string
	higressConfig  *configmap.HigressConfig
	envoyAssertion envoy.Assertion
}{
	{
		name: "tracing disabled",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable: false,
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:              "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing",
			TargetNamespace:   "higress-system",
			CheckType:         envoy.CheckTypeNotExist,
			ExpectEnvoyConfig: map[string]interface{}{},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:   true,
				Sampling: 100.0,
				Timeout:  500,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.provider",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"name": "envoy.tracers.opentelemetry",
				"typed_config": map[string]interface{}{
					"@type":        "type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig",
					"service_name": "higress-gateway.higress-system",
					"grpc_service": map[string]interface{}{
						"envoy_grpc": map[string]interface{}{
							"cluster_name": "outbound|4317||otel-collector",
						},
						"timeout": "0.500s",
					},
				},
			},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer with customTag literal",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:   true,
				Sampling: 100.0,
				Timeout:  500,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
				CustomTag: []configmap.CustomTag{
					{
						Tag:     "custom-literal",
						Literal: "literal-value",
					},
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.custom_tags",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"tag": "custom-literal",
				"literal": map[string]interface{}{
					"value": "literal-value",
				},
			},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer with customTag environment",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:   true,
				Sampling: 100.0,
				Timeout:  500,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
				CustomTag: []configmap.CustomTag{
					{
						Tag: "custom-env",
						Environment: &configmap.CustomTagValue{
							Key:          "ENV_KEY",
							DefaultValue: "env-default",
						},
					},
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.custom_tags",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"tag": "custom-env",
				"environment": map[string]interface{}{
					"name":          "ENV_KEY",
					"default_value": "env-default",
				},
			},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer with customTag requestHeader",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:   true,
				Sampling: 100.0,
				Timeout:  500,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
				CustomTag: []configmap.CustomTag{
					{
						Tag: "custom-header",
						RequestHeader: &configmap.CustomTagValue{
							Key:          "X-My-Header",
							DefaultValue: "header-default",
						},
					},
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.custom_tags",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"tag": "custom-header",
				"request_header": map[string]interface{}{
					"name":          "X-My-Header",
					"default_value": "header-default",
				},
			},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer with sampling 50",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:   true,
				Sampling: 50.0,
				Timeout:  501,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.random_sampling",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"value": 50.0,
			},
		},
	},
	{
		name: "tracing enabled: OpenTelemetry tracer with timeout 1000",
		higressConfig: &configmap.HigressConfig{
			Tracing: &configmap.Tracing{
				Enable:  true,
				Timeout: 1000,
				OpenTelemetry: &configmap.OpenTelemetry{
					Service: "otel-collector",
					Port:    "4317",
				},
			},
		},
		envoyAssertion: envoy.Assertion{
			Path:            "configs.#.dynamic_listeners.#.active_state.listener.filter_chains.#.filters.#.typed_config.tracing.provider",
			TargetNamespace: "higress-system",
			CheckType:       envoy.CheckTypeExist,
			ExpectEnvoyConfig: map[string]interface{}{
				"name": "envoy.tracers.opentelemetry",
				"typed_config": map[string]interface{}{
					"@type":        "type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig",
					"service_name": "higress-gateway.higress-system",
					"grpc_service": map[string]interface{}{
						"envoy_grpc": map[string]interface{}{
							"cluster_name": "outbound|4317||otel-collector",
						},
						"timeout": "1s",
					},
				},
			},
		},
	},
}

func init() {
	Register(ConfigmapTracing)
}
