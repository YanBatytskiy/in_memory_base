// Package slogdiscard provides an [slog.Handler] that drops every record.
// It is used in tests and for the "prod" logger level where logging is
// intentionally silent.
package slogdiscard

import (
	"context"
	"log/slog"
)

// NewDiscardLogger returns an [slog.Logger] that writes nowhere.
func NewDiscardLogger() *slog.Logger {
	return slog.New(NewDiscardHandler())
}

// DiscardHandler is an [slog.Handler] whose methods are all no-ops.
type DiscardHandler struct{}

// NewDiscardHandler returns a zero-value [DiscardHandler].
func NewDiscardHandler() *DiscardHandler {
	return &DiscardHandler{}
}

// Handle discards the record and returns nil.
func (h *DiscardHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

// WithAttrs returns the same handler; attributes are ignored.
func (h *DiscardHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns the same handler; groups are ignored.
func (h *DiscardHandler) WithGroup(_ string) slog.Handler {
	return h
}

// Enabled always returns false so slog skips record construction entirely.
func (h *DiscardHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}
