package reqcache

import (
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
)

// cachePool is a wrapper around sync.Pool.
type cachePool[K comparable, T any] struct {
	pool *sync.Pool
}

// newPoolWrapper creates a new poolWrapper.
func newPoolWrapper[K comparable, T any](size int) *cachePool[K, T] {
	return &cachePool[K, T]{
		pool: &sync.Pool{
			New: func() any {
				c, err := lru.New[K, *T](size)
				if err != nil {
					// we can't recover from this error, so panic
					// in practice, this should never happen due to validation in New
					panic(fmt.Errorf("failed to create poolWrapper: %w", err))
				}
				return c
			},
		},
	}
}

// Get returns an object from the pool.
func (w *cachePool[K, T]) Get() *lru.Cache[K, *T] {
	return w.pool.Get().(*lru.Cache[K, *T])
}

// Put puts an object in the pool.
func (w *cachePool[K, T]) Put(v *lru.Cache[K, *T]) {
	v.Purge()
	w.pool.Put(v)
}
