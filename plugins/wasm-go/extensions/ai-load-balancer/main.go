package main

import (
	"fmt"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/cluster_metrics"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/cluster_hash"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/global_least_request"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/prefix_cache"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-load-balancer",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.ProcessStreamDone(onHttpStreamDone),
	)
}

type LoadBalancer interface {
	HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action
	HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action
	HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action
	HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte
	HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action
	HandleHttpStreamDone(ctx wrapper.HttpContext)
}

type Config struct {
	lbType   string
	lbPolicy string
	lb       LoadBalancer
}

const (
	ClusterLoadBalancerType  = "cluster"
	EndpointLoadBalancerType = "endpoint"
	// Cluster load balancer policies
	MetricsBasedCluster = "cluster_metrics"
	ClusterHashCluster  = "cluster_hash"
	// Endpoint load balancer policies
	MetricsBasedEndpoint           = "endpoint_metrics"
	MetricsBasedEndpointDeprecated = "metrics_based" // Compatible with old configurations, equal to `endpoint_metrics`
	GlobalLeastRequestEndpoint     = "global_least_request"
	PrefixCacheEndpoint            = "prefix_cache"
)

func parseConfig(json gjson.Result, config *Config) error {
	config.lbType = json.Get("lb_type").String()
	// Compatible with old configurations
	if config.lbType == "" {
		config.lbType = EndpointLoadBalancerType
	}
	config.lbPolicy = json.Get("lb_policy").String()
	var err error
	switch config.lbType {
	case ClusterLoadBalancerType:
		switch config.lbPolicy {
		case MetricsBasedCluster:
			config.lb, err = cluster_metrics.NewClusterEndpointLoadBalancer(json.Get("lb_config"))
		case ClusterHashCluster:
			config.lb, err = cluster_hash.NewClusterHashLoadBalancer(json.Get("lb_config"))
		default:
			err = fmt.Errorf("lb_policy %s is not supported", config.lbPolicy)
		}
	case EndpointLoadBalancerType:
		switch config.lbPolicy {
		case MetricsBasedEndpoint, MetricsBasedEndpointDeprecated:
			config.lb, err = endpoint_metrics.NewMetricsEndpointLoadBalancer(json.Get("lb_config"))
		case GlobalLeastRequestEndpoint:
			config.lb, err = global_least_request.NewGlobalLeastRequestLoadBalancer(json.Get("lb_config"))
		case PrefixCacheEndpoint:
			config.lb, err = prefix_cache.NewPrefixCacheLoadBalancer(json.Get("lb_config"))
		default:
			err = fmt.Errorf("lb_psolicy %s is not supported", config.lbPolicy)
		}
	default:
		err = fmt.Errorf("lb_type %s is not supported", config.lbType)
	}
	return err
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	return config.lb.HandleHttpRequestHeaders(ctx)
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	return config.lb.HandleHttpRequestBody(ctx, body)
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	return config.lb.HandleHttpResponseHeaders(ctx)
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config Config, data []byte, endOfStream bool) []byte {
	return config.lb.HandleHttpStreamingResponseBody(ctx, data, endOfStream)
}

func onHttpResponseBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	return config.lb.HandleHttpResponseBody(ctx, body)
}

func onHttpStreamDone(ctx wrapper.HttpContext, config Config) {
	config.lb.HandleHttpStreamDone(ctx)
}
