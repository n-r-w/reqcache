package reqcache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockLogger is a mock implementation of the iLogger interface for testing purposes.
type mockLogger struct {
	logCalled bool
	name      string
	size      int
}

func (m *mockLogger) LogObjectPoolOverflow(_ context.Context, name string, size int) {
	m.logCalled = true
	m.name = name
	m.size = size
}

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

	logger := &mockLogger{
		logCalled: false,
		name:      "",
		size:      0,
	}
	pool := newObjectPool[int]("testPool", 1, logger)

	// Fill the pool
	pool.get(ctx)
	require.False(t, logger.logCalled, "Logger should not be called before overflow")

	// This should exceed the pool and trigger the logger
	pool.get(ctx)
	require.True(t, logger.logCalled, "Logger should be called after overflow")
	require.Equal(t, "testPool", logger.name, "Logger should receive the correct pool name")
	require.Equal(t, 1, logger.size, "Logger should receive the correct pool size")
}
