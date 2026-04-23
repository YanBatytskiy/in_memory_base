package inmemory

import (
	"math"
	"sync/atomic"
)

// IDGenerator produces monotonically increasing int64 identifiers, used as
// Log Sequence Numbers (LSN) for write operations. Safe for concurrent use;
// the underlying counter wraps back to 1 after [math.MaxInt64] to keep the
// id space non-negative.
type IDGenerator struct {
	counter atomic.Int64
}

// NewIDGenerator returns a generator seeded with previousID (the next
// Generate call will return previousID+1). Pass 0 to start from 1.
func NewIDGenerator(previousID int64) *IDGenerator {
	generator := &IDGenerator{}
	generator.counter.Store(previousID)
	return generator
}

// Generate returns the next identifier.
func (gen *IDGenerator) Generate() int64 {
	gen.counter.CompareAndSwap(math.MaxInt64, 0)
	return gen.counter.Add(1)
}
