package reqcache

import "context"

// IReqCache is an interface for caching data within a single request.
// For convenience of testing and replacing the implementation.
type IReqCache[K comparable, T any] interface {
	NewObject(ctx context.Context) *T
	Put(ctx context.Context, dataKey K, data *T)
	Exists(ctx context.Context, dataKey K) (found bool)
	Delete(ctx context.Context, dataKey K) bool
	Get(ctx context.Context, dataKey K) (obj *T, found bool)
	GetOrFetch(ctx context.Context, dataKey K, fetcher func(context.Context) (*T, error)) (*T, error)
	GetOrNew(ctx context.Context, dataKey K, prepare func(context.Context, *T) error) (*T, error)
	EndSession(ctx context.Context)
}
