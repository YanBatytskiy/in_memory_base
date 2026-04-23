package network

import "errors"

// Sentinel errors returned by the network package.
var (
	// ErrInvalidLogger is returned by [NewTCPServer] and [NewTCPClient]
	// when the logger is nil.
	ErrInvalidLogger = errors.New("invalid logger")
	// ErrInvalidConfig is returned by the initialization layer when the
	// network section of the config is absent.
	ErrInvalidConfig = errors.New("invalid config")
	// ErrInvalidMaxConn is returned by [NewTCPServer] when
	// [WithServerTCPMaxConnectionNumber] was not supplied.
	ErrInvalidMaxConn = errors.New("max connections is equal zero")
	// ErrInvalidBufferSize is returned by [NewTCPServer] when
	// [WithServerTCPBufferSize] was not supplied.
	ErrInvalidBufferSize = errors.New("buffer size is equal zero")
)
