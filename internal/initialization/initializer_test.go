package initialization

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	inmemory "github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

type stubListener struct{}

func (s *stubListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (s *stubListener) Close() error              { return nil }
func (s *stubListener) Addr() net.Addr            { return &net.TCPAddr{} }

func isOpNotPermitted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "operation not permitted")
}

func setServerField(t *testing.T, srv *network.TCPServer, name string, value interface{}) {
	t.Helper()

	v := reflect.ValueOf(srv).Elem().FieldByName(name)
	require.Truef(t, v.IsValid(), "field %s not found", name)
	ptr := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	ptr.Set(reflect.ValueOf(value))
}

func newBaseConfig(t *testing.T) *config.Config {
	t.Helper()

	walDir := t.TempDir()
	walFile := filepath.Join(walDir, "wal_0001.log")
	require.NoError(t, os.WriteFile(walFile, nil, 0o644))

	return &config.Config{
		Network: &config.NetworkConfig{
			Address:        "127.0.0.1:0",
			MaxConnections: 2,
			MaxMessageSize: 16,
			IdleTimeout:    50 * time.Millisecond,
			BufferSize:     16,
			TypeConn:       "tcp",
		},
		Logger: &config.LoggingConfig{
			Level: LoggerLevelProd,
		},
		Wal: &config.WalConfig{
			FlushingBatchTimeout: 10 * time.Millisecond,
			FlushingBatchCount:   1,
			MaxSegmentSize:       1024,
			SegmentStoragePath:   walDir,
			MaskName:             "wal_",
		},
		EngineType: "in_memory",
	}
}

func newInitializerWithStubServer(t *testing.T) *Initializer {
	t.Helper()

	log := slogdiscard.NewDiscardLogger()
	srv := &network.TCPServer{}

	setServerField(t, srv, "listener", net.Listener(&stubListener{}))
	setServerField(t, srv, "log", log)
	setServerField(t, srv, "bufferSize", 1)
	setServerField(t, srv, "maxConnections", 1)

	return &Initializer{
		Log:    log,
		engine: &inmemory.Engine{},
		server: srv,
	}
}

func TestNewInitializer_Table(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name       string
		cfgFn      func(t *testing.T) *config.Config
		wantErr    bool
		errCheckFn func(t *testing.T, err error)
	}

	tests := []testCase{
		{
			name: "nil config",
			cfgFn: func(t *testing.T) *config.Config {
				t.Helper()
				return nil
			},
			wantErr: true,
			errCheckFn: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "failed to initialize")
			},
		},
		{
			name: "nil network config",
			cfgFn: func(t *testing.T) *config.Config {
				t.Helper()
				cfg := newBaseConfig(t)
				cfg.Network = nil
				return cfg
			},
			wantErr: true,
			errCheckFn: func(t *testing.T, err error) {
				require.ErrorIs(t, err, network.ErrInvalidConfig)
			},
		},
		{
			name: "invalid wal path",
			cfgFn: func(t *testing.T) *config.Config {
				t.Helper()
				cfg := newBaseConfig(t)
				filePath := filepath.Join(t.TempDir(), "wal-file")
				require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o644))
				cfg.Wal.SegmentStoragePath = filePath
				return cfg
			},
			wantErr: true,
			errCheckFn: func(t *testing.T, err error) {
				require.ErrorIs(t, err, filesystem.ErrFailedToCreateDirectory)
			},
		},
		{
			name: "invalid network address",
			cfgFn: func(t *testing.T) *config.Config {
				t.Helper()
				cfg := newBaseConfig(t)
				cfg.Network.Address = "bad:addr"
				return cfg
			},
			wantErr: true,
		},
		{
			name: "success",
			cfgFn: func(t *testing.T) *config.Config {
				t.Helper()
				return newBaseConfig(t)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.cfgFn(t)
			init, err := NewInitializer(context.Background(), cfg)
			if err != nil && isOpNotPermitted(err) {
				t.Skip("net.Listen not permitted in this environment")
			}

			if tc.wantErr {
				require.Error(t, err)
				if tc.errCheckFn != nil {
					tc.errCheckFn(t, err)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, init)
			require.NotNil(t, init.Log)
			require.NotNil(t, init.engine)
			require.NotNil(t, init.server)

			listener := extractListener(t, init.server)
			_ = listener.Close()
		})
	}
}

func TestInitializer_StartDatabase_Table(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		cancelBefore bool
		cancelDelay  time.Duration
	}

	tests := []testCase{
		{
			name:         "context canceled before start",
			cancelBefore: true,
		},
		{
			name:        "context canceled after start",
			cancelDelay: 20 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			init := newInitializerWithStubServer(t)
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancelBefore {
				cancel()
			}
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- init.StartDatabase(ctx)
			}()

			if !tc.cancelBefore {
				//nolint:forbidigo // deliberate pause to test cancellation timing
				time.Sleep(tc.cancelDelay)
				cancel()
			}

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("StartDatabase did not return after cancel")
			}
		})
	}
}
