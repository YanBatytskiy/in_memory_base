package initialization

import (
	"context"
	"log/slog"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	inmemory "github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/wal"
)

// CreateEngine wraps [inmemory.NewEngine] with a log line on failure. Used
// by [NewInitializer].
func CreateEngine(ctx context.Context, log *slog.Logger, cfg *config.Config, wal *wal.Wal, hashTable *inmemory.HashTable) (*inmemory.Engine, error) {
	engine, err := inmemory.NewEngine(ctx, log, cfg, wal, hashTable)
	if err != nil {
		log.Info("failed to create in-memory engine", slog.String("error", err.Error()))
		return nil, err
	}

	return engine, err
}
