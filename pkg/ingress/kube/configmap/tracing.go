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

package configmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"

	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	networking "istio.io/api/networking/v1alpha3"
)

const (
	higressTracingEnvoyFilterName = "higress-config-tracing"
	defaultTimeout                = 500
	defaultSampling               = 100.0
)

type Tracing struct {
	// Flag to control trace
	Enable bool `json:"enable,omitempty"`
	// The percentage of requests (0.0 - 100.0) that will be randomly selected for trace generation,
	// if not requested by the client or not forced. Default is 100.0.
	Sampling float64 `json:"sampling,omitempty"`
	// The timeout for the gRPC request. Default is 500ms
	Timeout int32 `json:"timeout,omitempty"`
	// The tracer implementation to be used by Envoy.
	//
	// Types that are assignable to Tracer:
	Zipkin        *Zipkin        `json:"zipkin,omitempty"`
	Skywalking    *Skywalking    `json:"skywalking,omitempty"`
	OpenTelemetry *OpenTelemetry `json:"opentelemetry,omitempty"`
}

// Zipkin defines configuration for a Zipkin tracer.
type Zipkin struct {
	// Address of the Zipkin service (e.g. _zipkin:9411_).
	Service string `json:"service,omitempty"`
	Port    string `json:"port,omitempty"`
}

// Skywalking Defines configuration for a Skywalking tracer.
type Skywalking struct {
	// Address of the Skywalking tracer.
	Service string `json:"service,omitempty"`
	Port    string `json:"port,omitempty"`
	// The access token
	AccessToken string `json:"access_token,omitempty"`
}

// OpenTelemetry Defines configuration for a OpenTelemetry tracer.
type OpenTelemetry struct {
	// Address of OpenTelemetry tracer.
	Service string `json:"service,omitempty"`
	Port    string `json:"port,omitempty"`
}

func validServiceAndPort(service string, port string) bool {
	if len(service) == 0 || len(port) == 0 {
		return false
	}
	return true
}

func validTracing(t *Tracing) error {
	if t == nil {
		return nil
	}
	if t.Timeout <= 0 {
		return errors.New("timeout can not be less than zero")
	}

	if t.Sampling < 0 || t.Sampling > 100 {
		return errors.New("sampling must be in (0.0 - 100.0)")
	}

	tracerNum := 0
	if t.Zipkin != nil {
		if validServiceAndPort(t.Zipkin.Service, t.Zipkin.Port) {
			tracerNum++
		} else {
			return errors.New("zipkin service and port can not be empty")
		}
	}

	if t.Skywalking != nil {
		if validServiceAndPort(t.Skywalking.Service, t.Skywalking.Port) {
			tracerNum++
		} else {
			return errors.New("skywalking service and port can not be empty")
		}
	}

	if t.OpenTelemetry != nil {
		if validServiceAndPort(t.OpenTelemetry.Service, t.OpenTelemetry.Port) {
			tracerNum++
		} else {
			return errors.New("opentelemetry service and port can not be empty")
		}
	}

	if tracerNum != 1 && t.Enable == true {
		return errors.New("only one of skywalking，zipkin and opentelemetry configuration can be set")
	}
	return nil
}

func compareTracing(old *Tracing, new *Tracing) (Result, error) {
	if old == nil && new == nil {
		return ResultNothing, nil
	}

	if new == nil {
		return ResultDelete, nil
	}

	if !reflect.DeepEqual(old, new) {
		return ResultReplace, nil
	}

	return ResultNothing, nil
}

func deepCopyTracing(tracing *Tracing) (*Tracing, error) {
	newTracing := NewDefaultTracing()
	bytes, err := json.Marshal(tracing)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, newTracing)
	return newTracing, err
}

func NewDefaultTracing() *Tracing {
	tracing := &Tracing{
		Enable:   false,
		Timeout:  defaultTimeout,
		Sampling: defaultSampling,
	}
	return tracing
}

type TracingController struct {
	Namespace    string
	tracing      atomic.Value
	Name         string
	eventHandler ItemEventHandler
}

func NewTracingController(namespace string) *TracingController {
	tracingMgr := &TracingController{
		Namespace: namespace,
		tracing:   atomic.Value{},
		Name:      "tracing",
	}
	tracingMgr.SetTracing(NewDefaultTracing())
	return tracingMgr
}

