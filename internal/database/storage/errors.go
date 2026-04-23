package storage

import "errors"

// Sentinel errors returned by the storage façade.
var (
	// ErrInvalidLogger is returned by [NewStorage] when the logger is nil.
	ErrInvalidLogger = errors.New("storage.NewStorage: logger is invalid")
	// ErrKeyNotFound is returned by [Storage.Get] when the key is absent.
	// Callers should test for it with [errors.Is].
	ErrKeyNotFound = errors.New("key not found")
)
