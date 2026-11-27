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
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
)

const (
	higressGlobalEnvoyFilterName = "global-option"

	maxMaxRequestHeadersKb         = 8192
	minMaxConcurrentStreams        = 1
	maxMaxConcurrentStreams        = 2147483647
	minInitialStreamWindowSize     = 65535
	maxInitialStreamWindowSize     = 2147483647
	minInitialConnectionWindowSize = 65535
	maxInitialConnectionWindowSize = 2147483647

	defaultIdleTimeout                    = 180
	defaultRouteTimeout                   = 0
	defaultUpStreamIdleTimeout            = 10
	defaultUpStreamConnectionBufferLimits = 10485760
	defaultMaxRequestHeadersKb            = 60
	defaultConnectionBufferLimits         = 32768
	defaultMaxConcurrentStreams           = 100
	defaultInitialStreamWindowSize        = 65535
	defaultInitialConnectionWindowSize    = 1048576
	defaultAddXRealIpHeader               = false
	defaultDisableXEnvoyHeaders           = false
)

// Global configures the behavior of the downstream connection, x-real-ip header and x-envoy headers.
type Global struct {
	Downstream           *Downstream `json:"downstream,omitempty"`
	Upstream             *Upstream   `json:"upstream,omitempty"`
	AddXRealIpHeader     bool        `json:"addXRealIpHeader,omitempty"`
	DisableXEnvoyHeaders bool        `json:"disableXEnvoyHeaders,omitempty"`
}

// Downstream configures the behavior of the downstream connection.
type Downstream struct {
	// IdleTimeout limits the time that a connection may be idle and stream idle.
	IdleTimeout uint32 `json:"idleTimeout"`
	// MaxRequestHeadersKb limits the size of request headers allowed.
	MaxRequestHeadersKb uint32 `json:"maxRequestHeadersKb,omitempty"`
	// ConnectionBufferLimits configures the buffer size limits for connections.
	ConnectionBufferLimits uint32 `json:"connectionBufferLimits,omitempty"`
	// Http2 configures HTTP/2 specific options.
	Http2 *Http2 `json:"http2,omitempty"`
	// RouteTimeout limits the time that timeout for the route.
	RouteTimeout uint32 `json:"routeTimeout"`
}

// Upstream configures the behavior of the upstream connection.
type Upstream struct {
	// IdleTimeout limits the time that a connection may be idle on the upstream.
	IdleTimeout uint32 `json:"idleTimeout"`
	// ConnectionBufferLimits configures the buffer size limits for connections.
	ConnectionBufferLimits uint32 `json:"connectionBufferLimits,omitempty"`
}

// Http2 configures HTTP/2 specific options.
type Http2 struct {
	// MaxConcurrentStreams limits the number of concurrent streams allowed.
	MaxConcurrentStreams uint32 `json:"maxConcurrentStreams,omitempty"`
	// InitialStreamWindowSize limits the initial window size of stream.
	InitialStreamWindowSize uint32 `json:"initialStreamWindowSize,omitempty"`
	// InitialConnectionWindowSize limits the initial window size of connection.
	InitialConnectionWindowSize uint32 `json:"initialConnectionWindowSize,omitempty"`
}

// validGlobal validates the global config.
func validGlobal(global *Global) error {
	if global == nil {
		return nil
	}

	if global.Downstream == nil {
		return nil
	}

	downStream := global.Downstream

	// check maxRequestHeadersKb
	if downStream.MaxRequestHeadersKb > maxMaxRequestHeadersKb {
		return fmt.Errorf("maxRequestHeadersKb must be less than or equal to 8192")
	}
	// check http2
	if downStream.Http2 != nil {
		// check maxConcurrentStreams
		if downStream.Http2.MaxConcurrentStreams < minMaxConcurrentStreams ||
			downStream.Http2.MaxConcurrentStreams > maxMaxConcurrentStreams {
			return fmt.Errorf("http2.maxConcurrentStreams must be between 1 and 2147483647")
		}
		// check initialStreamWindowSize
		if downStream.Http2.InitialStreamWindowSize < minInitialStreamWindowSize ||
			downStream.Http2.InitialStreamWindowSize > maxInitialStreamWindowSize {
			return fmt.Errorf("http2.initialStreamWindowSize must be between 65535 and 2147483647")
		}
		// check initialConnectionWindowSize
		if downStream.Http2.InitialConnectionWindowSize < minInitialConnectionWindowSize ||
			downStream.Http2.InitialConnectionWindowSize > maxInitialConnectionWindowSize {
			return fmt.Errorf("http2.initialConnectionWindowSize must be between 65535 and 2147483647")
		}
	}

	return nil
}

