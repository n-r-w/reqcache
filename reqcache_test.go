//nolint:exhaustruct // tests
package reqcache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// mockLogger is a mock implementation of the iLogger interface for testing purposes.
type mockLogger struct {
	name string

	objHit  int
	objMiss int

	cacheHit  int
	cacheMiss int

	mu sync.Mutex
}

func (m *mockLogger) LogObjectPoolHitRatio(_ context.Context, name string, hit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.name = name
	if hit {
		m.objHit++
	} else {
		m.objMiss++
	}
}

func (m *mockLogger) LogCacheHitRatio(_ context.Context, name string, hit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.name = name
	if hit {
		m.cacheHit++
	} else {
		m.cacheMiss++
	}
}

type reqCacheTestObject struct {
	value int
}

func TestSession(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())

	require.True(t, InContext(ctx))

	require.Panics(t, func() {
		NewSession(ctx)
	}, "context already has a reqcache key")
}

func TestInContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Panics(t, func() { fromContext(ctx) })

	require.False(t, InContext(ctx))

	ctx = NewSession(ctx)
	require.True(t, InContext(ctx))
}

func TestReqCache_NewObject(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())

	cache := New[string, reqCacheTestObject](10, 10)
	obj := cache.NewObject(ctx)
	require.Equal(t, 0, obj.value)
}

func TestReqCache_Exists(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())
	cache := New[string, reqCacheTestObject](10, 10)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	cache.Put(ctx, key, value)

	require.True(t, cache.Exists(ctx, key))
	require.False(t, cache.Exists(ctx, "key2"))
}

func TestReqCache_PutAndGet(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())
	cache := New[string, reqCacheTestObject](10, 10)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	cache.Put(ctx, key, value)

	retrievedValue, ok := cache.Get(ctx, key)
	require.True(t, ok)
	require.Equal(t, value, retrievedValue)

	require.True(t, cache.Exists(ctx, key))

	const nonExistentKey = "key2"
	_, exists := cache.Get(ctx, nonExistentKey)
	require.False(t, exists)

	cache.Delete(ctx, key)
	require.False(t, cache.Exists(ctx, key))
}

func TestReqCache_Delete(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())
	cache := New[string, reqCacheTestObject](10, 10)

	key := "key1"
	value := &reqCacheTestObject{value: 100}
	cache.Put(ctx, key, value)

	retrievedValue, ok := cache.Get(ctx, key)
	require.True(t, ok)
	require.Equal(t, value, retrievedValue)

	cache.EndSession(ctx)

	_, exists := cache.Get(ctx, key)
	require.False(t, exists)
}

func TestNewObject(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())

	cache := New[string, reqCacheTestObject](10, 10)

	// Ensure that we can create new objects without overflowing the pool
	var prevObj *reqCacheTestObject
	for i := 0; i < 20; i++ {
		obj := cache.NewObject(ctx)
		require.Equal(t, 0, obj.value, "New object should have a value of 0")

		if prevObj == obj {
			t.Fatalf("New object should not be the same as the previous one")
		}

		prevObj = obj
	}

	// Ensure that the object pool is reset after clearing the cache
	cache.EndSession(ctx)
	require.Empty(t, cache.objects, "Object pool should be empty after cache is cleared")
}

func TestReqCache_GetOrFetch(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())
	cache := New[string, reqCacheTestObject](10, 10)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}

	// Fetcher function that returns the value
	fetcher := func(context.Context, *ReqCache[string, reqCacheTestObject]) (*reqCacheTestObject, error) {
		return value, nil
	}

	retrievedValue, err := cache.GetOrFetch(ctx, key, fetcher)
	require.NoError(t, err)
	require.Equal(t, value, retrievedValue)

	// Ensure value is correctly stored in the cache
	cachedValue, ok := cache.Get(ctx, key)
	require.True(t, ok)
	require.Equal(t, value, cachedValue)

	// Validate that fetcher is not called again and the cached value is returned
	newValue, err := cache.GetOrFetch(ctx, key,
		func(context.Context, *ReqCache[string, reqCacheTestObject]) (*reqCacheTestObject, error) {
			return &reqCacheTestObject{value: 200}, nil
		})
	require.NoError(t, err)
	require.Equal(t, value, newValue)

	// Ensure that error is returned if fetcher returns an error
	_, err = cache.GetOrFetch(ctx, "key2",
		func(context.Context, *ReqCache[string, reqCacheTestObject]) (*reqCacheTestObject, error) {
			return nil, errors.New("fetcher error")
		})
	require.Error(t, err)
}

