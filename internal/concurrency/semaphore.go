package concurrency

import "sync"

// Semaphore is a counting semaphore built on [sync.Cond]. It bounds the
// number of goroutines allowed past [Semaphore.Acquire] to the limit
// passed to [NewSemaphore]. Both Acquire and Release are safe for
// concurrent use.
//
// Compared to a buffered channel of tokens it uses marginally less memory
// for large limits and surfaces the "waiting" state in a way that is easy
// to reason about.
type Semaphore struct {
	count     int
	max       int
	condition *sync.Cond
}

// NewSemaphore returns a semaphore that admits at most limit concurrent
// holders.
func NewSemaphore(limit int) *Semaphore {
	mutex := &sync.Mutex{}
	return &Semaphore{
		count:     0,
		max:       limit,
		condition: sync.NewCond(mutex),
	}
}

// Acquire blocks until a slot is available, then occupies it. Callers
// must pair every Acquire with a later [Semaphore.Release], typically via
// defer.
func (sem *Semaphore) Acquire() {
	sem.condition.L.Lock()
	defer sem.condition.L.Unlock()

	for sem.count >= sem.max {
		sem.condition.Wait()
	}

	sem.count++
}

// Release frees the slot previously taken by [Semaphore.Acquire] and
// signals one waiter, if any.
func (sem *Semaphore) Release() {
	sem.condition.L.Lock()
	defer sem.condition.L.Unlock()

	sem.count--
	sem.condition.Signal()
}
