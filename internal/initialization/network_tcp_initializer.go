package initialization

import (
	"context"
	"log/slog"
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

// Defaults applied by [CreateTCPNetwork] when the config value is zero.
var (
	defaultIdleTimeout    = time.Duration(10 * time.Minute)
	defaultMaxConnections = 10
	defaultBufferSize     = 4096
	defaultServerAddress  = "127.0.0.1:3323"
)

// CreateTCPNetwork builds a [network.TCPServer] from a [config.NetworkConfig],
// substituting package-level defaults for any zero-valued field. ctx is used
// to bound the underlying listener setup.
func CreateTCPNetwork(ctx context.Context, log *slog.Logger, cfg *config.NetworkConfig) (*network.TCPServer, error) {
	if log == nil {
		return nil, network.ErrInvalidLogger
	}

	if cfg == nil {
		return nil, network.ErrInvalidConfig
	}

	var options []network.TCPServeOption

	if cfg.IdleTimeout != 0 {
		options = append(options, network.WithServerTCPIdleTimeout(cfg.IdleTimeout))
	} else {
		options = append(options, network.WithServerTCPIdleTimeout(time.Duration(defaultIdleTimeout)))
	}

	if cfg.MaxConnections != 0 {
		options = append(options, network.WithServerTCPMaxConnectionNumber(cfg.MaxConnections))
	} else {
		options = append(options, network.WithServerTCPMaxConnectionNumber(defaultMaxConnections))
	}

	if cfg.BufferSize != 0 {
		options = append(options, network.WithServerTCPBufferSize(cfg.BufferSize))
	} else {
		options = append(options, network.WithServerTCPBufferSize(defaultBufferSize))
	}

	address := ""

	if cfg.Address == "" {
		address = defaultServerAddress
	} else {
		address = cfg.Address
	}

	return network.NewTCPServer(ctx, address, log, options...)
}
