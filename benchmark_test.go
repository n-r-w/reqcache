package reqcache

import (
	"context"
	"strconv"
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

	for i := 0; i < b.N; i++ {
		ctx, cancel = context.WithCancel(context.Background())
		for range opCount {
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

	cache, _ := New[string, BenchObject](opCount, 0)

	for i := 0; i < b.N; i++ {
		ctx, cancel = context.WithCancel(context.Background())
		ctx, err := NewSession(ctx)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}

		for range opCount {
			obj, err := cache.NewObject(ctx)
			if err != nil {
				b.Fatalf("Failed to create object: %v", err)
			}
			_ = obj // Use obj to avoid unused variable error
		}

		// Delete
		if err := cache.EndSession(ctx); err != nil {
			b.Fatalf("Failed to end session: %v", err)
		}

		cancel()
	}

	_ = obj
	_ = ctx
}

// Define struct key type for benchmarking.
type StructKey struct {
	Field1 string
	Field2 string
	Field3 int64
	Field4 int64
}

// Helper function to create a composite string key.
func createStringKey(field1, field2 string, field3, field4 int64) string {
	return field1 + ":" + field2 + ":" + strconv.FormatInt(field3, 10) + ":" + strconv.FormatInt(field4, 10)
}

// Benchmark with string key - One Put followed by multiple Gets.
func BenchmarkStringKey(b *testing.B) {
	var (
		obj    *BenchObject
		ctx    context.Context
		cancel context.CancelFunc
		err    error
	)

	// Test data for the key
	field1 := "test_string_1"
	field2 := "test_string_2"
	field3 := int64(12345)
	field4 := int64(67890)
	getCount := 1000 // Number of Get operations per Put

	cache, _ := New[string, BenchObject](opCount, opCount)

	for i := 0; i < b.N; i++ {
		ctx, cancel = context.WithCancel(context.Background())
		ctx, err = NewSession(ctx)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}

		// Create the string key
		key := createStringKey(field1, field2, field3, field4)

		// One Put operation
		obj = &BenchObject{}
		if err := cache.Put(ctx, key, obj); err != nil {
			b.Fatalf("Failed to put object: %v", err)
		}

		// Multiple Get operations
		for range getCount {
			_, found, err := cache.Get(ctx, key)
			if err != nil {
				b.Fatalf("Failed to get object: %v", err)
			}
			if !found {
				b.Fatal("Object not found in cache")
			}
		}

		// End session
		if err := cache.EndSession(ctx); err != nil {
			b.Fatalf("Failed to end session: %v", err)
		}

		cancel()
	}

	_ = obj
	_ = ctx
}

// Benchmark with struct key - One Put followed by multiple Gets.
func BenchmarkStructKey(b *testing.B) {
	var (
		obj    *BenchObject
		ctx    context.Context
		cancel context.CancelFunc
		err    error
	)

	// Test data for the key
	field1 := "test_string_1"
	field2 := "test_string_2"
	field3 := int64(12345)
	field4 := int64(67890)
	getCount := 1000 // Number of Get operations per Put

	cache, _ := New[StructKey, BenchObject](opCount, opCount)

	for i := 0; i < b.N; i++ {
		ctx, cancel = context.WithCancel(context.Background())
		ctx, err = NewSession(ctx)
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}

		// Create the struct key
		key := StructKey{
			Field1: field1,
			Field2: field2,
			Field3: field3,
			Field4: field4,
		}

		// One Put operation
		obj = &BenchObject{}
		if err := cache.Put(ctx, key, obj); err != nil {
			b.Fatalf("Failed to put object: %v", err)
		}

		// Multiple Get operations
		for range getCount {
			_, found, err := cache.Get(ctx, key)
			if err != nil {
				b.Fatalf("Failed to get object: %v", err)
			}
			if !found {
				b.Fatal("Object not found in cache")
			}
		}

		// End session
		if err := cache.EndSession(ctx); err != nil {
			b.Fatalf("Failed to end session: %v", err)
		}

		cancel()
	}

	_ = obj
	_ = ctx
}
