// Package database is the top-level request dispatcher of the service.
//
// It parses an incoming text command through the compute layer, then routes
// SET / GET / DEL to the storage layer and formats the wire-level response
// string.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/YanBatytskiy/in_memory_base/internal/database/compute"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage"
)

//go:generate go run github.com/vektra/mockery/v3@v3.6.1 --config ../../.mockery.yaml

// ComputeLayer parses and validates a raw text command into tokens.
// The first token is the command name (SET / GET / DEL); the remaining
// tokens are its arguments.
type ComputeLayer interface {
	ParseAndValidate(_ context.Context, raw string) ([]string, error)
}

// StorageLayer is the write/read surface that Database delegates to.
// Implementations are expected to be safe for concurrent use.
type StorageLayer interface {
	Set(ctx context.Context, key, value string) error
	Del(ctx context.Context, key string) error
	Get(ctx context.Context, key string) (string, error)
}

// Database wires a ComputeLayer and a StorageLayer together and exposes a
// single entry point, [Database.DatabaseHandler], which is used by the
// network layer as a TCP request handler.
type Database struct {
	Compute ComputeLayer
	Storage StorageLayer
	log     *slog.Logger
}

// NewDatabase wires a compute layer, a storage layer and a logger into a
// Database. It returns an error if any of the three dependencies is nil.
func NewDatabase(
	log *slog.Logger,
	computeLayer ComputeLayer,
	storageLayer StorageLayer,
) (*Database, error) {
	const op = "database.NewDatabase"

	if computeLayer == nil {
		return nil, fmt.Errorf("%s: compute is invalid", op)
	}
	if storageLayer == nil {
		return nil, fmt.Errorf("%s: storage is invalid", op)
	}
	if log == nil {
		return nil, fmt.Errorf("%s: logger is invalid", op)
	}
	return &Database{
		Compute: computeLayer,
		Storage: storageLayer,
		log:     log,
	}, nil
}

// DatabaseHandler accepts a raw text command, parses and validates it, then
// dispatches it to the appropriate storage operation. It returns the string
// that will be sent back to the client; errors are also returned as strings
// so the TCP layer can pass them through unchanged.
func (db *Database) DatabaseHandler(ctx context.Context, raw string) string {
	const op = "database.handler"

	tokens, err := db.Compute.ParseAndValidate(ctx, raw)
	if err != nil {
		db.log.Debug("failed to parse command", slog.String("operation", op), slog.String("error", err.Error()))
		return "failed to parse command"
	}

	db.log.Debug("command start", slog.String("cmd", tokens[0]))

	switch tokens[0] {
	case compute.CommandSet:
		return db.handleSet(ctx, tokens)
	case compute.CommandGet:
		return db.handleGet(ctx, tokens)
	case compute.CommandDel:
		return db.handleDel(ctx, tokens)
	default:
		db.log.Debug("invalid command")

		return "invalid command"
	}
}

func (db *Database) handleSet(ctx context.Context, tokens []string) string {
	const op = "database.handleSet"

	if len(tokens)-1 != compute.CommandSetQ {
		db.log.Debug("must be two arguments", slog.String("operation", op))
		return "must be two arguments"
	}

	err := db.Storage.Set(ctx, tokens[1], tokens[2])
	if err != nil {
		db.log.Debug("failed SET", slog.String("operation", op), slog.Any("error", err))
		return "failed SET"
	}

	db.log.Info("command success", slog.String("cmd", tokens[0]), slog.String("key", tokens[1]))
	return "OK"
}

func (db *Database) handleGet(ctx context.Context, tokens []string) string {
	const op = "database.handleGet"

	if len(tokens)-1 != compute.CommandGetQ {
		db.log.Debug("must be one argument", slog.String("operation", op))
		return "must be one argument"
	}

	result, err := db.Storage.Get(ctx, tokens[1])
	if err != nil {
		if errors.Is(err, storage.ErrKeyNotFound) {
			return "NOT_FOUND"
		}
		db.log.Debug("failed GET", slog.String("operation", op), slog.Any("error", err))
		return "failed GET"
	}

	db.log.Debug("command success", slog.String("cmd", tokens[0]), slog.String("key", tokens[1]))
	return "VALUE " + result
}

func (db *Database) handleDel(ctx context.Context, tokens []string) string {
	const op = "database.handleDel"

	if len(tokens)-1 != compute.CommandDelQ {
		db.log.Debug("must be one argument", slog.String("operation", op))
		return "must be one argument"
	}

	err := db.Storage.Del(ctx, tokens[1])
	if err != nil {
		db.log.Debug("failed DEL", slog.String("operation", op), slog.Any("error", err))
		return "failed DEL"
	}

	db.log.Debug("command success", slog.String("cmd", tokens[0]), slog.String("key", tokens[1]))
	return "DELETED"
}