func (t *TracingController) SetTracing(tracing *Tracing) {
	t.tracing.Store(tracing)
}

func (t *TracingController) GetTracing() *Tracing {
	value := t.tracing.Load()
	if value != nil {
		if tracing, ok := value.(*Tracing); ok {
			return tracing
		}
	}
	return nil
}

func (t *TracingController) GetName() string {
	return t.Name
}

func (t *TracingController) AddOrUpdateHigressConfig(name util.ClusterNamespacedName, old *HigressConfig, new *HigressConfig) error {
	if err := validTracing(new.Tracing); err != nil {
		IngressLog.Errorf("data:%+v convert to tracing , error: %+v", new.Tracing, err)
		return nil
	}

	result, _ := compareTracing(old.Tracing, new.Tracing)

	switch result {
	case ResultReplace:
		if newTracing, err := deepCopyTracing(new.Tracing); err != nil {
			IngressLog.Infof("tracing deepcopy error:%v", err)
		} else {
			t.SetTracing(newTracing)
			IngressLog.Infof("AddOrUpdate Higress config tracing")
			t.eventHandler(higressTracingEnvoyFilterName)
			IngressLog.Infof("send event with filter name:%s", higressTracingEnvoyFilterName)
		}
	case ResultDelete:
		t.SetTracing(NewDefaultTracing())
		IngressLog.Infof("Delete Higress config tracing")
		t.eventHandler(higressTracingEnvoyFilterName)
		IngressLog.Infof("send event with filter name:%s", higressTracingEnvoyFilterName)
	}

	return nil
}

func (t *TracingController) ValidHigressConfig(higressConfig *HigressConfig) error {
	if higressConfig == nil {
		return nil
	}
	if higressConfig.Tracing == nil {
		return nil
	}

	return validTracing(higressConfig.Tracing)
}

func (t *TracingController) RegisterItemEventHandler(eventHandler ItemEventHandler) {
	t.eventHandler = eventHandler
}

func (t *TracingController) ConstructEnvoyFilters() ([]*config.Config, error) {
	configs := make([]*config.Config, 0)
	tracing := t.GetTracing()
	namespace := t.Namespace

	if tracing == nil {
		return configs, nil
	}

	if tracing.Enable == false {
		return configs, nil
	}

	tracingConfig := t.constructTracingTracer(tracing, namespace)
	if len(tracingConfig) == 0 {
		return configs, nil
	}

	tracerPatchValue, err := util.BuildPatchStruct(tracingConfig)
	if err != nil {
		return nil, err
	}

	// Internal-only static patch: the JSON is a compile-time constant, so an
	// error here indicates a programming mistake rather than bad user input.
	routerPatchValue, err := util.BuildPatchStruct(`{
				"name":"envoy.filters.http.router",
				"typed_config":{
					"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
					"start_child_span": true
				}
			}`)
	if err != nil {
		return nil, err
	}

	configPatches := []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
		{
			ApplyTo: networking.EnvoyFilter_NETWORK_FILTER,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
				ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
					Listener: &networking.EnvoyFilter_ListenerMatch{
						FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
							Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
								Name: "envoy.filters.network.http_connection_manager",
							},
						},
					},
				},
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     tracerPatchValue,
			},
		},
		{
			ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
				ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
					Listener: &networking.EnvoyFilter_ListenerMatch{
						FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
							Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
								Name: "envoy.filters.network.http_connection_manager",
								SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
									Name: "envoy.filters.http.router",
								},
							},
						},
					},
				},
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     routerPatchValue,
			},
		},
	}

	patches, err := t.constructTracingExtendPatches(tracing)
	if err != nil {
		return nil, err
	}
	configPatches = append(configPatches, patches...)

	config := &config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.EnvoyFilter,
			Name:             higressTracingEnvoyFilterName,
			Namespace:        namespace,
		},
		Spec: &networking.EnvoyFilter{
			ConfigPatches: configPatches,
		},
	}

	configs = append(configs, config)
	return configs, nil
}

func tracingClusterName(port, service string) string {
	return fmt.Sprintf("outbound|%s||%s", port, service)
}

