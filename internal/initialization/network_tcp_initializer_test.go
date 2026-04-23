package initialization

import (
	"log/slog"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

func TestCreateNetwork_Table(t *testing.T) {
	type testCase struct {
		name       string
		loggerNil  bool
		cfg        *config.NetworkConfig
		wantErr    error
		expectErr  bool
		wantMax    int
		wantBuf    int
		wantIdle   time.Duration
		wantAddrFn func(net.Addr) bool
	}

	tests := []testCase{
		{
			name:      "nil logger",
			loggerNil: true,
			cfg:       &config.NetworkConfig{},
			wantErr:   network.ErrInvalidLogger,
		},
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: network.ErrInvalidConfig,
		},
		{
			name: "negative values fallback",
			cfg: &config.NetworkConfig{
				Address:        "127.0.0.1:0",
				MaxConnections: -1,
				BufferSize:     -1,
				IdleTimeout:    -time.Second,
			},
			wantMax:  100,
			wantBuf:  4098,
			wantIdle: 5 * time.Minute,
		},
		{
			name:     "uses defaults when zero",
			cfg:      &config.NetworkConfig{},
			wantMax:  defaultMaxConnections,
			wantBuf:  defaultBufferSize,
			wantIdle: defaultIdleTimeout,
			wantAddrFn: func(a net.Addr) bool {
				return a.String() == defaultServerAddress
			},
		},
		{
			name: "uses config values",
			cfg: &config.NetworkConfig{
				Address:        "127.0.0.1:0",
				MaxConnections: 5,
				BufferSize:     128,
				IdleTimeout:    time.Second,
			},
			wantMax:  5,
			wantBuf:  128,
			wantIdle: time.Second,
		},
		{
			name: "invalid address returns error",
			cfg: &config.NetworkConfig{
				Address:        "bad:addr",
				MaxConnections: 1,
				BufferSize:     1,
				IdleTimeout:    time.Second,
			},
			expectErr: true, // net.Listen returns error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var logger *slog.Logger
			if !tc.loggerNil {
				logger = slogdiscard.NewDiscardLogger()
			}

			srv, err := CreateTCPNetwork(t.Context(), logger, tc.cfg)

			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			if tc.expectErr {
				require.Error(t, err)
				return
			}

			if err != nil && strings.Contains(err.Error(), "operation not permitted") {
				t.Skip("net.Listen not permitted in this environment")
			}

			require.NoError(t, err)
			require.NotNil(t, srv)

			ln := extractListener(t, srv)
			if tc.wantAddrFn != nil {
				require.True(t, tc.wantAddrFn(ln.Addr()))
			}
			if tc.wantMax != 0 {
				require.Equal(t, tc.wantMax, extractIntField(t, srv, "maxConnections"))
			}
			if tc.wantBuf != 0 {
				require.Equal(t, tc.wantBuf, extractIntField(t, srv, "bufferSize"))
			}
			if tc.wantIdle != 0 {
				require.Equal(t, tc.wantIdle, extractDurationField(t, srv, "idleTimeout"))
			}

			_ = ln.Close()
		})
	}
}

func extractListener(t *testing.T, srv *network.TCPServer) net.Listener {
	t.Helper()
	v := reflect.ValueOf(srv).Elem().FieldByName("listener")
	ptr := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	return ptr.Interface().(net.Listener)
}

func extractIntField(t *testing.T, srv *network.TCPServer, name string) int {
	t.Helper()
	v := reflect.ValueOf(srv).Elem().FieldByName(name)
	ptr := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	return int(ptr.Int())
}

func extractDurationField(t *testing.T, srv *network.TCPServer, name string) time.Duration {
	t.Helper()
	v := reflect.ValueOf(srv).Elem().FieldByName(name)
	ptr := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	return time.Duration(ptr.Int())
}
