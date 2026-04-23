// Package concurrency hosts the small synchronisation primitives the rest
// of the service builds on:
//
//   - Promise[T] / Future[T] — a one-shot, typed write/read pair used by
//     the WAL to hand the flush outcome back to blocked callers.
//   - Semaphore — a counting semaphore built on sync.Cond, used by the TCP
//     server to bound accepted connections.
//   - WithLock — a tiny helper that runs a closure under a sync.Locker.
package concurrency

// PromiseError is a convenience alias used throughout the WAL: its result
// type is an error (nil on success).
type PromiseError = Promise[error]

// Promise is the write side of a one-shot typed channel. It is resolved
// exactly once; subsequent Set calls are silently ignored. Use
// [NewPromise] to construct one, [Promise.GetFuture] to hand a Future to
// readers, and [Promise.Set] to publish the value.
//
// The zero value is not usable; the internal channel must be allocated by
// [NewPromise].
type Promise[T any] struct {
	result   chan T
	promised bool
}

// NewPromise returns a fresh, unresolved Promise[T].
func NewPromise[T any]() Promise[T] {
	return Promise[T]{
		result: make(chan T, 1),
	}
}

// Set publishes value to the promise. The first call delivers the value
// and closes the channel; subsequent calls are no-ops so producers can
// signal completion idempotently.
func (promise *Promise[T]) Set(value T) {
	if promise.promised {
		return
	}

	promise.promised = true
	promise.result <- value
	close(promise.result)
}

// GetFuture returns a [Future] that will observe the eventual value.
// Multiple futures may be derived from the same promise, but since the
// channel is closed after the single Set, only one Get will receive the
// value; the rest will observe the zero value.
func (promise *Promise[T]) GetFuture() Future[T] {
	return NewFuture(promise.result)
}
