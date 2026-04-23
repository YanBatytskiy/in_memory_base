// Package storage is a thin façade above the underlying engine.
//
// It exposes Set / Get / Del and is responsible for logging and translating
// engine errors into the sentinel [ErrKeyNotFound]. All state lives in the
// engine; Storage itself is stateless and safe for concurrent use.
package storage

import (
	"context"
	"errors"
	"log/slog"
)

// Storage wires an engine into the Database layer. It separates the
// write-side and read-side interfaces so tests can mock each independently.
type Storage struct {
	log            *slog.Logger
	commandStorage CommandStorage
	queryStorage   QueryStorage
}

//go:generate go run github.com/vektra/mockery/v3@v3.6.1 --config ../../../.mockery.yaml

// CommandStorage is the write-side contract of the engine.
type CommandStorage interface {
	Set(ctx context.Context, key, value string) error
	Del(ctx context.Context, key string) error
}

// QueryStorage is the read-side contract of the engine.
type QueryStorage interface {
	Get(ctx context.Context, key string) (string, error)
}

// NewStorage wraps an engine that satisfies both [CommandStorage] and
// [QueryStorage] and returns a Storage ready for use. It returns
// [ErrInvalidLogger] if log is nil.
func NewStorage(log *slog.Logger, eng interface {
	CommandStorage
	QueryStorage
},
) (*Storage, error) {
	if log == nil {
		return nil, ErrInvalidLogger
	}

	return &Storage{
		log:            log,
		commandStorage: eng,
		queryStorage:   eng,
	}, nil
}

// Set writes value under key, delegating to the underlying engine.
func (s *Storage) Set(ctx context.Context, key, value string) error {
	err := s.commandStorage.Set(ctx, key, value)
	if err != nil {
		s.log.Error("set failed", slog.String("key", key), slog.Any("err", err))
		return err
	}
	return nil
}

// Get returns the value stored under key, or [ErrKeyNotFound] if absent.
func (s *Storage) Get(ctx context.Context, key string) (string, error) {
	result, err := s.queryStorage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			s.log.Debug("get not found", slog.String("key", key))
		} else {
			s.log.Error("get failed", slog.String("key", key), slog.Any("err", err))
		}
		return "", err
	}
	return result, nil
}

// Del removes key from the store. Deleting an absent key is logged at Debug
// level but does not return an error to the caller.
func (s *Storage) Del(ctx context.Context, key string) error {
	err := s.commandStorage.Del(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			s.log.Debug("del not found", slog.String("key", key))
		} else {
			s.log.Error("del failed", slog.String("key", key), slog.Any("err", err))
		}
		return err
	}
	return nil
}