// compareGlobal compares the old and new global option.
func compareGlobal(old *Global, new *Global) (Result, error) {
	if old == nil && new == nil {
		return ResultNothing, nil
	}

	if new == nil {
		return ResultDelete, nil
	}

	if new.Downstream == nil && new.Upstream == nil && !new.AddXRealIpHeader && !new.DisableXEnvoyHeaders {
		return ResultDelete, nil
	}

	if !reflect.DeepEqual(old, new) {
		return ResultReplace, nil
	}

	return ResultNothing, nil
}

// deepCopyGlobal deep copies the global option.
func deepCopyGlobal(global *Global) (*Global, error) {
	newGlobal := NewDefaultGlobalOption()
	if global.Downstream != nil {
		newGlobal.Downstream.IdleTimeout = global.Downstream.IdleTimeout
		newGlobal.Downstream.MaxRequestHeadersKb = global.Downstream.MaxRequestHeadersKb
		newGlobal.Downstream.ConnectionBufferLimits = global.Downstream.ConnectionBufferLimits
		if global.Downstream.Http2 != nil {
			newGlobal.Downstream.Http2.MaxConcurrentStreams = global.Downstream.Http2.MaxConcurrentStreams
			newGlobal.Downstream.Http2.InitialStreamWindowSize = global.Downstream.Http2.InitialStreamWindowSize
			newGlobal.Downstream.Http2.InitialConnectionWindowSize = global.Downstream.Http2.InitialConnectionWindowSize
		}
		newGlobal.Downstream.RouteTimeout = global.Downstream.RouteTimeout
	}
	if global.Upstream != nil {
		newGlobal.Upstream.IdleTimeout = global.Upstream.IdleTimeout
		newGlobal.Upstream.ConnectionBufferLimits = global.Upstream.ConnectionBufferLimits
	}
	newGlobal.AddXRealIpHeader = global.AddXRealIpHeader
	newGlobal.DisableXEnvoyHeaders = global.DisableXEnvoyHeaders
	return newGlobal, nil
}

// NewDefaultGlobalOption returns a default global config.
func NewDefaultGlobalOption() *Global {
	return &Global{
		Downstream:           NewDefaultDownstream(),
		Upstream:             NewDefaultUpStream(),
		AddXRealIpHeader:     defaultAddXRealIpHeader,
		DisableXEnvoyHeaders: defaultDisableXEnvoyHeaders,
	}
}

// NewDefaultDownstream returns a default downstream config.
func NewDefaultDownstream() *Downstream {
	return &Downstream{
		IdleTimeout:            defaultIdleTimeout,
		MaxRequestHeadersKb:    defaultMaxRequestHeadersKb,
		ConnectionBufferLimits: defaultConnectionBufferLimits,
		Http2:                  NewDefaultHttp2(),
		RouteTimeout:           defaultRouteTimeout,
	}
}

// NewDefaultUpStream returns a default upstream config.
func NewDefaultUpStream() *Upstream {
	return &Upstream{
		IdleTimeout:            defaultUpStreamIdleTimeout,
		ConnectionBufferLimits: defaultUpStreamConnectionBufferLimits,
	}
}

// NewDefaultHttp2 returns a default http2 config.
func NewDefaultHttp2() *Http2 {
	return &Http2{
		MaxConcurrentStreams:        defaultMaxConcurrentStreams,
		InitialStreamWindowSize:     defaultInitialStreamWindowSize,
		InitialConnectionWindowSize: defaultInitialConnectionWindowSize,
	}
}

// GlobalOptionController is the controller of downstream config.
type GlobalOptionController struct {
	Namespace    string
	global       atomic.Value
	Name         string
	eventHandler ItemEventHandler
}

// NewGlobalOptionController returns a GlobalOptionController.
func NewGlobalOptionController(namespace string) *GlobalOptionController {
	globalOptionController := &GlobalOptionController{
		Namespace: namespace,
		global:    atomic.Value{},
		Name:      "global-option",
	}
	globalOptionController.SetGlobal(NewDefaultGlobalOption())
	return globalOptionController
}

func (g *GlobalOptionController) SetGlobal(global *Global) {
	g.global.Store(global)
}

func (g *GlobalOptionController) GetGlobal() *Global {
	value := g.global.Load()
	if value != nil {
		if global, ok := value.(*Global); ok {
			return global
		}
	}
	return nil
}

func (g *GlobalOptionController) GetName() string {
	return g.Name
}

