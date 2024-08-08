package cache

import (
	"sync"

	"github.com/drand/go-clients/drand"
)

// MapCache is a simple cache that stores data in memory.
type MapCache struct {
	sync.RWMutex
	data map[uint64]drand.Result
}

// NewMapCache creates a new in memory cache backed by a map.
func NewMapCache() *MapCache {
	return &MapCache{data: make(map[uint64]drand.Result)}
}

// TryGet provides a round beacon or nil if it is not cached.
func (mc *MapCache) TryGet(round uint64) drand.Result {
	mc.RLock()
	defer mc.RUnlock()
	r, ok := mc.data[round]
	if !ok {
		return nil
	}
	return r
}

// Add adds an item to the cache
func (mc *MapCache) Add(round uint64, result drand.Result) {
	mc.Lock()
	mc.data[round] = result
	mc.Unlock()
}
