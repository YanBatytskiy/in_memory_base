package inmemory

import (
	"strconv"
	"testing"
)

// BenchmarkHashTable_Set measures the cost of a single Set under the
// write lock on a table that starts empty.
func BenchmarkHashTable_Set(b *testing.B) {
	h := NewHashTable()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		h.Set(strconv.Itoa(i), "v")
	}
}

// BenchmarkHashTable_Get measures the cost of a single Get under the
// read lock on a pre-populated table.
func BenchmarkHashTable_Get(b *testing.B) {
	const seed = 10_000

	h := NewHashTable()
	for i := range seed {
		h.Set(strconv.Itoa(i), "v")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		_, _ = h.Get(strconv.Itoa(i % seed))
	}
}

// BenchmarkHashTable_Del measures the cost of Del on an always-empty
// slot (still acquires the write lock and calls builtin delete).
func BenchmarkHashTable_Del(b *testing.B) {
	h := NewHashTable()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		h.Del(strconv.Itoa(i))
	}
}

// BenchmarkHashTable_ConcurrentReadWrite mixes reads and writes across
// GOMAXPROCS goroutines, showing how the RWMutex behaves under a read-
// heavy (75% Get, 25% Set) workload on a hot working set of 1000 keys.
func BenchmarkHashTable_ConcurrentReadWrite(b *testing.B) {
	const hot = 1000

	h := NewHashTable()
	for i := range hot {
		h.Set(strconv.Itoa(i), "v")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := strconv.Itoa(i % hot)
			if i%4 == 0 {
				h.Set(key, "v")
			} else {
				_, _ = h.Get(key)
			}
			i++
		}
	})
}
