package concurrency

// FutureError is the read side of a [PromiseError].
type FutureError = Future[error]

// Future is the read side of a [Promise]. Calling Get blocks until the
// paired Promise is resolved and then returns its value.
type Future[T any] struct {
	result <-chan T
}

// NewFuture wraps an already-allocated channel. Application code normally
// obtains a Future via [Promise.GetFuture] rather than calling this
// directly.
func NewFuture[T any](result <-chan T) Future[T] {
	return Future[T]{
		result: result,
	}
}

// Get blocks until the paired Promise is resolved and returns the
// published value. Once the promise's channel is closed, subsequent Gets
// return the zero value immediately.
func (future *Future[T]) Get() T {
	return <-future.result
}
