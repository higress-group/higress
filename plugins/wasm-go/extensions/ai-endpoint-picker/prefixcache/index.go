package prefixcache

import (
	"container/list"
	"math"
	"sync"
)

type cacheEntry struct {
	hash uint64
	cost int
}

type endpointCache struct {
	capacity int
	usedCost int
	order    *list.List
	entries  map[uint64]*list.Element
}

type Index struct {
	mu              sync.RWMutex
	defaultCapacity int
	endpoints       map[string]*endpointCache
}

func NewIndex(defaultCapacity int) *Index {
	if defaultCapacity <= 0 {
		defaultCapacity = DefaultCapacity
	}
	return &Index{defaultCapacity: defaultCapacity, endpoints: map[string]*endpointCache{}}
}

func (index *Index) SetCapacity(endpoint string, capacity int) {
	index.mu.Lock()
	defer index.mu.Unlock()
	cache := index.endpoint(endpoint)
	if capacity <= 0 {
		capacity = index.defaultCapacity
	}
	cache.capacity = capacity
	cache.evict()
}

func (index *Index) Score(endpoint string, chains [][]Block) float64 {
	index.mu.RLock()
	defer index.mu.RUnlock()
	cache := index.endpoints[endpoint]
	matchedTokens, totalTokens := 0, 0
	for _, chain := range chains {
		for _, block := range chain {
			totalTokens += block.EstimatedTokens
		}
		for _, block := range chain {
			if cache == nil || cache.entries[block.Hash] == nil {
				break
			}
			matchedTokens += block.EstimatedTokens
		}
	}
	if totalTokens == 0 {
		return 0
	}
	ratioScore := float64(matchedTokens) / float64(totalTokens)
	lengthRatio := math.Min(float64(matchedTokens)/8192, 1)
	return 0.75*ratioScore + 0.25*lengthRatio*lengthRatio
}

func (index *Index) Record(endpoint string, chains [][]Block, actualBlockSize int) {
	if actualBlockSize <= 0 {
		actualBlockSize = DefaultBlockSize
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	cache := index.endpoint(endpoint)
	for _, chain := range chains {
		for blockIndex := len(chain) - 1; blockIndex >= 0; blockIndex-- {
			block := chain[blockIndex]
			cost := block.EstimatedTokens / actualBlockSize
			if block.EstimatedTokens%actualBlockSize != 0 {
				cost++
			}
			if element := cache.entries[block.Hash]; element != nil {
				entry := element.Value.(cacheEntry)
				cache.usedCost += cost - entry.cost
				element.Value = cacheEntry{hash: block.Hash, cost: cost}
				cache.order.MoveToFront(element)
			} else {
				element := cache.order.PushFront(cacheEntry{hash: block.Hash, cost: cost})
				cache.entries[block.Hash] = element
				cache.usedCost += cost
			}
			cache.evict()
		}
	}
}

func (index *Index) Delete(endpoint string) {
	index.mu.Lock()
	defer index.mu.Unlock()
	delete(index.endpoints, endpoint)
}

func (index *Index) Cleanup(active map[string]struct{}) {
	index.mu.Lock()
	defer index.mu.Unlock()
	for endpoint := range index.endpoints {
		if _, ok := active[endpoint]; !ok {
			delete(index.endpoints, endpoint)
		}
	}
}

func (index *Index) Len(endpoint string) int {
	index.mu.RLock()
	defer index.mu.RUnlock()
	cache := index.endpoints[endpoint]
	if cache == nil {
		return 0
	}
	return len(cache.entries)
}

func (index *Index) UsedCost(endpoint string) int {
	index.mu.RLock()
	defer index.mu.RUnlock()
	cache := index.endpoints[endpoint]
	if cache == nil {
		return 0
	}
	return cache.usedCost
}

func (index *Index) EndpointCount() int {
	index.mu.RLock()
	defer index.mu.RUnlock()
	return len(index.endpoints)
}

func (index *Index) endpoint(name string) *endpointCache {
	cache := index.endpoints[name]
	if cache == nil {
		cache = &endpointCache{
			capacity: index.defaultCapacity,
			order:    list.New(),
			entries:  map[uint64]*list.Element{},
		}
		index.endpoints[name] = cache
	}
	return cache
}

func (cache *endpointCache) evict() {
	for cache.usedCost > cache.capacity {
		oldest := cache.order.Back()
		entry := oldest.Value.(cacheEntry)
		cache.usedCost -= entry.cost
		delete(cache.entries, entry.hash)
		cache.order.Remove(oldest)
	}
}