func (g *GlobalOptionController) AddOrUpdateHigressConfig(name util.ClusterNamespacedName, old *HigressConfig, new *HigressConfig) error {
	newGlobal := &Global{
		Downstream:           new.Downstream,
		Upstream:             new.Upstream,
		AddXRealIpHeader:     new.AddXRealIpHeader,
		DisableXEnvoyHeaders: new.DisableXEnvoyHeaders,
	}

	oldGlobal := &Global{
		Downstream:           old.Downstream,
		Upstream:             old.Upstream,
		AddXRealIpHeader:     old.AddXRealIpHeader,
		DisableXEnvoyHeaders: old.DisableXEnvoyHeaders,
	}

	err := validGlobal(newGlobal)
	if err != nil {
		IngressLog.Errorf("data:%+v convert to global-option config error, error: %+v", newGlobal, err)
		return nil
	}

	result, _ := compareGlobal(oldGlobal, newGlobal)

	switch result {
	case ResultReplace:
		if newGlobalCopy, err := deepCopyGlobal(newGlobal); err != nil {
			IngressLog.Infof("global-option config deepcopy error:%v", err)
		} else {
			g.SetGlobal(newGlobalCopy)
			IngressLog.Infof("AddOrUpdate Higress config global-option")
			g.eventHandler(higressGlobalEnvoyFilterName)
			IngressLog.Infof("send event with filter name:%s", higressGlobalEnvoyFilterName)
		}
	case ResultDelete:
		g.SetGlobal(NewDefaultGlobalOption())
		IngressLog.Infof("Delete Higress config global-option")
		g.eventHandler(higressGlobalEnvoyFilterName)
		IngressLog.Infof("send event with filter name:%s", higressGlobalEnvoyFilterName)
	}

	return nil
}

func (g *GlobalOptionController) ValidHigressConfig(higressConfig *HigressConfig) error {
	if higressConfig == nil {
		return nil
	}

	if higressConfig.Downstream == nil {
		return nil
	}

	global := &Global{
		Downstream:           higressConfig.Downstream,
		Upstream:             higressConfig.Upstream,
		AddXRealIpHeader:     higressConfig.AddXRealIpHeader,
		DisableXEnvoyHeaders: higressConfig.DisableXEnvoyHeaders,
	}

	return validGlobal(global)
}

func (g *GlobalOptionController) ConstructEnvoyFilters() ([]*config.Config, error) {
	configPatch := make([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	global := g.GetGlobal()
	if global == nil {
		return []*config.Config{}, nil
	}

	namespace := g.Namespace

	if global.AddXRealIpHeader {
		addXRealIpStruct := g.constructAddXRealIpHeader()
		addXRealIpHeaderConfig, err := g.generateAddXRealIpHeaderEnvoyFilter(addXRealIpStruct, namespace)
		if err != nil {
			return nil, err
		}
		configPatch = append(configPatch, addXRealIpHeaderConfig...)
	}

	if global.DisableXEnvoyHeaders {
		disableXEnvoyHeadersStruct := g.constructDisableXEnvoyHeaders()
		disableXEnvoyHeadersConfig, err := g.generateDisableXEnvoyHeadersEnvoyFilter(disableXEnvoyHeadersStruct, namespace)
		if err != nil {
			return nil, err
		}
		configPatch = append(configPatch, disableXEnvoyHeadersConfig...)
	}

	if global.Downstream != nil {
		downstreamStruct := g.constructDownstream(global.Downstream)
		bufferLimitStruct := g.constructBufferLimit(global.Downstream)
		routeTimeoutStruct := g.constructRouteTimeout(global.Downstream)
		downstreamConfig, err := g.generateDownstreamEnvoyFilter(downstreamStruct, bufferLimitStruct, routeTimeoutStruct, namespace)
		if err != nil {
			return nil, err
		}
		if downstreamConfig != nil {
			configPatch = append(configPatch, downstreamConfig...)
		}
	}

	if global.Upstream != nil {
		upstreamStruct := g.constructUpstream(global.Upstream)
		bufferLimitStruct := g.constructUpstreamBufferLimit(global.Upstream)
		upstreamConfig, err := g.generateUpstreamEnvoyFilter(upstreamStruct, bufferLimitStruct, namespace)
		if err != nil {
			return nil, err
		}
		if upstreamConfig != nil {
			configPatch = append(configPatch, upstreamConfig...)
		}
	}

	if len(configPatch) == 0 {
		return []*config.Config{}, nil
	}

	return generateEnvoyFilter(namespace, configPatch), nil
}

func generateEnvoyFilter(namespace string, configPatch []*networking.EnvoyFilter_EnvoyConfigObjectPatch) []*config.Config {
	configs := make([]*config.Config, 0)
	envoyConfig := &config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.EnvoyFilter,
			Name:             higressGlobalEnvoyFilterName,
			Namespace:        namespace,
		},
		Spec: &networking.EnvoyFilter{
			ConfigPatches: configPatch,
		},
	}
	configs = append(configs, envoyConfig)
	return configs
}

