package reqcache

import (
	"context"
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
)

// ILogger is an interface for logging new object pool overflows and cache hit/miss ratio.
type ILogger interface {
	LogObjectPoolHitRatio(ctx context.Context, name string, hit bool)
	LogCacheHitRatio(ctx context.Context, name string, hit bool)
}

// NewSession adds a unique key for caching data in the cache.
// Must be called once at the beginning of the request processing.
func NewSession(ctx context.Context) context.Context {
	if InContext(ctx) {
		panic("context already has a reqcache key")
	}

	return context.WithValue(ctx, contextKey, atomic.AddUint64(&requestID, 1))
}

// InContext checks if there is a key for caching data in the cache.
// In other words, checks if NewSession was called.
func InContext(ctx context.Context) bool {
	return ctx.Value(contextKey) != nil
}

// ReqCache is a structure for caching data within a single request.
type ReqCache[K comparable, T any] struct {
	op options

	cacheSize int
	objSize   int

	data     map[uint64]*lru.Cache[K, *T]
	dataPool *cachePool[K, T]

	objects     map[uint64]*objectPool[T]
	objectsPool *objectSyncPool[T]

	muData    sync.RWMutex
	muObjects sync.Mutex
}

// WithLogger sets a logger for displaying/metrics new object pool overflows.
// By default, the logger is nil.
func WithLogger(name string, logger ILogger) Option {
	return func(c *options) {
		c.name = name
		c.logger = logger
	}
}

// New creates a new instance of ReqCache.
// objSize is the size of the array of objects of type T, preallocating memory for them.
// cacheSize is the size of the cache in a single request.
func New[K comparable, T any](objSize, cacheSize int, opts ...Option) *ReqCache[K, T] {
	m := &ReqCache[K, T]{
		op:          options{}, //nolint:exhaustruct // default values
		cacheSize:   cacheSize,
		objSize:     objSize,
		dataPool:    newPoolWrapper[K, T](cacheSize),
		objectsPool: newObjectSyncPool[T](),
		objects:     make(map[uint64]*objectPool[T]),
		data:        make(map[uint64]*lru.Cache[K, *T]),
		muData:      sync.RWMutex{},
		muObjects:   sync.Mutex{},
	}

	for _, opt := range opts {
		opt(&m.op)
	}

	return m
}

// NewObject creates a new object of type T.
func (m *ReqCache[K, T]) NewObject(ctx context.Context) *T {
	requestKey := fromContext(ctx)

	m.muObjects.Lock()
	defer m.muObjects.Unlock()

	p, ok := m.objects[requestKey]
	if !ok {
		p = m.objectsPool.Get(m.op.name, m.objSize, m.op.logger)
		m.objects[requestKey] = p
	}

	return p.get(ctx)
}

// Put saves data in the cache.
func (m *ReqCache[K, T]) Put(ctx context.Context, dataKey K, data *T) {
	m.checkCache()

	requestKey := fromContext(ctx)

	m.muData.Lock()
	defer m.muData.Unlock()

	d, ok := m.data[requestKey]
	if !ok {
		d = m.dataPool.Get()
		m.data[requestKey] = d
	}

	d.Add(dataKey, data)
}

// Exists checks if the data exists in the cache.
func (m *ReqCache[K, T]) Exists(ctx context.Context, dataKey K) (found bool) { //nolint:nonamedreturns // false positive
	if m.op.logger != nil {
		defer func() { m.op.logger.LogCacheHitRatio(ctx, m.op.name, found) }()
	}

	m.checkCache()

	requestKey := fromContext(ctx)

	m.muData.RLock()
	defer m.muData.RUnlock()

	d, ok := m.data[requestKey]
	if !ok {
		return false
	}

	return d.Contains(dataKey)
}

// Delete deletes data from the cache.
func (m *ReqCache[K, T]) Delete(ctx context.Context, dataKey K) bool {
	m.checkCache()

	requestKey := fromContext(ctx)

	m.muData.Lock()
	defer m.muData.Unlock()

	d, ok := m.data[requestKey]
	if !ok {
		return false
	}

	return d.Remove(dataKey)
}

// Get returns data from the cache.
func (m *ReqCache[K, T]) Get(ctx context.Context, dataKey K) (obj *T, found bool) { //nolint:nonamedreturns,lll // false positive
	if m.op.logger != nil {
		defer func() { m.op.logger.LogCacheHitRatio(ctx, m.op.name, found) }()
	}

	m.checkCache()

	requestKey := fromContext(ctx)

	m.muData.RLock()
	defer m.muData.RUnlock()

	data, ok := m.data[requestKey]
	if !ok {
		return nil, false
	}

	return data.Get(dataKey)
}

// GetOrFetch returns data from the cache or fetches it from the fetcher function.
func (m *ReqCache[K, T]) GetOrFetch(ctx context.Context, dataKey K,
	fetcher func(context.Context, *ReqCache[K, T]) (*T, error),
) (*T, error) {
	v, ok := m.Get(ctx, dataKey)
	if ok {
		return v, nil
	}

	obj, err := fetcher(ctx, m)
	if err != nil {
		return nil, err
	}

	m.Put(ctx, dataKey, obj)

	return obj, nil
}

// GetOrNew returns data from the cache or creates it and prepares with the prepare function.
func (m *ReqCache[K, T]) GetOrNew(ctx context.Context, dataKey K, prepare func(context.Context, *T) error) (*T, error) {
	v, ok := m.Get(ctx, dataKey)
	if ok {
		return v, nil
	}

	obj := m.NewObject(ctx)
	if err := prepare(ctx, obj); err != nil {
		return nil, err
	}

	m.Put(ctx, dataKey, obj)

	return obj, nil
}

// EndSession deletes data from the cache.
// It is recommended to call EndSession in the defer statement.
// After calling EndSession, the cache object with the session context key is no longer usable.
func (m *ReqCache[K, T]) EndSession(ctx context.Context) {
	requestKey := fromContext(ctx)

	m.muData.Lock()
	if v, ok := m.data[requestKey]; ok {
		delete(m.data, requestKey)
		m.dataPool.Put(v)
	}
	m.muData.Unlock()

	m.muObjects.Lock()
	if v, ok := m.objects[requestKey]; ok {
		delete(m.objects, requestKey)
		m.objectsPool.Put(v)
	}
	m.muObjects.Unlock()
}

func (m *ReqCache[K, T]) checkCache() {
	if m.cacheSize <= 0 {
		panic("cache size must be greater than 0")
	}
}

// Option is a function for configuring ReqCache.
type Option func(*options)

type options struct {
	name   string
	logger ILogger
}

type contextKeyType struct{}

//nolint:gochecknoglobals // ок for context key
var (
	contextKey = contextKeyType{}
	requestID  uint64
)

// fromContext returns the key from the context.
func fromContext(ctx context.Context) uint64 {
	if ctx == nil {
		panic("no reqcache key in context")
	}

	v, ok := ctx.Value(contextKey).(uint64)
	if !ok {
		panic("no reqcache key in context")
	}

	return v
}
