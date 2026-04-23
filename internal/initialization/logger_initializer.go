package initialization

import (
	"log/slog"
	"os"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogpretty"
)

// Supported values for [config.LoggingConfig.Level].
const (
	// LoggerLevelInfo enables Info+ logs via the pretty handler.
	LoggerLevelInfo = "info"
	// LoggerLevelDev enables Debug+ logs via the pretty handler.
	LoggerLevelDev = "dev"
	// LoggerLevelProd disables all logging via the discard handler.
	LoggerLevelProd = "prod"
)

// CreateLogger builds a [slog.Logger] that matches cfg.Logger.Level.
// Unknown levels fall back to Info with the pretty handler.
func CreateLogger(cfg *config.Config) (*slog.Logger, error) {
	var log *slog.Logger

	switch cfg.Logger.Level {
	case LoggerLevelInfo:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelInfo},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)

	case LoggerLevelDev:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelDebug},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)

	case LoggerLevelProd:
		log = slogdiscard.NewDiscardLogger()
	default:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelInfo},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)

	}
	log.Info("starting service", slog.String("logger level", cfg.Logger.Level))
	return log, nil
}
