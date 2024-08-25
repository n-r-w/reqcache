package reqcache

import (
	"context"
	"sync"
)

// ILogger is an interface for logging new object pool overflows.
type ILogger interface {
	LogObjectPoolOverflow(ctx context.Context, name string, size int)
}

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
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index >= len(p.data) {
		if p.logger != nil {
			p.logger.LogObjectPoolOverflow(ctx, p.name, len(p.data))
		}

		return new(T)
	}

	res := &p.data[p.index]
	p.index++

	return res
}

// objectSyncPool is a wrapper around sync.Pool.
type objectSyncPool[T any] struct {
	pool *sync.Pool
}

// newObjectSyncPool creates a new objectSyncPool.
func newObjectSyncPool[T any]() *objectSyncPool[T] {
	return &objectSyncPool[T]{
		pool: &sync.Pool{
			New: func() any {
				return &objectPool[T]{ //nolint:exhaustruct // default values
					mu: sync.Mutex{},
				}
			},
		},
	}
}

// Get returns an object from the pool.
func (w *objectSyncPool[T]) Get(name string, size int, logger ILogger) *objectPool[T] {
	o, _ := w.pool.Get().(*objectPool[T])
	o.index = 0
	o.name = name
	o.logger = logger
	o.mu = sync.Mutex{}

	if cap(o.data) < size {
		o.data = make([]T, size)
	} else {
		o.data = o.data[:size]
		var zero T
		for i := 0; i < size; i++ {
			o.data[i] = zero
		}
	}

	return o
}

// Put puts an object in the pool.
func (w *objectSyncPool[T]) Put(v *objectPool[T]) {
	v.logger = nil

	w.pool.Put(v)
}
