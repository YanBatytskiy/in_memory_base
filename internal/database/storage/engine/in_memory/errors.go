package inmemory

import "errors"

// ErrInvalidLogger is returned by [NewEngine] when the logger is nil.
var ErrInvalidLogger = errors.New("invalid logger")
