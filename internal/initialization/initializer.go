// Package initialization is the composition root for the server binary.
//
// It reads the validated [config.Config], builds the logger, WAL, hash
// table, engine, TCP server and Database in the right order, and exposes a
// single [Initializer.StartDatabase] entry point that blocks until the
// server exits.
package initialization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database"
	"github.com/YanBatytskiy/in_memory_base/internal/database/compute"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage"
	inmemory "github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

// Initializer owns the fully-wired dependency graph needed to run the
// server. Construct one with [NewInitializer] and drive it with
// [Initializer.StartDatabase].
type Initializer struct {
	Log    *slog.Logger
	engine *inmemory.Engine
	server *network.TCPServer
}

// NewInitializer builds every component the server depends on: logger,
// WAL + hash table, engine, TCP listener. WAL recovery runs here, so the
// returned Initializer reflects a fully rebuilt in-memory state.
func NewInitializer(ctx context.Context, cfg *config.Config) (*Initializer, error) {
	const op = "initialization.NewInitializer"

	if cfg == nil {
		return nil, fmt.Errorf("%s: failed to initialize: config is invalid", op)
	}

	log, err := CreateLogger(cfg)
	if err != nil {
		return nil, err
	}

	wal, ht, err := CreateWal(ctx, log, cfg)
	if err != nil {
		log.Debug("failed to create Wal",
			slog.String("operation", op),
			slog.String("error", err.Error()))
		return nil, err
	}

	engine, err := CreateEngine(ctx, log, cfg, wal, ht)
	if err != nil {
		log.Debug("failed to create storage engine", slog.String("error", err.Error()))
		return nil, fmt.Errorf("%s: failed to create storage engine", op)
	}

	server, err := CreateTCPNetwork(ctx, log, cfg.Network)
	if err != nil {
		return nil, err
	}

	return &Initializer{
		Log:    log,
		engine: engine,
		server: server,
	}, nil
}

// StartDatabase wires the compute, storage and database layers, then blocks
// serving TCP clients inside an [errgroup]. It returns when ctx is
// cancelled or the server loop fails.
func (init *Initializer) StartDatabase(ctx context.Context) error {
	const op = "initialization.StartDatabase"

	compute, err := compute.NewCompute(init.Log)
	if err != nil {
		init.Log.Debug("failed to create compute layer",
			slog.String("operation", op),
			slog.String("error", err.Error()))
		return errors.New("failed to create compute layer")
	}

	storage, err := storage.NewStorage(init.Log, init.engine)
	if err != nil {
		init.Log.Debug("failed to create storage layer",
			slog.String("operation", op),
			slog.String("error", err.Error()))
		return errors.New("failed to create storage layer")
	}

	database, err := database.NewDatabase(init.Log, compute, storage)
	if err != nil {
		init.Log.Debug("failed to create database",
			slog.String("operation", op),
			slog.String("error", err.Error()))
		return fmt.Errorf("%s: failed to create database", op)
	}

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		init.server.HandleClientQueries(groupCtx, func(ctx context.Context, query []byte) []byte {
			response := database.DatabaseHandler(ctx, string(query))
			return []byte(response)
		})
		return nil
	})

	err = group.Wait()
	return err
}
