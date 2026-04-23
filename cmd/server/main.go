// Command server starts the in-memory key/value database daemon.
//
// It loads configuration from CONFIG_PATH (falling back to defaults), wires
// all internal components through the initialization package, and serves TCP
// clients until the process receives SIGINT or SIGTERM. On shutdown it waits
// up to five seconds for in-flight requests and the WAL to drain.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/initialization"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bootstrap := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.NewConfig()
	if err != nil {
		bootstrap.Error("failed to load config", slog.String("error", err.Error()))
		stop()
		//nolint:gocritic // stop() is called explicitly right above; defer is a safety net
		os.Exit(1)
	}

	init, err := initialization.NewInitializer(ctx, cfg)
	if err != nil {
		bootstrap.Error("failed to initialize", slog.String("error", err.Error()))
		stop()
		os.Exit(1)
	}

	bootstrap.Info("Server initialized. Server started.")
	go func() {
		err = init.StartDatabase(ctx)
		if err != nil {
			init.Log.Error("failed to start database", slog.String("error", err.Error()))
			stop()
		}
	}()

	waitForShutdown(ctx, init.Log)
	init.Log.Info("service stopped")
}

func waitForShutdown(
	ctx context.Context,
	log *slog.Logger,
) {
	<-ctx.Done()
	log.Info("received shutdown signal")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	<-shutdownCtx.Done()
	log.Info("shutdown complete")
}
