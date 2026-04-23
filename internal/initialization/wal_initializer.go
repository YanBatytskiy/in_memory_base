package initialization

import (
	"context"
	"errors"
	"log/slog"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	inmemory "github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/wal"
)

// ErrInvalidLogger is returned by [CreateWal] when the logger is nil.
var ErrInvalidLogger = errors.New("invalid logger")

// CreateWal resolves the segment directory, builds the hash table and WAL,
// replays existing segments, and launches the background flusher goroutine.
// Returns the WAL and the hash table so the caller can wire the engine.
func CreateWal(ctx context.Context, log *slog.Logger, cfg *config.Config) (*wal.Wal, *inmemory.HashTable, error) {
	if log == nil {
		return nil, nil, ErrInvalidLogger
	}

	directory, err := filesystem.MakeDirectory(log, cfg.Wal.SegmentStoragePath)
	if err != nil {
		return nil, nil, err
	}

	hashTable := inmemory.NewHashTable()

	segment := filesystem.NewSegment(log, directory, cfg.Wal.MaskName, cfg.Wal.MaxSegmentSize)

	wal, err := wal.NewWal(log, cfg.Wal, segment, hashTable)
	if err != nil {
		return nil, nil, err
	}

	err = wal.Recovery(directory)
	if err != nil {
		return nil, nil, err
	}

	go wal.Start(ctx)

	return wal, hashTable, nil
}
