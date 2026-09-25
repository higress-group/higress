/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package scheduling implements request scheduling algorithms.
package scheduling

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend/sglang"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend/vllm"

	"github.com/prometheus/common/expfmt"
)

const (
	MetricPolicyDefault = "default"
	MetricPolicyLeast   = "least"
	MetricPolicyMost    = "most"
)

const (
	EngineVLLM   = "vllm"
	EngineSGLang = "sglang"
)

const (
	// TODO(https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/16) Make this configurable.
	kvCacheThreshold = 0.8
	// TODO(https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/16) Make this configurable.
	queueThresholdCritical = 5
	// TODO(https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/16) Make this configurable.
	// the threshold for queued requests to be considered low below which we can prioritize LoRA affinity.
	// The value of 50 is arrived heuristicically based on experiments.
	queueingThresholdLoRA = 50
)

var (
	defaultFilter = &filter{
		name:          "critical request",
		filter:        toFilterFunc(criticalRequestPredicate),
		nextOnSuccess: lowLatencyFilter,
		nextOnFailure: sheddableRequestFilter,
	}

	// queueLoRAAndKVCacheFilter applied least queue -> low cost lora ->  least KV Cache filter
	queueLoRAAndKVCacheFilter = &filter{
		name:   "least queuing",
		filter: leastQueuingFilterFunc,
		nextOnSuccessOrFailure: &filter{
			name:   "low cost LoRA",
			filter: toFilterFunc(lowLoRACostPredicate),
			nextOnSuccessOrFailure: &filter{
				name:   "least KV cache percent",
				filter: leastKVCacheFilterFunc,
			},
		},
	}

	// queueAndKVCacheFilter applies least queue followed by least KV Cache filter
	queueAndKVCacheFilter = &filter{
		name:   "least queuing",
		filter: leastQueuingFilterFunc,
		nextOnSuccessOrFailure: &filter{
			name:   "least KV cache percent",
			filter: leastKVCacheFilterFunc,
		},
	}

	lowLatencyFilter = &filter{
		name:   "low queueing filter",
		filter: toFilterFunc((lowQueueingPodPredicate)),
		nextOnSuccess: &filter{
			name:          "affinity LoRA",
			filter:        toFilterFunc(loRAAffinityPredicate),
			nextOnSuccess: queueAndKVCacheFilter,
			nextOnFailure: &filter{
				name:                   "can accept LoRA Adapter",
				filter:                 toFilterFunc(canAcceptNewLoraPredicate),
				nextOnSuccessOrFailure: queueAndKVCacheFilter,
			},
		},
		nextOnFailure: queueLoRAAndKVCacheFilter,
	}

	sheddableRequestFilter = &filter{
		// When there is at least one model server that's not queuing requests, and still has KV
		// cache below a certain threshold, we consider this model server has capacity to handle
		// a sheddable request without impacting critical requests.
		name:          "has capacity for sheddable requests",
		filter:        toFilterFunc(noQueueAndLessThanKVCacheThresholdPredicate(queueThresholdCritical, kvCacheThreshold)),
		nextOnSuccess: queueLoRAAndKVCacheFilter,
		// If all pods are queuing or running above the KVCache threshold, we drop the sheddable
		// request to make room for critical requests.
		nextOnFailure: &filter{
			name: "drop request",
			filter: func(req *LLMRequest, pods []*backend.PodMetrics) ([]*backend.PodMetrics, error) {
				// api.LogDebugf("Dropping request %v", req)
				return []*backend.PodMetrics{}, errors.New("dropping request due to limited backend resources")
			},
		},
	}
)

func NewScheduler(pm []*backend.PodMetrics, filter Filter) *Scheduler {

	return &Scheduler{
		podMetrics: pm,
		filter:     filter,
	}
}

type Scheduler struct {
	podMetrics []*backend.PodMetrics
	filter     Filter
}

// Schedule finds the target pod based on metrics and the requested lora adapter.
func (s *Scheduler) Schedule(req *LLMRequest) (targetPod backend.Pod, err error) {
	pods, err := s.filter.Filter(req, s.podMetrics)
	if err != nil || len(pods) == 0 {
		return backend.Pod{}, fmt.Errorf("failed to apply filter, resulted %v pods: %w", len(pods), err)
	}
	i := rand.Intn(len(pods))
	return pods[i].Pod, nil
}

func GetScheduler(hostMetrics map[string]string, metricPolicy string, targetMetric string, engine string) (*Scheduler, error) {
	if len(hostMetrics) == 0 {
		return nil, errors.New("backend is not support llm scheduling")
	}
	var pms []*backend.PodMetrics
	for addr, metric := range hostMetrics {
		parser := expfmt.TextParser{}
		metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(metric))
		if err != nil {
			return nil, err
		}
		pm := &backend.PodMetrics{
			Pod: backend.Pod{
				Name:    addr,
				Address: addr,
			},
			Metrics: backend.Metrics{},
			UserSelectedMetric: backend.UserSelectedMetric{
				MetricName: targetMetric,
			},
		}
		switch engine {
		case EngineSGLang:
			pm, err = sglang.PromToPodMetrics(metricFamilies, pm)
		default:
			pm, err = vllm.PromToPodMetrics(metricFamilies, pm)
		}
		if err != nil {
			return nil, err
		}
		pms = append(pms, pm)
	}
	if metricPolicy == MetricPolicyLeast {
		filterFunc := func(req *LLMRequest, pods []*backend.PodMetrics) ([]*backend.PodMetrics, error) {
			min := math.MaxFloat64
			max := 0.0
			filtered := []*backend.PodMetrics{}

			for _, pod := range pods {
				if pod.MetricValue <= min {
					min = pod.MetricValue
				}
				if pod.MetricValue >= max {
					max = pod.MetricValue
				}
			}

			for _, pod := range pods {
				if pod.MetricValue >= min && pod.MetricValue <= min+(max-min)/float64(len(pods)) {
					filtered = append(filtered, pod)
				}
			}
			return filtered, nil
		}
		filter := filter{
			name:   "least user selected metric",
			filter: filterFunc,
		}
		return NewScheduler(pms, &filter), nil
	} else if metricPolicy == MetricPolicyMost {
		filterFunc := func(req *LLMRequest, pods []*backend.PodMetrics) ([]*backend.PodMetrics, error) {
			min := math.MaxFloat64
			max := 0.0
			filtered := []*backend.PodMetrics{}

			for _, pod := range pods {
				if pod.MetricValue <= min {
					min = pod.MetricValue
				}
				if pod.MetricValue >= max {
					max = pod.MetricValue
				}
			}

			for _, pod := range pods {
				if pod.MetricValue <= max && pod.MetricValue >= max-(max-min)/float64(len(pods)) {
					filtered = append(filtered, pod)
				}
			}
			return filtered, nil
		}
		filter := filter{
			name:   "most user selected metric",
			filter: filterFunc,
		}
		return NewScheduler(pms, &filter), nil
	}
	return NewScheduler(pms, defaultFilter), nil
}
