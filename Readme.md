[![Go Reference](https://pkg.go.dev/badge/github.com/n-r-w/reqcache.svg)](https://pkg.go.dev/github.com/n-r-w/reqcache)
[![Go Coverage](https://github.com/n-r-w/reqcache/wiki/coverage.svg)](https://raw.githack.com/wiki/n-r-w/reqcache/coverage.html)
![CI Status](https://github.com/n-r-w/reqcache/actions/workflows/go.yml/badge.svg)
[![Stability](http://badges.github.io/stability-badges/dist/stable.svg)](http://github.com/badges/stability-badges)
[![Go Report](https://goreportcard.com/badge/github.com/n-r-w/reqcache)](https://goreportcard.com/badge/github.com/n-r-w/reqcache)

# ReqCache

ReqCache is a Go package for caching data within a single request context.
It allows to:

- Pre-allocate a single block of memory for creating multiple objects and reduce the load on the garbage collector
- Cache objects within a single request by unique keys

## Installation

To install the package, you need to run:

```sh
go get -u github.com/n-r-w/reqcache
```

## Benchmarks with and without batch allocation

```sh
BenchmarkWithoutBatchAllocation-32          747    1513720 ns/op 10240114 B/op    10002 allocs/op
BenchmarkWithBatchAllocation-32            4189     251629 ns/op     2598 B/op        3 allocs/op
```

## Usage

### Create a reqcache object

Cache object can be shared between multiple requests. It is recommended to create a single cache object for each unique type of object and key.

```go
const (
    // number of pre-allocated objects. can be 0 if pre-allocation is not needed
    preAllocatedObjects = 1000
    // maximum number of objects in the cache. can be 0 if the cache not needed
    maxCacheSize = 10000
)

cache := reqcache.New[KeyType, ObjectType](
    preAllocatedObjects, maxCacheSize,    
    reqcache.WithLogger("cache name", logger), // used for logging/metrics pre-allocated memory overflow    
)
```

### Start a new session

NewSession adds a new session key to the context. It must be called once at the beginning of the request processing.

```go
ctx = reqcache.NewSession(ctx)
```

### End the session

EndSession removes all cache data from the reqcache object, associated with the session key.

```go
defer cache.EndSession(ctx)
```

### Create a new object

NewObject takes a pointer to object from the pre-allocated memory.
If no pre-allocated memory is available (because too many objects have already been taken from the cache), a new object is created.
In case of an object pool overflow, the logger will be called.

```go
newObj := cache.NewObject(ctx)
```

### Put an object into the cache

Put adds an object to the cache by a unique key.

```go
cache.Put(ctx, key, newObj)
```

### Get an object from the cache

Get returns an object from the cache by a unique key.

```go
obj, ok := cache.Get(ctx, key)
```

### Other methods

- `Exists` checks if an object exists in the cache.
- `Delete` removes an object from the cache.
- `GetOrFetch` returns data from the cache or fetches it from the fetcher function.
- `GetOrNew` returns data from the cache or creates it and prepares with the prepare function.

## Example

```go
//nolint:gochecknoglobals // ок example
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"

    "github.com/n-r-w/reqcache"
)

type MyCache = reqcache.ReqCache[myKey, myObject]

func main() {
    const (
        objSize   = 5  // number of pre-allocated objects
        cacheSize = 10 // maximum number of objects in the cache

    )

    cache := reqcache.New[myKey, myObject](
        objSize, cacheSize,
        reqcache.WithLogger("example", &myLogger{}), // logging and metrics for object pool overflows
    )

    http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Prepare context for cache operations
        ctx := reqcache.NewSession(r.Context())

        // clean up the cache data after the request
        defer cache.EndSession(ctx)

        // Simulate some data processing
        workFunc1(ctx, cache)
        workFunc2(ctx, cache)

        _, _ = fmt.Fprintf(w, "request processed")

        log.Println("Request processed")
    }))

    log.Println("Server started at http://127.0.0.1:8080")

    if err := http.ListenAndServe(":8080", nil); err != nil { //nolint:gosec // no need for example
        log.Fatal(err)
    }
}

var (
    dataKey1 = myKey{Key1: "some-key11", Key2: "some-key12"}
    dataKey2 = myKey{Key1: "some-key21", Key2: "some-key22"}
    dataKey3 = myKey{Key1: "some-key31", Key2: "some-key32"}
    dataKey4 = myKey{Key1: "some-key41", Key2: "some-key42"}
)

func workFunc1(ctx context.Context, cache *MyCache) {
    // Create a new object from the pre-allocated memory
    newObj1 := cache.NewObject(ctx)
    // Set the value
    newObj1.Value = "Hello, World 1!"

    // Put the object into the cache
    cache.Put(ctx, dataKey1, newObj1)

    // Create another object manually
    newObj2 := &myObject{Value: "Hello, World 2!"}

    // Put the object into the cache
    cache.Put(ctx, dataKey2, newObj2)
}

func workFunc2(ctx context.Context, cache *MyCache) {
    // obj1 is cached
    if obj1, ok := cache.Get(ctx, dataKey1); ok {
        log.Println("obj1 is cached:", obj1.Value)
    }

    // obj3 is not cached. will be fetched and cached
    if obj3, err := cache.GetOrFetch(ctx, dataKey3,
        func(_ context.Context, _ *MyCache) (*myObject, error) {
            // fetching data
            return &myObject{Value: "Hello, World 3!"}, nil
        }); err != nil {
        log.Println(err.Error())
    } else {
        log.Println("obj3 is fetched:", obj3.Value)
    }

    workFunc3(ctx, cache)
}

func workFunc3(ctx context.Context, cache *MyCache) {
    // obj3 is cached
    if obj3, ok := cache.Get(ctx, dataKey3); ok {
        log.Println("obj3 is cached:", obj3.Value)
    }

    // obj4 is not cached. will be created by NewObject, prepared and cached
    if obj4, err := cache.GetOrNew(ctx, dataKey4,
        func(_ context.Context, obj *myObject) error {
            // preparing data
            obj.Value = "Hello, World 4!"
            return nil
        }); err != nil {
        log.Println(err.Error())
    } else {
        log.Println("obj4 is created by NewObject and prepared:", obj4.Value)
    }
}

// myLogger is a custom logger for ReqCache. It logs object pool overflows.
// Not required.
type myLogger struct{}

// LogObjectPoolOverflow logs object pool overflows. Implements interface with single method LogObjectPoolOverflow.
func (l *myLogger) LogObjectPoolOverflow(_ context.Context, name string, size int) {
    log.Printf("Object pool overflow: %s, size: %d", name, size)
    // send metrics...
}

// myKey is a custom key type for ReqCache.
type myKey struct {
    Key1 string
    Key2 string
}

// myObject is a custom object type for ReqCache.
type myObject struct {
    Value string
}
```
