package reqcache

import (
	"context"
	"errors"
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
func NewSession(ctx context.Context) (context.Context, error) {
	if InContext(ctx) {
		return nil, ErrSessionAlreadyExists
	}

	return context.WithValue(ctx, contextKey, atomic.AddUint64(&requestID, 1)), nil
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

// New creates a new instance of ReqCache.
// objSize is the size of the array of objects of type T, preallocating memory for them.
// cacheSize is the size of the cache in a single request.
func New[K comparable, T any](objSize, cacheSize int, opts ...Option) (*ReqCache[K, T], error) {
	m := &ReqCache[K, T]{
		op:          options{}, //nolint:exhaustruct // default values
		cacheSize:   cacheSize,
		objSize:     objSize,
		objectsPool: nil,
		dataPool:    newPoolWrapper[K, T](cacheSize),
		objects:     make(map[uint64]*objectPool[T]),
		data:        make(map[uint64]*lru.Cache[K, *T]),
		muData:      sync.RWMutex{},
		muObjects:   sync.Mutex{},
	}

	for _, opt := range opts {
		opt(&m.op)
	}

	if err := m.validate(); err != nil {
		return nil, err
	}

	m.objectsPool = newObjectSyncPool[T](m.op.name, m.objSize, m.op.logger)

	return m, nil
}

// validate validates the ReqCache configuration.
func (m *ReqCache[K, T]) validate() error {
	if m.cacheSize <= 0 {
		return errors.New("cache size must be greater than 0")
	}

	if m.objSize <= 0 {
		return errors.New("object size must be greater than 0")
	}

	if m.op.logger != nil && m.op.name == "" {
		return errors.New("operation name must be set when logger is provided")
	}

	return nil
}

// NewObject creates a new object of type T.
func (m *ReqCache[K, T]) NewObject(ctx context.Context) (*T, error) {
	requestKey, err := fromContext(ctx)
	if err != nil {
		return nil, err
	}

	m.muObjects.Lock()
	defer m.muObjects.Unlock()

	p, ok := m.objects[requestKey]
	if !ok {
		p = m.objectsPool.Get()
		m.objects[requestKey] = p
	}

	return p.get(ctx), nil
}

// Put saves data in the cache.
func (m *ReqCache[K, T]) Put(ctx context.Context, dataKey K, data *T) error {
	requestKey, err := fromContext(ctx)
	if err != nil {
		return err
	}

	m.muData.Lock()
	defer m.muData.Unlock()

	d, ok := m.data[requestKey]
	if !ok {
		d = m.dataPool.Get()
		m.data[requestKey] = d
	}

	d.Add(dataKey, data)

	return nil
}

// Exists checks if the data exists in the cache.
func (m *ReqCache[K, T]) Exists(ctx context.Context, dataKey K) (
	found bool, err error,
) {
	if m.op.logger != nil {
		defer func() { m.op.logger.LogCacheHitRatio(ctx, m.op.name, found) }()
	}

	requestKey, err := fromContext(ctx)
	if err != nil {
		return false, err
	}

	m.muData.RLock()
	defer m.muData.RUnlock()

	d, ok := m.data[requestKey]
	if !ok {
		return false, nil
	}

	return d.Contains(dataKey), nil
}

// Delete deletes data from the cache.
func (m *ReqCache[K, T]) Delete(ctx context.Context, dataKey K) (bool, error) {
	requestKey, err := fromContext(ctx)
	if err != nil {
		return false, err
	}

	m.muData.Lock()
	defer m.muData.Unlock()

	d, ok := m.data[requestKey]
	if !ok {
		return false, nil
	}

	return d.Remove(dataKey), nil
}

// Get returns data from the cache.
func (m *ReqCache[K, T]) Get(ctx context.Context, dataKey K) (obj *T, found bool, err error) {
	if m.op.logger != nil {
		defer func() { m.op.logger.LogCacheHitRatio(ctx, m.op.name, found) }()
	}

	requestKey, err := fromContext(ctx)
	if err != nil {
		return nil, false, err
	}

	m.muData.RLock()
	defer m.muData.RUnlock()

	data, ok := m.data[requestKey]
	if !ok {
		return nil, false, nil
	}

	obj, found = data.Get(dataKey)
	return obj, found, nil
}

// GetOrFetch returns data from the cache or fetches it from the fetcher function,
// for example, from the database.
func (m *ReqCache[K, T]) GetOrFetch(ctx context.Context, dataKey K,
	fetcher func(context.Context) (*T, error),
) (*T, error) {
	v, ok, err := m.Get(ctx, dataKey)
	if err != nil {
		return nil, err
	}

	if ok {
		return v, nil
	}

	obj, err := fetcher(ctx)
	if err != nil {
		return nil, err
	}

	if err := m.Put(ctx, dataKey, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// GetOrNew returns data from the cache or creates it and prepares with the prepare function.
func (m *ReqCache[K, T]) GetOrNew(ctx context.Context, dataKey K, prepare func(context.Context, *T) error) (*T, error) {
	v, ok, err := m.Get(ctx, dataKey)
	if err != nil {
		return nil, err
	}

	if ok {
		return v, nil
	}

	obj, err := m.NewObject(ctx)
	if err != nil {
		return nil, err
	}

	if err := prepare(ctx, obj); err != nil {
		return nil, err
	}

	if err := m.Put(ctx, dataKey, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// EndSession deletes data from the cache.
// It is recommended to call EndSession in the defer statement.
// After calling EndSession, the cache object with the session context key is no longer usable.
func (m *ReqCache[K, T]) EndSession(ctx context.Context) error {
	requestKey, err := fromContext(ctx)
	if err != nil {
		return err
	}

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

	return nil
}

type contextKeyType struct{}

//nolint:gochecknoglobals // ок for context key
var (
	contextKey = contextKeyType{}
	requestID  uint64
)

// fromContext returns the key from the context.
func fromContext(ctx context.Context) (uint64, error) {
	if ctx == nil {
		return 0, ErrNoSessionInContext
	}

	v, ok := ctx.Value(contextKey).(uint64)
	if !ok {
		return 0, ErrNoSessionInContext
	}

	return v, nil
}