func (g *GlobalOptionController) RegisterItemEventHandler(eventHandler ItemEventHandler) {
	g.eventHandler = eventHandler
}

// generateDownstreamEnvoyFilter generates the downstream envoy filter.
func (g *GlobalOptionController) generateDownstreamEnvoyFilter(downstreamValueStruct string, bufferLimitStruct string, routeTimeoutStruct string, namespace string) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	var downstreamConfig []*networking.EnvoyFilter_EnvoyConfigObjectPatch

	if len(downstreamValueStruct) != 0 {
		downstreamValue, err := util.BuildPatchStruct(downstreamValueStruct)
		if err != nil {
			return nil, err
		}
		downstreamConfig = append(downstreamConfig, &networking.EnvoyFilter_EnvoyConfigObjectPatch{
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
				Value:     downstreamValue,
			},
		})
	}

	if len(bufferLimitStruct) != 0 {
		bufferLimitValue, err := util.BuildPatchStruct(bufferLimitStruct)
		if err != nil {
			return nil, err
		}
		downstreamConfig = append(downstreamConfig, &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			ApplyTo: networking.EnvoyFilter_LISTENER,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     bufferLimitValue,
			},
		})
	}

	if len(routeTimeoutStruct) != 0 {
		routeTimeoutValue, err := util.BuildPatchStruct(routeTimeoutStruct)
		if err != nil {
			return nil, err
		}
		downstreamConfig = append(downstreamConfig, &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			ApplyTo: networking.EnvoyFilter_HTTP_ROUTE,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
				ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
					RouteConfiguration: &networking.EnvoyFilter_RouteConfigurationMatch{
						Vhost: &networking.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
							Route: &networking.EnvoyFilter_RouteConfigurationMatch_RouteMatch{
								Action: networking.EnvoyFilter_RouteConfigurationMatch_RouteMatch_ROUTE,
							},
						},
					},
				},
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     routeTimeoutValue,
			},
		})
	}

	return downstreamConfig, nil
}

func (g *GlobalOptionController) generateUpstreamEnvoyFilter(upstreamValueStruct string, bufferLimit string, namespace string) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	var upstreamConfig []*networking.EnvoyFilter_EnvoyConfigObjectPatch

	if len(upstreamValueStruct) != 0 {
		upstreamValue, err := util.BuildPatchStruct(upstreamValueStruct)
		if err != nil {
			return nil, err
		}
		upstreamConfig = append(upstreamConfig, &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			ApplyTo: networking.EnvoyFilter_CLUSTER,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     upstreamValue,
			},
		})
	}

	if len(bufferLimit) != 0 {
		bufferLimitValue, err := util.BuildPatchStruct(bufferLimit)
		if err != nil {
			return nil, err
		}
		upstreamConfig = append(upstreamConfig, &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			ApplyTo: networking.EnvoyFilter_CLUSTER,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     bufferLimitValue,
			},
		})
	}

	return upstreamConfig, nil
}

// generateAddXRealIpHeaderEnvoyFilter generates the add x-real-ip header envoy filter.
func (g *GlobalOptionController) generateAddXRealIpHeaderEnvoyFilter(addXRealIpHeaderStruct string, namespace string) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	// Internal-only patch: addXRealIpHeaderStruct is generated by this controller,
	// so an error here indicates a programming mistake rather than bad user input.
	addXRealIpHeaderValue, err := util.BuildPatchStruct(addXRealIpHeaderStruct)
	if err != nil {
		return nil, err
	}
	addXRealIpHeaderConfig := []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
		{
			ApplyTo: networking.EnvoyFilter_ROUTE_CONFIGURATION,
			Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: networking.EnvoyFilter_GATEWAY,
			},
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
				Value:     addXRealIpHeaderValue,
			},
		},
	}
	return addXRealIpHeaderConfig, nil
}

