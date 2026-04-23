package wal

import "github.com/YanBatytskiy/in_memory_base/internal/concurrency"

// WriteRequest couples a [Log] entry with a one-shot Promise that conveys
// the durability result back to the caller waiting inside [Wal.Set] or
// [Wal.Del].
type WriteRequest struct {
	log     Log
	promise concurrency.PromiseError
}

// NewWriteRequest constructs a request with a freshly allocated promise.
func NewWriteRequest(lsn int64,
	commandID int, arguments []string,
) *WriteRequest {
	return &WriteRequest{
		log: Log{
			LSN:       lsn,
			CommandID: commandID,
			Arguments: arguments,
		},
		promise: concurrency.NewPromise[error](),
	}
}

// SetResponse resolves the request's promise with err.
func (writeRequest *WriteRequest) SetResponse(err error) {
	writeRequest.promise.Set(err)
}

// FutureResponse returns a Future that will receive the outcome of the
// WAL flush. Calling [concurrency.Future.Get] on it blocks until the flush
// completes.
func (writeRequest *WriteRequest) FutureResponse() concurrency.FutureError {
	return writeRequest.promise.GetFuture()
}
