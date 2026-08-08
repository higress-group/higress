package prefixcache

import (
	"container/list"
	"sync"
)

type cacheEntry struct {
	hash uint64
}

type endpointCache struct {
	capacity int
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

func (index *Index) Score(endpoint string, chains [][]uint64) float64 {
	index.mu.RLock()
	defer index.mu.RUnlock()
	cache := index.endpoints[endpoint]
	matched, total := 0, 0
	for _, chain := range chains {
		total += len(chain)
		for _, hash := range chain {
			if cache == nil || cache.entries[hash] == nil {
				break
			}
			matched++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}

func (index *Index) Record(endpoint string, chains [][]uint64) {
	index.mu.Lock()
	defer index.mu.Unlock()
	cache := index.endpoint(endpoint)
	for _, chain := range chains {
		for _, hash := range chain {
			if element := cache.entries[hash]; element != nil {
				cache.order.MoveToFront(element)
				continue
			}
			element := cache.order.PushFront(cacheEntry{hash: hash})
			cache.entries[hash] = element
			cache.evict()
		}
	}
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
	for len(cache.entries) > cache.capacity {
		oldest := cache.order.Back()
		entry := oldest.Value.(cacheEntry)
		delete(cache.entries, entry.hash)
		cache.order.Remove(oldest)
	}
}