func (t *TracingController) constructHTTP2ProtocolOptionsPatch(port, service string) (*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	// Internal-only static patch: the JSON is a compile-time constant, so an
	// error here indicates a programming mistake rather than bad user input.
	http2ProtocolOptions := `{"typed_extension_protocol_options": {
  "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
      "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
		  "explicit_http_config": {
		        "http2_protocol_options": {}
		  }
  }
}}`

	http2PatchValue, err := util.BuildPatchStruct(http2ProtocolOptions)
	if err != nil {
		return nil, err
	}

	return &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_CLUSTER,
		Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: networking.EnvoyFilter_GATEWAY,
			ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Cluster{
				Cluster: &networking.EnvoyFilter_ClusterMatch{
					Name: tracingClusterName(port, service),
				},
			},
		},
		Patch: &networking.EnvoyFilter_Patch{
			Operation: networking.EnvoyFilter_Patch_MERGE,
			Value:     http2PatchValue,
		},
	}, nil
}

func (t *TracingController) constructTracingExtendPatches(tracing *Tracing) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	if tracing == nil {
		return nil, nil
	}
	var patches []*networking.EnvoyFilter_EnvoyConfigObjectPatch
	if skywalking := tracing.Skywalking; skywalking != nil {
		patch, err := t.constructHTTP2ProtocolOptionsPatch(skywalking.Port, skywalking.Service)
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch)
	}
	if otel := tracing.OpenTelemetry; otel != nil {
		patch, err := t.constructHTTP2ProtocolOptionsPatch(otel.Port, otel.Service)
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch)
	}

	return patches, nil
}

func (t *TracingController) constructTracingTracer(tracing *Tracing, namespace string) string {
	tracingConfig := ""
	timeout := float32(tracing.Timeout) / 1000
	if tracing.Skywalking != nil {
		skywalking := tracing.Skywalking
		tracingConfig = fmt.Sprintf(`{
	"name": "envoy.filters.network.http_connection_manager",
	"typed_config": {
		"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
		"tracing": {
			"provider": {
				"name": "envoy.tracers.skywalking",
				"typed_config": {
					"@type": "type.googleapis.com/envoy.config.trace.v3.SkyWalkingConfig",
					"client_config": {
						"service_name": "higress-gateway.%s",
                                                "backend_token": "%s"
					},
					"grpc_service": {
						"envoy_grpc": {
							"cluster_name": "%s"
						},
						"timeout": "%.3fs"
					}
				}
			},
			"random_sampling": {
				"value": %.1f
			}
		}
	}
}`, namespace, skywalking.AccessToken, tracingClusterName(skywalking.Port, skywalking.Service), timeout, tracing.Sampling)
	}

	if tracing.Zipkin != nil {
		zipkin := tracing.Zipkin
		tracingConfig = fmt.Sprintf(`{
	"name": "envoy.filters.network.http_connection_manager",
	"typed_config": {
		"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
		"tracing": {
			"provider": {
				"name": "envoy.tracers.zipkin",
				"typed_config": {
					"@type": "type.googleapis.com/envoy.config.trace.v3.ZipkinConfig",
                                        "collector_cluster": "%s",
                                        "collector_endpoint": "/api/v2/spans",
                                        "collector_hostname": "higress-gateway",
                                        "collector_endpoint_version": "HTTP_JSON",
                                        "split_spans_for_request": true
				}
			},
			"random_sampling": {
				"value": %.1f
			}
		}
	}
}`, tracingClusterName(zipkin.Port, zipkin.Service), tracing.Sampling)
	}

	if tracing.OpenTelemetry != nil {
		opentelemetry := tracing.OpenTelemetry
		tracingConfig = fmt.Sprintf(`{
	"name": "envoy.filters.network.http_connection_manager",
	"typed_config": {
		"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
		"tracing": {
			"provider": {
				"name": "envoy.tracers.opentelemetry",
				"typed_config": {
					"@type": "type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig",
					"service_name": "higress-gateway.%s",
					"grpc_service": {
						"envoy_grpc": {
							"cluster_name": "%s"
						},
						"timeout": "%.3fs"
					}
				}
			},
			"random_sampling": {
				"value": %.1f
			}
		}
	}
}`, namespace, tracingClusterName(opentelemetry.Port, opentelemetry.Service), timeout, tracing.Sampling)
	}
	return tracingConfig
}
