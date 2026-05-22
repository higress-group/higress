package main

import (
	"fmt"
	"hash/fnv"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-provider-affinity",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

type ProviderAffinityConfig struct {
	ConsumerHeader string `json:"consumer_header"`
	ClusterHeader  string `json:"cluster_header"`
	// slots is expanded from providers by weight, length == 100.
	slots []string
}

type provider struct {
	Cluster string
	Weight  int
}

func parseConfig(json gjson.Result, config *ProviderAffinityConfig) error {
	config.ConsumerHeader = json.Get("consumer_header").String()
	if config.ConsumerHeader == "" {
		config.ConsumerHeader = "x-mse-consumer"
	}

	config.ClusterHeader = json.Get("cluster_header").String()
	if config.ClusterHeader == "" {
		config.ClusterHeader = "x-higress-target-cluster"
	}

	providersJson := json.Get("providers")
	if !providersJson.Exists() || !providersJson.IsArray() || len(providersJson.Array()) == 0 {
		return fmt.Errorf("providers is required and must be a non-empty array")
	}

	var providers []provider
	var totalWeight int
	for _, p := range providersJson.Array() {
		cluster := p.Get("cluster").String()
		if cluster == "" {
			return fmt.Errorf("each provider must have a non-empty cluster field")
		}
		weight := int(p.Get("weight").Int())
		if weight <= 0 {
			return fmt.Errorf("provider %q has invalid weight %d, must be > 0", cluster, weight)
		}
		providers = append(providers, provider{Cluster: cluster, Weight: weight})
		totalWeight += weight
	}

	if totalWeight != 100 {
		return fmt.Errorf("sum of provider weights must be 100, got %d", totalWeight)
	}

	slots := make([]string, 0, 100)
	for _, p := range providers {
		for i := 0; i < p.Weight; i++ {
			slots = append(slots, p.Cluster)
		}
	}
	config.slots = slots
	return nil
}

func selectCluster(slots []string, consumer string) string {
	h := fnv.New32a()
	h.Write([]byte(consumer))
	index := int(h.Sum32()) % len(slots)
	if index < 0 {
		index += len(slots)
	}
	return slots[index]
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config ProviderAffinityConfig) types.Action {
	consumer, err := proxywasm.GetHttpRequestHeader(config.ConsumerHeader)
	if err != nil || consumer == "" {
		log.Warnf("[ai-provider-affinity] missing consumer header %q, rejecting request", config.ConsumerHeader)
		_ = proxywasm.SendHttpResponse(403, nil, []byte("consumer header required"), -1)
		return types.ActionPause
	}

	cluster := selectCluster(config.slots, consumer)
	if err := proxywasm.ReplaceHttpRequestHeader(config.ClusterHeader, cluster); err != nil {
		log.Errorf("[ai-provider-affinity] failed to set target header: %v", err)
		_ = proxywasm.SendHttpResponse(500, nil, []byte("internal error"), -1)
		return types.ActionPause
	}

	log.Debugf("[ai-provider-affinity] consumer=%s -> %s=%s", consumer, config.ClusterHeader, cluster)
	return types.ActionContinue
}
