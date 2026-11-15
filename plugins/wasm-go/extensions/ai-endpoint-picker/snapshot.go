package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/tidwall/gjson"
)

const endpointSnapshotTTL = 250 * time.Millisecond

var errUpstreamHostsUnavailable = errors.New("upstream hosts unavailable")

type upstreamHostsGetter func() ([][2]string, error)
type vllmMetricsParser func([]byte) (scheduling.VLLMMetrics, error)

type compactHostSnapshot struct {
	address     string
	healthy     bool
	metrics     scheduling.VLLMMetrics
	cacheConfig scheduling.CacheConfig
	fingerprint uint64
}

func (snapshot compactHostSnapshot) candidate(model string) parsedHostCandidate {
	signals := map[scheduling.SignalName]scheduling.SignalValue{}
	if snapshot.healthy {
		signals = snapshot.metrics.SignalsForModel(model)
	}
	return parsedHostCandidate{
		endpoint: scheduling.EndpointSnapshot{
			Address: snapshot.address,
			Healthy: snapshot.healthy,
			Signals: signals,
		},
		cacheConfig: snapshot.cacheConfig,
	}
}

type endpointSnapshotResult struct {
	hosts    []compactHostSnapshot
	skipMask candidateSkipReason
	skipped  int
}

type endpointSnapshotCache struct {
	mutex     sync.Mutex
	getHosts  upstreamHostsGetter
	parse     vllmMetricsParser
	now       func() time.Time
	expiresAt time.Time
	result    endpointSnapshotResult
}

func newEndpointSnapshotCache(getHosts upstreamHostsGetter, parser vllmMetricsParser, now func() time.Time) *endpointSnapshotCache {
	return &endpointSnapshotCache{getHosts: getHosts, parse: parser, now: now}
}

func (cache *endpointSnapshotCache) get() (endpointSnapshotResult, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	now := cache.now()
	if !cache.expiresAt.IsZero() && now.Before(cache.expiresAt) {
		return cache.result, nil
	}
	hosts, err := cache.getHosts()
	if err != nil || len(hosts) == 0 {
		cache.result = endpointSnapshotResult{}
		cache.expiresAt = time.Time{}
		return endpointSnapshotResult{}, errUpstreamHostsUnavailable
	}

	previous := make(map[string]compactHostSnapshot, len(cache.result.hosts))
	for _, host := range cache.result.hosts {
		previous[host.address] = host
	}
	result := endpointSnapshotResult{hosts: make([]compactHostSnapshot, 0, len(hosts))}
	for _, host := range hosts {
		snapshot, reason := cache.parseHost(host, previous[strings.TrimSpace(host[0])])
		if reason != 0 {
			result.skipMask |= reason
			result.skipped++
			continue
		}
		result.hosts = append(result.hosts, snapshot)
	}
	cache.result = result
	cache.expiresAt = now.Add(endpointSnapshotTTL)
	return result, nil
}

func (cache *endpointSnapshotCache) parseHost(host [2]string, previous compactHostSnapshot) (compactHostSnapshot, candidateSkipReason) {
	address, metadata := strings.TrimSpace(host[0]), host[1]
	if address == "" {
		return compactHostSnapshot{}, candidateSkipAddress
	}
	if !gjson.Valid(metadata) || !gjson.Parse(metadata).IsObject() {
		return compactHostSnapshot{}, candidateSkipMetadata
	}
	health := gjson.Get(metadata, "health_status")
	if health.Type != gjson.String || strings.TrimSpace(health.String()) == "" {
		return compactHostSnapshot{}, candidateSkipHealth
	}
	snapshot := compactHostSnapshot{address: address, healthy: health.String() == "Healthy"}
	if !snapshot.healthy {
		return snapshot, 0
	}
	metrics := gjson.Get(metadata, "metrics")
	if metrics.Exists() && metrics.Type != gjson.String {
		return compactHostSnapshot{}, candidateSkipMetrics
	}
	metricsText := metrics.String()
	compactMetrics, fingerprint, err := scheduling.CompactVLLMMetrics(metricsText)
	if err != nil {
		return compactHostSnapshot{}, candidateSkipMetrics
	}
	snapshot.fingerprint = fingerprint
	if previous.address == address && previous.healthy && previous.fingerprint == snapshot.fingerprint {
		snapshot.metrics = previous.metrics
		snapshot.cacheConfig = previous.cacheConfig
		return snapshot, 0
	}
	parsed, err := cache.parse(compactMetrics)
	if err != nil {
		return compactHostSnapshot{}, candidateSkipMetrics
	}
	snapshot.cacheConfig = parsed.CacheConfig
	parsed.CacheConfig = scheduling.CacheConfig{}
	snapshot.metrics = parsed
	return snapshot, 0
}
