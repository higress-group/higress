package cluster_hash

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	DefaultHashHeader    = "x-mse-consumer"
	DefaultClusterHeader = "x-higress-target-cluster"
	DefaultMaxBodyBytes  = 100 * 1024 * 1024
	maxUint32Value       = uint64(^uint32(0))
)

type hashKeySource string

const (
	hashKeySourceHeader   hashKeySource = "header"
	hashKeySourceCookie   hashKeySource = "cookie"
	hashKeySourceBody     hashKeySource = "body"
	hashKeySourceMetadata hashKeySource = "metadata"
)

type clusterEntry struct {
	Cluster string
	Weight  int
}

type hashKeyConfig struct {
	Source       hashKeySource
	Name         string
	JSONPath     string
	PropertyPath []string
	MaxBodyBytes uint32
}

type ClusterHashLoadBalancer struct {
	HashHeader    string
	ClusterHeader string
	Key           hashKeyConfig
	// slots is expanded from clusters by weight, length == 100.
	slots []string
}

func NewClusterHashLoadBalancer(json gjson.Result) (ClusterHashLoadBalancer, error) {
	lb := ClusterHashLoadBalancer{}

	lb.HashHeader = json.Get("hash_header").String()
	if lb.HashHeader == "" {
		lb.HashHeader = DefaultHashHeader
	}
	lb.Key = hashKeyConfig{
		Source:       hashKeySourceHeader,
		Name:         lb.HashHeader,
		MaxBodyBytes: DefaultMaxBodyBytes,
	}

	if keyJson := json.Get("key"); keyJson.Exists() {
		key, err := parseHashKeyConfig(keyJson, lb.HashHeader)
		if err != nil {
			return lb, err
		}
		lb.Key = key
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

func parseHashKeyConfig(json gjson.Result, fallbackHeader string) (hashKeyConfig, error) {
	key := hashKeyConfig{
		Source:       hashKeySource(json.Get("source").String()),
		MaxBodyBytes: DefaultMaxBodyBytes,
	}
	if key.Source == "" {
		key.Source = hashKeySourceHeader
	}
	if maxBodyBytes := json.Get("max_body_bytes").Uint(); maxBodyBytes > 0 {
		if maxBodyBytes > maxUint32Value {
			return key, fmt.Errorf("key.max_body_bytes exceeds uint32 limit")
		}
		key.MaxBodyBytes = uint32(maxBodyBytes)
	}

	switch key.Source {
	case hashKeySourceHeader:
		key.Name = json.Get("name").String()
		if key.Name == "" {
			key.Name = fallbackHeader
		}
		if key.Name == "" {
			key.Name = DefaultHashHeader
		}
	case hashKeySourceCookie:
		key.Name = json.Get("name").String()
		if key.Name == "" {
			return key, fmt.Errorf("key.name is required when key.source is cookie")
		}
	case hashKeySourceBody:
		key.JSONPath = normalizeJSONPath(json.Get("jsonPath").String())
		if key.JSONPath == "" {
			key.JSONPath = normalizeJSONPath(json.Get("json_path").String())
		}
		if key.JSONPath == "" {
			return key, fmt.Errorf("key.jsonPath is required when key.source is body")
		}
	case hashKeySourceMetadata:
		for _, part := range json.Get("propertyPath").Array() {
			if part.String() != "" {
				key.PropertyPath = append(key.PropertyPath, part.String())
			}
		}
		if len(key.PropertyPath) == 0 {
			for _, part := range json.Get("property_path").Array() {
				if part.String() != "" {
					key.PropertyPath = append(key.PropertyPath, part.String())
				}
			}
		}
		if len(key.PropertyPath) == 0 {
			name := json.Get("name").String()
			if name == "" {
				return key, fmt.Errorf("key.name or key.propertyPath is required when key.source is metadata")
			}
			key.PropertyPath = []string{"metadata", name}
		}
	default:
		return key, fmt.Errorf("unsupported key.source %q", key.Source)
	}

	return key, nil
}

func normalizeJSONPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	if path == "$" {
		return ""
	}
	return path
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
	if lb.Key.Source == hashKeySourceBody {
		if requestHasNoBody(ctx) {
			return rejectMissingHashKey(fmt.Errorf("request body required for body key source"))
		}
		ctx.SetRequestBodyBufferLimit(lb.Key.MaxBodyBytes)
		return types.HeaderStopIteration
	}

	hashKey, err := lb.extractHashKeyFromHeaders()
	if err != nil {
		return rejectMissingHashKey(err)
	}
	return lb.routeByHashKey(hashKey)
}

func (lb ClusterHashLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	if lb.Key.Source != hashKeySourceBody {
		return types.ActionContinue
	}
	hashKey := extractBodyHashKey(body, lb.Key.JSONPath)
	if hashKey == "" {
		return rejectMissingHashKey(fmt.Errorf("missing body json path %q", lb.Key.JSONPath))
	}
	return lb.routeByHashKey(hashKey)
}

func (lb ClusterHashLoadBalancer) extractHashKeyFromHeaders() (string, error) {
	switch lb.Key.Source {
	case hashKeySourceHeader:
		hashKey, err := proxywasm.GetHttpRequestHeader(lb.Key.Name)
		if err != nil || hashKey == "" {
			return "", fmt.Errorf("missing hash header %q", lb.Key.Name)
		}
		return hashKey, nil
	case hashKeySourceCookie:
		cookieHeader, err := proxywasm.GetHttpRequestHeader("cookie")
		if err != nil || cookieHeader == "" {
			return "", fmt.Errorf("missing cookie header")
		}
		hashKey := extractCookieHashKey(cookieHeader, lb.Key.Name)
		if hashKey == "" {
			return "", fmt.Errorf("missing cookie %q", lb.Key.Name)
		}
		return hashKey, nil
	case hashKeySourceMetadata:
		raw, err := proxywasm.GetProperty(lb.Key.PropertyPath)
		if err != nil || len(raw) == 0 {
			return "", fmt.Errorf("missing metadata property %q", strings.Join(lb.Key.PropertyPath, "."))
		}
		hashKey := string(raw)
		if hashKey == "" {
			return "", fmt.Errorf("empty metadata property %q", strings.Join(lb.Key.PropertyPath, "."))
		}
		return hashKey, nil
	default:
		return "", fmt.Errorf("unsupported key.source %q", lb.Key.Source)
	}
}

func extractCookieHashKey(cookieHeader, name string) string {
	request := http.Request{Header: http.Header{"Cookie": []string{cookieHeader}}}
	cookie, err := request.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func extractBodyHashKey(body []byte, jsonPath string) string {
	return gjson.GetBytes(body, jsonPath).String()
}

func requestHasNoBody(ctx wrapper.HttpContext) bool {
	method, _ := proxywasm.GetHttpRequestHeader(":method")
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return true
	}
	return !ctx.HasRequestBody()
}

func (lb ClusterHashLoadBalancer) routeByHashKey(hashKey string) types.Action {
	cluster := lb.selectCluster(hashKey)
	if err := proxywasm.ReplaceHttpRequestHeader(lb.ClusterHeader, cluster); err != nil {
		log.Errorf("[ai-load-balancer/cluster_hash] failed to set target header: %v", err)
		_ = proxywasm.SendHttpResponse(500, nil, []byte("internal error"), -1)
		return types.ActionPause
	}

	log.Debugf("[ai-load-balancer/cluster_hash] %s key routed -> %s=%s", lb.Key.Source, lb.ClusterHeader, cluster)
	return types.ActionContinue
}

func rejectMissingHashKey(err error) types.Action {
	log.Warnf("[ai-load-balancer/cluster_hash] %v, rejecting request", err)
	_ = proxywasm.SendHttpResponse(403, nil, []byte("hash key required"), -1)
	return types.ActionPause
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