// generateDisableXEnvoyHeadersEnvoyFilter generates the disable x-envoy headers envoy filter.
func (g *GlobalOptionController) generateDisableXEnvoyHeadersEnvoyFilter(disableXEnvoyStruct string, namespace string) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	// Internal-only patch: disableXEnvoyStruct is generated by this controller,
	// so an error here indicates a programming mistake rather than bad user input.
	disableXEnvoyValue, err := util.BuildPatchStruct(disableXEnvoyStruct)
	if err != nil {
		return nil, err
	}
	disableXEnvoyHeadersConfig := []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
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
				Operation: networking.EnvoyFilter_Patch_REPLACE,
				Value:     disableXEnvoyValue,
			},
		},
	}
	return disableXEnvoyHeadersConfig, nil
}

// constructDownstream constructs the downstream config.
func (g *GlobalOptionController) constructDownstream(downstream *Downstream) string {
	downstreamConfig := ""
	idleTimeout := downstream.IdleTimeout
	maxRequestHeadersKb := downstream.MaxRequestHeadersKb

	if downstream.Http2 != nil {
		maxConcurrentStreams := downstream.Http2.MaxConcurrentStreams
		initialStreamWindowSize := downstream.Http2.InitialStreamWindowSize
		initialConnectionWindowSize := downstream.Http2.InitialConnectionWindowSize

		downstreamConfig = fmt.Sprintf(`
		{
			"name": "envoy.filters.network.http_connection_manager",
			"typed_config": {
				"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
				"common_http_protocol_options": {
					"idleTimeout": "%ds"
				},
				"http2_protocol_options": {
					"maxConcurrentStreams": %d,
					"initialStreamWindowSize": %d,
					"initialConnectionWindowSize": %d
				},
				"maxRequestHeadersKb": %d,
				"streamIdleTimeout": "%ds"
			}
		}
`, idleTimeout, maxConcurrentStreams, initialStreamWindowSize, initialConnectionWindowSize, maxRequestHeadersKb, idleTimeout)
		return downstreamConfig
	}

	downstreamConfig = fmt.Sprintf(`
		{
			"name": "envoy.filters.network.http_connection_manager",
			"typed_config": {
				"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
				"common_http_protocol_options": {
					"idleTimeout": "%ds"
				},
				"maxRequestHeadersKb": %d,
				"streamIdleTimeout": "%ds"
			}
		}
`, idleTimeout, maxRequestHeadersKb, idleTimeout)

	return downstreamConfig
}

// constructUpstream constructs the upstream config.
func (g *GlobalOptionController) constructUpstream(upstream *Upstream) string {
	upstreamConfig := ""
	idleTimeout := upstream.IdleTimeout

	upstreamConfig = fmt.Sprintf(`
		{
			"common_http_protocol_options": {
					"idleTimeout": "%ds"
            }
		}
`, idleTimeout)

	return upstreamConfig
}

// constructUpstreamBufferLimit constructs the upstream buffer limit config.
func (g *GlobalOptionController) constructUpstreamBufferLimit(upstream *Upstream) string {
	upstreamBufferLimitStruct := fmt.Sprintf(`
		{
			"per_connection_buffer_limit_bytes": %d
		}
	`, upstream.ConnectionBufferLimits)
	return upstreamBufferLimitStruct
}

// constructAddXRealIpHeader constructs the add x-real-ip header config.
func (g *GlobalOptionController) constructAddXRealIpHeader() string {
	addXRealIpHeaderStruct := fmt.Sprintf(`
		{
			"request_headers_to_add": [
				{
					"append": false,
					"header": {
						"key": "x-real-ip",
						"value": "%%REQ(X-ENVOY-EXTERNAL-ADDRESS)%%"
					}
				}
			]
		}
`)
	return addXRealIpHeaderStruct
}

// constructDisableXEnvoyHeaders constructs the disable x-envoy headers config.
func (g *GlobalOptionController) constructDisableXEnvoyHeaders() string {
	disableXEnvoyHeadersStruct := fmt.Sprintf(`
		{
			"name": "envoy.filters.http.router",
			"typed_config": {
				"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
				"suppress_envoy_headers": true
			}
		}
`)
	return disableXEnvoyHeadersStruct
}

// constructBufferLimit constructs the buffer limit config.
func (g *GlobalOptionController) constructBufferLimit(downstream *Downstream) string {
	return fmt.Sprintf(`
		{
			"per_connection_buffer_limit_bytes": %d
		}
	`, downstream.ConnectionBufferLimits)
}

// constructRouteTimeout constructs the route timeout config.
func (g *GlobalOptionController) constructRouteTimeout(downstream *Downstream) string {
	if downstream.RouteTimeout == 0 {
		return ""
	}
	return fmt.Sprintf(`
	{
		"route": {
			"timeout": "%ds"
		}
	}
	`, downstream.RouteTimeout)
}
