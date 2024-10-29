package reqcache

import (
	"context"
	"sync"
)

// objectPool manages an array of objects of type T, preallocating memory for them.
type objectPool[T any] struct {
	mu    sync.Mutex
	data  []T
	index int

	name   string
	logger ILogger
}

// newObjectPool creates a new objectPool.
func newObjectPool[T any](name string, size int, logger ILogger) *objectPool[T] {
	return &objectPool[T]{
		mu:     sync.Mutex{},
		data:   make([]T, size),
		index:  0,
		name:   name,
		logger: logger,
	}
}

// get returns a pointer to a new object of type T from the array.
func (p *objectPool[T]) get(ctx context.Context) *T {
	var hit bool
	if p.logger != nil {
		defer func() { p.logger.LogObjectPoolHitRatio(ctx, p.name, hit) }()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index >= len(p.data) {
		return new(T)
	}

	res := &p.data[p.index]
	p.index++
	hit = true

	return res
}

// objectSyncPool is a wrapper around sync.Pool.
type objectSyncPool[T any] struct {
	pool *sync.Pool
}

// newObjectSyncPool creates a new objectSyncPool.
func newObjectSyncPool[T any](name string, size int, logger ILogger) *objectSyncPool[T] {
	return &objectSyncPool[T]{
		pool: &sync.Pool{
			New: func() any {
				return newObjectPool[T](name, size, logger)
			},
		},
	}
}

// Get returns an object from the pool.
func (w *objectSyncPool[T]) Get() *objectPool[T] {
	o, _ := w.pool.Get().(*objectPool[T])
	o.index = 0

	var zero T
	for i := 0; i < len(o.data); i++ {
		o.data[i] = zero
	}

	return o
}

// Put puts an object in the pool.
func (w *objectSyncPool[T]) Put(v *objectPool[T]) {
	w.pool.Put(v)
}