func TestReqCache_GetOrNew(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())
	cache := New[string, reqCacheTestObject](10, 10)

	const key = "key1"
	initialValue := 100

	// Prepare function that sets the value
	prepare := func(_ context.Context, obj *reqCacheTestObject) error {
		obj.value = initialValue
		return nil
	}

	retrievedValue, err := cache.GetOrNew(ctx, key, prepare)
	require.NoError(t, err)
	require.Equal(t, initialValue, retrievedValue.value)

	// Ensure value is correctly stored in the cache
	cachedValue, ok := cache.Get(ctx, key)
	require.True(t, ok)
	require.Equal(t, initialValue, cachedValue.value)

	// Validate that prepare is not called again and the cached value is returned
	newPrepare := func(_ context.Context, obj *reqCacheTestObject) error {
		obj.value = 200
		return nil
	}

	newValue, err := cache.GetOrNew(ctx, key, newPrepare)
	require.NoError(t, err)
	require.Equal(t, initialValue, newValue.value)

	// Ensure that error is returned if prepare returns an error
	_, err = cache.GetOrNew(ctx, "key2", func(context.Context, *reqCacheTestObject) error {
		return errors.New("prepare error")
	})
	require.Error(t, err)
}

func TestReqCache_HitRatio(t *testing.T) {
	t.Parallel()

	ctx := NewSession(context.Background())

	logger := &mockLogger{}
	cache := New[string, reqCacheTestObject](0, 1, WithLogger("test", logger))

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	cache.Put(ctx, key, value)

	// Ensure that we get object from the cache
	retrievedValue, ok := cache.Get(ctx, key)
	require.True(t, ok)
	require.Equal(t, value, retrievedValue)
	require.Equal(t, &mockLogger{name: "test", objHit: 0, objMiss: 0, cacheHit: 1, cacheMiss: 0}, logger)

	// Not found in the cache
	_, ok = cache.Get(ctx, "key2")
	require.False(t, ok)
	require.Equal(t, &mockLogger{name: "test", objHit: 0, objMiss: 0, cacheHit: 1, cacheMiss: 1}, logger)
}

func TestAsyncReqCache(t *testing.T) {
	t.Parallel()

	const (
		nParallel = 100
		objCount  = 100
	)

	var (
		errGroup errgroup.Group
		cache    = New[string, reqCacheTestObject](objCount, objCount)
	)

	// Ensure that we can work with multiple threads without interference between them
	for i := 0; i < nParallel; i++ {
		errGroup.Go(func() error {
			ctx := NewSession(context.Background())
			defer cache.EndSession(ctx)

			objects := make([]*reqCacheTestObject, objCount)

			for k := 0; k < objCount; k++ {
				key := "key" + strconv.Itoa(k)
				obj := cache.NewObject(ctx)
				obj.value = k
				cache.Put(ctx, key, obj)
				objects[k] = obj
			}

			for k := 0; k < objCount; k++ {
				key := "key" + strconv.Itoa(k)
				v, ok := cache.Get(ctx, key)
				if !ok {
					return fmt.Errorf("value not found, expected %d", k)
				}

				if v.value != k {
					return fmt.Errorf("value mismatch, expected %d, got %d", k, v.value)
				}

				if v != objects[k] {
					return fmt.Errorf("object mismatch, expected %p, got %p", objects[k], v)
				}
			}

			reqID := fromContext(ctx)

			cache.muData.RLock()
			defer cache.muData.RUnlock()
			cacheLen := cache.data[reqID].Len()
			if cacheLen != objCount {
				return fmt.Errorf("data cache length mismatch, expected %d, got %d", objCount, cacheLen)
			}

			cache.muObjects.Lock()
			defer cache.muObjects.Unlock()
			objectsLen := cache.objects[reqID].index
			if objectsLen != objCount {
				return fmt.Errorf("pool length mismatch, expected %d, got %d", objCount, objectsLen)
			}

			return nil
		})
	}

	require.NoError(t, errGroup.Wait())

	// Ensure that the object pool is empty after all goroutines are done
	require.Empty(t, cache.objects, "Object pool should be empty after all goroutines are done")
	require.Empty(t, cache.data, "Data cache should be empty after all goroutines are done")
}
