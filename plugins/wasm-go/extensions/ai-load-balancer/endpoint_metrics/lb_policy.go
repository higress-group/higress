package endpoint_metrics

import (
	"math/rand"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/scheduling"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	FixedQueueSize = 100
)

type MetricsEndpointLoadBalancer struct {
	metricPolicy     string
	targetMetric     string
	engine           string
	endpointRequests *utils.FixedQueue[string]
	maxRate          float64
}

func NewMetricsEndpointLoadBalancer(json gjson.Result) (MetricsEndpointLoadBalancer, error) {
	lb := MetricsEndpointLoadBalancer{}
	if json.Get("metric_policy").Exists() {
		lb.metricPolicy = json.Get("metric_policy").String()
	} else {
		lb.metricPolicy = scheduling.MetricPolicyDefault
	}
	if json.Get("target_metric").Exists() {
		lb.targetMetric = json.Get("target_metric").String()
	}
	if json.Get("engine").Exists() {
		lb.engine = json.Get("engine").String()
	} else {
		lb.engine = scheduling.EngineVLLM
	}
	if json.Get("rate_limit").Exists() {
		lb.maxRate = json.Get("rate_limit").Float()
	} else {
		lb.maxRate = 1.0
	}
	lb.endpointRequests = utils.NewFixedQueue[string](FixedQueueSize)
	return lb, nil
}

// Callbacks which are called in request path
func (lb MetricsEndpointLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	// If return types.ActionContinue, SetUpstreamOverrideHost will not take effect
	return types.HeaderStopIteration
}

func (lb MetricsEndpointLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	requestModel := gjson.GetBytes(body, "model")
	if !requestModel.Exists() {
		return types.ActionContinue
	}
	llmReq := &scheduling.LLMRequest{
		Model:    requestModel.String(),
		Critical: true,
	}
	hostInfos, err := proxywasm.GetUpstreamHosts()
	if err != nil {
		return types.ActionContinue
	}
	hostMetrics := make(map[string]string)
	for _, hostInfo := range hostInfos {
		if gjson.Get(hostInfo[1], "health_status").String() == "Healthy" {
			hostMetrics[hostInfo[0]] = gjson.Get(hostInfo[1], "metrics").String()
		}
	}
	scheduler, err := scheduling.GetScheduler(hostMetrics, lb.metricPolicy, lb.targetMetric, lb.engine)
	if err != nil {
		log.Debugf("initial scheduler failed: %v", err)
		return types.ActionContinue
	}
	targetPod, err := scheduler.Schedule(llmReq)
	log.Debugf("targetPod: %+v", targetPod.Address)
	if err != nil {
		log.Debugf("pod select failed: %v", err)
		return types.ActionContinue
	}
	finalAddress := targetPod.Address
	otherHosts := []string{} // 如果当前host超过请求数限制，那么在其中随机挑选一个
	currentRate := 0.0
	for k := range hostMetrics {
		if k != finalAddress {
			otherHosts = append(otherHosts, k)
		}
	}
	if lb.endpointRequests.Size() != 0 {
		count := 0.0
		lb.endpointRequests.ForEach(func(i int, item string) {
			if item == finalAddress {
				count += 1
			}
		})
		currentRate = count / float64(lb.endpointRequests.Size())
	}
	if currentRate > lb.maxRate && len(otherHosts) > 0 {
		finalAddress = otherHosts[rand.Intn(len(otherHosts))]
	}
	lb.endpointRequests.Enqueue(finalAddress)
	log.Debugf("pod %s is selected", finalAddress)
	proxywasm.SetUpstreamOverrideHost([]byte(finalAddress))
	return types.ActionContinue
}

func (lb MetricsEndpointLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	ctx.DontReadResponseBody()
	return types.ActionContinue
}

func (lb MetricsEndpointLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	return data
}

func (lb MetricsEndpointLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb MetricsEndpointLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {}
