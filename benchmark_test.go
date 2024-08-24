package reqcache

import (
	"context"
	"testing"
)

// Define a simple object type for benchmarking purposes.
type BenchObject struct {
	Data [1024]byte // Simulate a sizable object
}

const opCount = 10000

// Benchmark without ReqCache - Creating objects directly.
func BenchmarkWithoutBatchAllocation(b *testing.B) {
	var (
		obj    *BenchObject
		ctx    context.Context
		cancel context.CancelFunc
	)

	for n := 0; n < b.N; n++ {
		ctx, cancel = context.WithCancel(context.Background())
		for i := 0; i < opCount; i++ {
			obj = new(BenchObject)
		}
		cancel()
	}

	_ = obj
	_ = ctx
}

// Benchmark with ReqCache - Using ReqCache to create objects.
func BenchmarkWithBatchAllocation(b *testing.B) {
	var (
		obj    *BenchObject
		ctx    context.Context
		cancel context.CancelFunc
	)

	cache := New[string, BenchObject](opCount, 0)

	for n := 0; n < b.N; n++ {
		ctx, cancel = context.WithCancel(context.Background())
		ctx = NewSession(ctx)

		for i := 0; i < opCount; i++ {
			obj = cache.NewObject(ctx)
		}

		// Delete
		cache.EndSession(ctx)

		cancel()
	}

	_ = obj
	_ = ctx
}
