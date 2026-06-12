package cluster_hash

import (
	"fmt"
	"hash/fnv"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	DefaultHashHeader    = "x-mse-consumer"
	DefaultClusterHeader = "x-higress-target-cluster"
)

type clusterEntry struct {
	Cluster string
	Weight  int
}

type ClusterHashLoadBalancer struct {
	HashHeader    string
	ClusterHeader string
	// slots is expanded from clusters by weight, length == 100.
	slots []string
}

func NewClusterHashLoadBalancer(json gjson.Result) (ClusterHashLoadBalancer, error) {
	lb := ClusterHashLoadBalancer{}

	lb.HashHeader = json.Get("hash_header").String()
	if lb.HashHeader == "" {
		lb.HashHeader = DefaultHashHeader
	}

	lb.ClusterHeader = json.Get("cluster_header").String()
	if lb.ClusterHeader == "" {
		lb.ClusterHeader = DefaultClusterHeader
	}

	clustersJson := json.Get("clusters")
	if !clustersJson.Exists() || !clustersJson.IsArray() || len(clustersJson.Array()) == 0 {
		return lb, fmt.Errorf("clusters is required and must be a non-empty array")
	}

	var clusters []clusterEntry
	var totalWeight int
	for _, c := range clustersJson.Array() {
		cluster := c.Get("cluster").String()
		if cluster == "" {
			return lb, fmt.Errorf("each entry must have a non-empty cluster field")
		}
		weight := int(c.Get("weight").Int())
		if weight <= 0 {
			return lb, fmt.Errorf("cluster %q has invalid weight %d, must be > 0", cluster, weight)
		}
		clusters = append(clusters, clusterEntry{Cluster: cluster, Weight: weight})
		totalWeight += weight
	}

	if totalWeight != 100 {
		return lb, fmt.Errorf("sum of cluster weights must be 100, got %d", totalWeight)
	}

	slots := make([]string, 0, 100)
	for _, c := range clusters {
		for i := 0; i < c.Weight; i++ {
			slots = append(slots, c.Cluster)
		}
	}
	lb.slots = slots
	return lb, nil
}

func (lb ClusterHashLoadBalancer) selectCluster(hashKey string) string {
	h := fnv.New32a()
	h.Write([]byte(hashKey))
	index := int(h.Sum32()) % len(lb.slots)
	if index < 0 {
		index += len(lb.slots)
	}
	return lb.slots[index]
}

func (lb ClusterHashLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	hashKey, err := proxywasm.GetHttpRequestHeader(lb.HashHeader)
	if err != nil || hashKey == "" {
		log.Warnf("[ai-load-balancer/cluster_hash] missing hash header %q, rejecting request", lb.HashHeader)
		_ = proxywasm.SendHttpResponse(403, nil, []byte("hash header required"), -1)
		return types.ActionPause
	}

	cluster := lb.selectCluster(hashKey)
	if err := proxywasm.ReplaceHttpRequestHeader(lb.ClusterHeader, cluster); err != nil {
		log.Errorf("[ai-load-balancer/cluster_hash] failed to set target header: %v", err)
		_ = proxywasm.SendHttpResponse(500, nil, []byte("internal error"), -1)
		return types.ActionPause
	}

	log.Debugf("[ai-load-balancer/cluster_hash] %s=%s -> %s=%s", lb.HashHeader, hashKey, lb.ClusterHeader, cluster)
	return types.ActionContinue
}

func (lb ClusterHashLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb ClusterHashLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	return types.ActionContinue
}

func (lb ClusterHashLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	return data
}

func (lb ClusterHashLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb ClusterHashLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {}
