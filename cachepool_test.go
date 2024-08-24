package reqcache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type cachePoolTestObject struct {
	value int
}

func TestCachePool(t *testing.T) {
	t.Parallel()

	// Define keys and values for test
	keys := []int{1, 2, 3}
	values := []*cachePoolTestObject{{value: 1}, {value: 2}, {value: 3}}

	// Create a new pool wrapper with cache size 2
	pool := newPoolWrapper[int, cachePoolTestObject](2)

	// Get a cache instance from pool
	cache := pool.Get()

	// Ensure cache is empty initially
	for _, key := range keys {
		_, ok := cache.Get(key)
		require.False(t, ok, "expected cache to be empty initially")
	}

	// Insert data into cache
	for i, key := range keys {
		cache.Add(key, values[i])
	}

	// Ensure only two items are stored due to LRU policy
	_, ok := cache.Get(keys[0])
	require.False(t, ok, "expected first item to be evicted")

	for i := 1; i < len(keys); i++ {
		var val *cachePoolTestObject
		val, ok = cache.Get(keys[i])
		require.True(t, ok, "expected item to be in cache")
		require.Equal(t, values[i], val)
	}

	// Put the cache back into the pool
	pool.Put(cache)

	// Get a new cache instance from pool and verify it is empty (since we called Purge)
	newCache := pool.Get()
	for _, key := range keys {
		_, ok = newCache.Get(key)
		require.False(t, ok, "expected cache to be empty after purge")
	}
}
