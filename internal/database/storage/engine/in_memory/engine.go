// Package inmemory is the storage engine implementation.
//
// It combines a concurrent hash table ([HashTable]) with a write-ahead log
// ([wal.Wal]): writes are first appended to the WAL and then applied to the
// hash table, while reads are served directly from the hash table. On
// startup the WAL is replayed by [wal.Wal.Recovery] to rebuild the in-memory
// state.
package inmemory

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/wal"
	contextid "github.com/YanBatytskiy/in_memory_base/internal/lib/context_util"
)

// Sentinel errors returned by the engine.
var (
	// ErrFailedGenerateLSN means the monotonic LSN generator produced 0,
	// which is treated as an invalid id.
	ErrFailedGenerateLSN = errors.New("failed to generate LSN")
	// ErrCreateWal is returned by [NewEngine] when a required dependency
	// (WAL or hash table) is nil.
	ErrCreateWal = errors.New("failed create wal")
)

// Engine is the storage engine used by the rest of the service. Writes go
// through the WAL; reads hit the hash table directly. Safe for concurrent
// use.
type Engine struct {
	log           *slog.Logger
	wal           *wal.Wal
	currentLSN    *IDGenerator
	commandEngine CommandEngine
	queryEngine   QueryEngine
}

// CommandEngine is the write-side dependency of Engine. In production it is
// satisfied by [wal.Wal]; tests substitute a mock.
type CommandEngine interface {
	Set(ctx context.Context, key, value string) error
	Del(ctx context.Context, key string) error
}

// QueryEngine is the read-side dependency of Engine. It is intentionally a
// concrete type (not an interface) because reads bypass the WAL and go
// straight to the in-memory hash table.
type QueryEngine struct {
	hashTable *HashTable
}

// NewEngine assembles an Engine from its dependencies. It returns
// [ErrInvalidLogger] if log is nil and [ErrCreateWal] if wal or hashTable
// is nil.
func NewEngine(ctx context.Context, log *slog.Logger, cfg *config.Config, wal *wal.Wal, hashTable *HashTable) (*Engine, error) {
	if log == nil {
		return nil, ErrInvalidLogger
	}
	if wal == nil {
		return nil, ErrCreateWal
	}
	if hashTable == nil {
		return nil, ErrCreateWal
	}

	return &Engine{
		log:           log,
		wal:           wal,
		commandEngine: wal,
		currentLSN:    NewIDGenerator(0),
		queryEngine:   QueryEngine{hashTable: hashTable},
	}, nil
}

// Set assigns a fresh LSN to the write and forwards it to the WAL, which
// durably logs the operation before applying it to the hash table.
func (engine *Engine) Set(ctx context.Context, key, value string) error {
	txID := engine.currentLSN.Generate()
	if txID == 0 {
		engine.log.Debug("engine failed to generate LSN")
		return ErrFailedGenerateLSN
	}

	ctxCommand := context.WithValue(ctx, contextid.TxIDKey, strconv.FormatInt(txID, 10))
	return engine.commandEngine.Set(ctxCommand, key, value)
}

// Del assigns a fresh LSN to the deletion and forwards it to the WAL.
func (engine *Engine) Del(ctx context.Context, key string) error {
	txID := engine.currentLSN.Generate()
	if txID == 0 {
		engine.log.Debug("engine failed to generate LSN")
		return ErrFailedGenerateLSN
	}

	ctxCommand := context.WithValue(ctx, contextid.TxIDKey, strconv.FormatInt(txID, 10))
	return engine.commandEngine.Del(ctxCommand, key)
}

// Get returns the current value for key or [storage.ErrKeyNotFound] if
// the key is absent.
func (engine *Engine) Get(ctx context.Context, key string) (string, error) {
	_ = ctx

	result, ok := engine.queryEngine.hashTable.Get(key)
	if !ok {
		return "", storage.ErrKeyNotFound
	}
	return result, nil
}
