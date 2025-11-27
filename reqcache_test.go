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

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	require.True(t, InContext(ctx))

	// Should return an error when trying to create a session that already exists
	_, err = NewSession(ctx)
	require.ErrorIs(t, err, ErrSessionAlreadyExists)
}

func TestInContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Should return an error when trying to get session from context without one
	_, err := fromContext(ctx)
	require.ErrorIs(t, err, ErrNoSessionInContext)

	require.False(t, InContext(ctx))

	ctx, err = NewSession(ctx)
	require.NoError(t, err)

	require.True(t, InContext(ctx))
}

func TestReqCache_NewObject(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	obj, err := cache.NewObject(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, obj.value)
}

func TestReqCache_Exists(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	err = cache.Put(ctx, key, value)
	require.NoError(t, err)

	found, err := cache.Exists(ctx, key)
	require.NoError(t, err)
	require.True(t, found)

	found, err = cache.Exists(ctx, "key2")
	require.NoError(t, err)
	require.False(t, found)
}

func TestReqCache_PutAndGet(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	err = cache.Put(ctx, key, value)
	require.NoError(t, err)

	retrievedValue, found, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, retrievedValue)

	found, err = cache.Exists(ctx, key)
	require.NoError(t, err)
	require.True(t, found)

	const nonExistentKey = "key2"
	_, found, err = cache.Get(ctx, nonExistentKey)
	require.NoError(t, err)
	require.False(t, found)

	deleted, err := cache.Delete(ctx, key)
	require.NoError(t, err)
	require.True(t, deleted)

	found, err = cache.Exists(ctx, key)
	require.NoError(t, err)
	require.False(t, found)
}

func TestReqCache_Delete(t *testing.T) {
	t.Parallel()
	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	key := "key1"
	value := &reqCacheTestObject{value: 100}
	err = cache.Put(ctx, key, value)
	require.NoError(t, err)

	retrievedValue, found, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, retrievedValue)

	err = cache.EndSession(ctx)
	require.NoError(t, err)

	_, found, err = cache.Get(ctx, key)
	require.NoError(t, err)
	require.False(t, found)
}

func TestNewObject(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	// Ensure that we can create new objects without overflowing the pool
	var prevObj *reqCacheTestObject
	for range 20 {
		obj, err := cache.NewObject(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, obj.value, "New object should have a value of 0")

		if prevObj == obj {
			t.Fatalf("New object should not be the same as the previous one")
		}

		prevObj = obj
	}

	// Ensure that the object pool is reset after clearing the cache
	err = cache.EndSession(ctx)
	require.NoError(t, err)
	require.Empty(t, cache.objects, "Object pool should be empty after cache is cleared")
}

func TestReqCache_GetOrFetch(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}

	// Fetcher function that returns the value
	fetcher := func(context.Context) (*reqCacheTestObject, error) {
		return value, nil
	}

	retrievedValue, err := cache.GetOrFetch(ctx, key, fetcher)
	require.NoError(t, err)
	require.Equal(t, value, retrievedValue)

	// Ensure value is correctly stored in the cache
	cachedValue, found, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, cachedValue)

	// Validate that fetcher is not called again and the cached value is returned
	newValue, err := cache.GetOrFetch(ctx, key,
		func(context.Context) (*reqCacheTestObject, error) {
			return &reqCacheTestObject{value: 200}, nil
		})
	require.NoError(t, err)
	require.Equal(t, value, newValue)

	// Ensure that error is returned if fetcher returns an error
	_, err = cache.GetOrFetch(ctx, "key2",
		func(context.Context) (*reqCacheTestObject, error) {
			return nil, errors.New("fetcher error")
		})
	require.Error(t, err)
}

func TestReqCache_GetOrNew(t *testing.T) {
	t.Parallel()

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	cache, err := New[string, reqCacheTestObject](10, 10)
	require.NoError(t, err)

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
	cachedValue, found, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
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

	ctx, err := NewSession(context.Background())
	require.NoError(t, err)

	logger := &mockLogger{}
	cache, err := New[string, reqCacheTestObject](1, 1, WithLogger("test", logger))
	require.NoError(t, err)

	const key = "key1"
	value := &reqCacheTestObject{value: 100}
	err = cache.Put(ctx, key, value)
	require.NoError(t, err)

	// Ensure that we get object from the cache
	retrievedValue, found, err := cache.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, value, retrievedValue)
	require.Equal(t, &mockLogger{name: "test", objHit: 0, objMiss: 0, cacheHit: 1, cacheMiss: 0}, logger)

	// Not found in the cache
	_, found, err = cache.Get(ctx, "key2")
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, &mockLogger{name: "test", objHit: 0, objMiss: 0, cacheHit: 1, cacheMiss: 1}, logger)
}

func TestAsyncReqCache(t *testing.T) {
	t.Parallel()

	const (
		nParallel = 100
		objCount  = 1000
	)

	var (
		errGroup   errgroup.Group
		cache, err = New[string, reqCacheTestObject](objCount, objCount)
	)
	require.NoError(t, err)

	// Ensure that we can work with multiple threads without interference between them
	for range nParallel {
		errGroup.Go(func() error {
			ctx, err := NewSession(context.Background())
			if err != nil {
				return err
			}
			defer func() {
				_ = cache.EndSession(ctx)
			}()

			objects := make([]*reqCacheTestObject, objCount)

			for k := range objCount {
				key := "key" + strconv.Itoa(k)
				obj, err := cache.NewObject(ctx)
				if err != nil {
					return err
				}
				obj.value = k
				err = cache.Put(ctx, key, obj)
				if err != nil {
					return err
				}
				objects[k] = obj
			}

			for k := range objCount {
				key := "key" + strconv.Itoa(k)
				v, found, err := cache.Get(ctx, key)
				if err != nil {
					return err
				}
				if !found {
					return fmt.Errorf("value not found, expected %d", k)
				}

				if v.value != k {
					return fmt.Errorf("value mismatch, expected %d, got %d", k, v.value)
				}

				if v != objects[k] {
					return fmt.Errorf("object mismatch, expected %p, got %p", objects[k], v)
				}
			}

			reqID, err := fromContext(ctx)
			if err != nil {
				return err
			}

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
