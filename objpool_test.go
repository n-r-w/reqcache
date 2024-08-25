//nolint:exhaustruct // tests
package reqcache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewObjectPool(t *testing.T) {
	t.Parallel()

	pool := newObjectPool[int]("testPool", 10, nil)

	require.NotNil(t, pool, "New object pool should not be nil")
	require.Len(t, pool.data, 10, "New object pool should have the correct size")
	require.Equal(t, 0, pool.index, "New object pool should have an initial index of 0")
	require.Equal(t, "testPool", pool.name, "New object pool should have the correct name")
	require.Nil(t, pool.logger, "New object pool should have a nil logger")
}

func TestObjectPoolGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	pool := newObjectPool[int]("testPool", 2, nil)

	require.Len(t, pool.data, 2, "Object pool should have 2 elements")

	// Get objects from the pool
	obj1 := pool.get(ctx)
	require.NotNil(t, obj1, "Object 1 should not be nil")
	require.Equal(t, 1, pool.index, "Pool index should be incremented after getting an object")
	require.Same(t, obj1, &pool.data[0], "Object 1 pointer should be equal to the first element of the pool")

	obj2 := pool.get(ctx)
	require.NotNil(t, obj2, "Object 2 should not be nil")
	require.Equal(t, 2, pool.index, "Pool index should be incremented after getting an object")
	require.Same(t, obj2, &pool.data[1], "Object 2 pointer should be equal to the second element of the pool")

	// Pool exceeds its capacity, new object gets created
	obj3 := pool.get(ctx)
	require.NotNil(t, obj3, "Object 3 should not be nil")
	require.Equal(t, 2, pool.index, "Pool index should not be incremented after exceeding capacity")
	require.NotSame(t, obj3, &pool.data[0], "Object 3 pointer should not be equal to the first element of the pool")
	require.NotSame(t, obj3, &pool.data[1], "Object 3 pointer should not be equal to the second element of the pool")
}

func TestObjectPoolOverflowLogging(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	logger := &mockLogger{}
	pool := newObjectPool[int]("testPool", 1, logger)

	// Fill the pool
	pool.get(ctx)
	require.Equal(t, &mockLogger{name: "testPool", objHit: 1, objMiss: 0}, logger)

	// This should exceed the pool
	pool.get(ctx)
	require.Equal(t, &mockLogger{name: "testPool", objHit: 1, objMiss: 1}, logger)
}

func TestObjectSyncPoolReuse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	syncPool := newObjectSyncPool[int]()

	// Request an object from the sync pool
	const objCount = 10

	pool1 := syncPool.Get("testSyncPool", objCount, nil)
	for i := 0; i < objCount; i++ {
		obj := pool1.get(ctx)
		*obj = i + 1
	}

	// Put the pool back
	syncPool.Put(pool1)

	// Request another object pool, it should reuse the previous pool and not reallocate memory
	pool2 := syncPool.Get("testSyncPool", objCount/2, nil)
	require.Same(t, pool1, pool2, "Reused object pool should be the same as the previous pool")
	require.Equal(t, 0, pool2.index, "Reused object pool should have an initial index of 0")
	require.Len(t, pool2.data, objCount/2, "Reused object pool should have the correct size")

	// Check that the objects are cleared
	for i := 0; i < objCount/2; i++ {
		obj := pool2.get(ctx)
		require.Equal(t, 0, *obj, "Object should be cleared")
	}
}
