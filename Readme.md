[![Go Reference](https://pkg.go.dev/badge/github.com/n-r-w/reqcache.svg)](https://pkg.go.dev/github.com/n-r-w/reqcache)
[![Go Coverage](https://github.com/n-r-w/reqcache/wiki/coverage.svg)](https://raw.githack.com/wiki/n-r-w/reqcache/coverage.html)
![CI Status](https://github.com/n-r-w/reqcache/actions/workflows/go.yml/badge.svg)
[![Stability](http://badges.github.io/stability-badges/dist/stable.svg)](http://github.com/badges/stability-badges)
[![Go Report](https://goreportcard.com/badge/github.com/n-r-w/reqcache)](https://goreportcard.com/badge/github.com/n-r-w/reqcache)

# ReqCache

ReqCache is a Go package for caching data within a single request context.
Usable in web servers, gRPC servers, and other applications with a request-response lifecycle.
It allows to:

- Pre-allocate a single block of memory for creating multiple objects and reduce the load on the garbage collector
- Cache objects by unique keys

## Installation

To install the package, you need to run:

```sh
go get -u github.com/n-r-w/reqcache@latest
```

## Benchmarks with and without batch allocation

```sh
BenchmarkWithoutBatchAllocation-32          747    1513720 ns/op 10240114 B/op    10002 allocs/op
BenchmarkWithBatchAllocation-32            4189     251629 ns/op     2598 B/op        3 allocs/op
```

## Key Type Performance

When choosing cache key types, string keys generally perform better than struct keys:

```sh
BenchmarkStringKey-16    	   61869	     18234 ns/op
BenchmarkStructKey-16    	   44949	     26602 ns/op
```

String keys are ~31% faster due to more efficient hashing and comparison operations in Go's map implementation.

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

cache, err := reqcache.New[KeyType, ObjectType](
    preAllocatedObjects, maxCacheSize,    
    reqcache.WithLogger("cache name", logger), // used for logging/metrics pre-allocated memory overflow and cache hits   
)
```

### Start a new session

NewSession adds a new session key to the context. It must be called once at the beginning of the request processing.

```go
ctx, err := reqcache.NewSession(ctx)
if err != nil {
    // handle error
    panic(err)
}
```

### End the session

EndSession removes all cache data from the reqcache object, associated with the session key.

```go
defer func() {
    if err := cache.EndSession(ctx); err != nil {
        // handle error
        log.Printf("Error ending session: %v", err)
    }
}()
```

### Create a new object

NewObject takes a pointer to object from the pre-allocated memory.
If no pre-allocated memory is available (because too many objects have already been taken from the cache), a new object is created.
In case of an object pool overflow, the logger will be called.

```go
newObj, err := cache.NewObject(ctx)
```

### Put an object into the cache

Put adds an object to the cache by a unique key.

```go
if err := cache.Put(ctx, key, newObj); err != nil {
    // handle error
    panic(err)
}
```

### Get an object from the cache

Get returns an object from the cache by a unique key.

```go
obj, ok, err := cache.Get(ctx, key)
```

### Other methods

- `Exists` checks if an object exists in the cache.
- `Delete` removes an object from the cache.
- `GetOrFetch` returns data from the cache or fetches it from the fetcher function (for example, from a database).
- `GetOrNew` returns data from the cache or creates it and prepares with the prepare function.

## Example

```go
//nolint:gochecknoglobals,revive // example
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

    cache, err := reqcache.New[myKey, myObject](
        objSize, cacheSize,
        reqcache.WithLogger("example", &myLogger{}), // logging and metrics for object pool overflows
    )
    if err != nil {
        log.Fatal(err)
    }

    http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Prepare context for cache operations
        ctx, err := reqcache.NewSession(r.Context())
        if err != nil {
            http.Error(w, "Failed to create session", http.StatusInternalServerError)
            return
        }

        // clean up the cache data after the request
        defer func() {
            if err := cache.EndSession(ctx); err != nil {
                log.Printf("Error ending session: %v", err)
            }
        }()

        // Simulate some data processing
        if err := workFunc1(ctx, cache); err != nil {
            http.Error(w, "Processing error", http.StatusInternalServerError)
            return
        }
        if err := workFunc2(ctx, cache); err != nil {
            http.Error(w, "Processing error", http.StatusInternalServerError)
            return
        }

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

func workFunc1(ctx context.Context, cache *MyCache) error {
    // Create a new object from the pre-allocated memory
    newObj1, err := cache.NewObject(ctx)
    if err != nil {
        return err
    }
    // Set the value
    newObj1.Value = "Hello, World 1!"

    // Put the object into the cache
    if err := cache.Put(ctx, dataKey1, newObj1); err != nil {
        return err
    }

    // Create another object manually
    newObj2 := &myObject{Value: "Hello, World 2!"}

    // Put the object into the cache
    if err := cache.Put(ctx, dataKey2, newObj2); err != nil {
        return err
    }
    
    return nil
}

func workFunc2(ctx context.Context, cache *MyCache) error {
    // obj1 is cached
    obj1, found, err := cache.Get(ctx, dataKey1)
    if err != nil {
        return err
    }
    if found {
        log.Println("obj1 is cached:", obj1.Value)
    }

    // obj3 is not cached. will be fetched and cached
    obj3, err := cache.GetOrFetch(ctx, dataKey3,
        func(_ context.Context) (*myObject, error) {
            // fetching data
            return &myObject{Value: "Hello, World 3!"}, nil
        })
    if err != nil {
        return err
    }
    log.Println("obj3 is fetched:", obj3.Value)

    return workFunc3(ctx, cache)
}

func workFunc3(ctx context.Context, cache *MyCache) error {
    // obj3 is cached
    obj3, found, err := cache.Get(ctx, dataKey3)
    if err != nil {
        return err
    }
    if found {
        log.Println("obj3 is cached:", obj3.Value)
    }

    // obj4 is not cached. will be created by NewObject, prepared and cached
    obj4, err := cache.GetOrNew(ctx, dataKey4,
        func(_ context.Context, obj *myObject) error {
            // preparing data
            obj.Value = "Hello, World 4!"
            return nil
        })
    if err != nil {
        return err
    }
    log.Println("obj4 is created by NewObject and prepared:", obj4.Value)
    
    return nil
}

// myLogger is a custom logger for ReqCache. It logs object pool overflows and cache hits.
// Not required.
type myLogger struct{}

func (m *myLogger) LogObjectPoolHitRatio(_ context.Context, name string, hit bool) {
    log.Printf("Object pool hit: %s, hit: %v", name, hit)
}

func (m *myLogger) LogCacheHitRatio(_ context.Context, name string, hit bool) {
    log.Printf("Cache hit: %s, hit: %v", name, hit)
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
