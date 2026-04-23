// Package config loads and validates server configuration.
//
// Values are read from the YAML file pointed to by the CONFIG_PATH
// environment variable; when CONFIG_PATH is empty, defaults and env-based
// overrides are applied instead. Validation is performed with
// [go-playground/validator].
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/ilyakaznacheev/cleanenv"
)

// Config is the top-level configuration struct. All nested pointers are
// non-nil after [NewConfig] returns.
type Config struct {
	Network *NetworkConfig `yaml:"network"`
	Logger  *LoggingConfig `yaml:"logging"`
	Wal     *WalConfig     `yaml:"wal"`

	EngineType string `yaml:"engine_type" env-default:"in_memory"`
}

// NetworkConfig holds the TCP listener parameters.
type NetworkConfig struct {
	Address        string        `yaml:"engine_address"  env-default:"127.0.0.1:3323"`
	MaxConnections int           `yaml:"max_connections" env-default:"100" validate:"gt=0"`
	MaxMessageSize int           `yaml:"max_message_size" env-default:"4096" validate:"gt=0"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"    env-default:"5m" validate:"gt=0"`
	BufferSize     int           `yaml:"buffer_size"     env-default:"4096" validate:"gt=0"`
	TypeConn       string        `yaml:"type" env-default:"tcp"`
}

// LoggingConfig selects the slog handler; see [initialization.LoggerLevel*]
// for the recognised values.
type LoggingConfig struct {
	Level string `yaml:"level" env-default:"info"`
}

// WalConfig controls WAL batching and segment rotation. See
// [internal/database/storage/engine/in_memory/wal] for semantics.
type WalConfig struct {
	FlushingBatchTimeout time.Duration `yaml:"flushing_batch_timeout" env-default:"10ms" validate:"gt=0"`
	FlushingBatchCount   int           `yaml:"flushing_batch_count" env-default:"100" validate:"gt=0"`
	FlushingBatchVolume  int           `yaml:"flushing_batch_volume" env-default:"10485760" validate:"gt=0"`
	MaxSegmentSize       int64         `yaml:"max_segment_size" env-default:"1073741824" validate:"gt=0"`
	SegmentStoragePath   string        `yaml:"segment_storage_path" env-default:"./storage/wal" validate:"required"`
	MaskName             string        `yaml:"mask_name" env-default:"wal_" validate:"required"`
}

// NewConfig loads configuration from CONFIG_PATH or from environment
// variables, applies the hard-coded defaults for any unset field, and
// validates the result. Returns a fully-populated Config or a wrapped
// error describing what failed.
func NewConfig() (*Config, error) {
	const op = "config.NewConfig"

	configPath := os.Getenv("CONFIG_PATH")

	cfg := Config{
		Network: &NetworkConfig{
			Address:        "127.0.0.1:3323",
			MaxConnections: 100,
			MaxMessageSize: 4096,
			IdleTimeout:    5 * time.Minute,
			BufferSize:     4096,
			TypeConn:       "tcp",
		},
		EngineType: "in_memory",
		Logger: &LoggingConfig{
			Level: "info",
		},
		Wal: &WalConfig{
			FlushingBatchTimeout: 10 * time.Millisecond,
			FlushingBatchCount:   2,
			FlushingBatchVolume:  10485760,   // 10Mb
			MaxSegmentSize:       1073741824, // 1Gb
			SegmentStoragePath:   "./storage/wal",
			MaskName:             "segment_",
		},
	}
	if configPath != "" {
		//nolint:gosec // CONFIG_PATH is set by the operator who launches the binary; no untrusted party controls this value
		_, err := os.Stat(configPath)
		if err != nil {
			return nil, fmt.Errorf("%s: wrong path: error accessing config file", op)
		}

		err = cleanenv.ReadConfig(configPath, &cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: error reading config file", op)
		}
	} else {
		err := cleanenv.ReadEnv(&cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: error reading config from env variables", op)
		}
	}

	err := validator.New().Struct(&cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: config validation error: %w", op, err)
	}

	return &cfg, nil
}
