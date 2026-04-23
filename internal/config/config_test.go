package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

func TestNewConfig_Table(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  bool
		check    func(t *testing.T, cfg *Config)
	}{
		{
			name: "defaults applied when fields missing",
			contents: `
network:
  engine_address: "127.0.0.1:5555"
logging:
  level: "prod"
`,
			check: func(t *testing.T, cfg *Config) {
				require.Equal(t, "127.0.0.1:5555", cfg.Network.Address)
				require.Equal(t, 4096, cfg.Network.MaxMessageSize)
				require.Equal(t, 100, cfg.Network.MaxConnections)
				require.Equal(t, 5*time.Minute, cfg.Network.IdleTimeout)
				require.Equal(t, 4096, cfg.Network.BufferSize)
				require.Equal(t, "tcp", cfg.Network.TypeConn)
				require.Equal(t, "in_memory", cfg.EngineType)
				require.Equal(t, "prod", cfg.Logger.Level)
			},
		},
		{
			name: "validation fails on negative",
			contents: `
network:
  engine_address: "127.0.0.1:5555"
  max_connections: -1
logging:
  level: "info"
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.contents)
			t.Setenv("CONFIG_PATH", path)

			cfg, err := NewConfig()
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, cfg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestNewConfigLoadsExampleFile(t *testing.T) {
	path := filepath.Join("..", "..", "config", "yaml", "example.yaml")
	t.Setenv("CONFIG_PATH", path)

	cfg, err := NewConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, "127.0.0.1:3323", cfg.Network.Address)
	require.Equal(t, "dev", cfg.Logger.Level)
	require.Equal(t, 10*time.Millisecond, cfg.Wal.FlushingBatchTimeout)
	require.Equal(t, "segment_", cfg.Wal.MaskName)
}
