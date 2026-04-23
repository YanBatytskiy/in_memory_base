//go:build integration

package integration_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/initialization"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

// freePort picks an ephemeral port on 127.0.0.1 by opening a listener,
// reading its address and closing it. There is a small race between the
// close and the test's own Listen call, but it's acceptable for local
// integration tests.
func freePort(tb testing.TB) string {
	tb.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	addr := l.Addr().String()
	require.NoError(tb, l.Close())
	return addr
}

// testConfig returns a config tuned for integration tests: prod-level
// logger (silent), aggressive WAL batching (flush after every request) and
// a per-test temp directory for segment files.
func testConfig(tb testing.TB, addr string, maxConnections int) *config.Config {
	tb.Helper()

	return &config.Config{
		Network: &config.NetworkConfig{
			Address:        addr,
			MaxConnections: maxConnections,
			MaxMessageSize: 4096,
			IdleTimeout:    5 * time.Second,
			BufferSize:     4096,
			TypeConn:       "tcp",
		},
		Logger: &config.LoggingConfig{Level: initialization.LoggerLevelProd},
		Wal: &config.WalConfig{
			FlushingBatchTimeout: 10 * time.Millisecond,
			FlushingBatchCount:   1,
			FlushingBatchVolume:  1 << 20,
			MaxSegmentSize:       10 << 20,
			SegmentStoragePath:   tb.TempDir(),
			MaskName:             "wal_",
		},
		EngineType: "in_memory",
	}
}

// serverHandle owns a running server and exposes its listen address plus
// a graceful shutdown helper for tests to call at the end.
type serverHandle struct {
	addr     string
	stop     context.CancelFunc
	done     <-chan struct{}
	dataPath string
}

// startServer boots the whole stack (logger → WAL+engine → TCP listener →
// database handler loop) and waits until the port starts accepting
// connections. The returned handle is cancelled via Stop().
func startServer(tb testing.TB, cfg *config.Config) *serverHandle {
	tb.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	init, err := initialization.NewInitializer(ctx, cfg)
	require.NoError(tb, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = init.StartDatabase(ctx)
	}()

	waitForPort(tb, cfg.Network.Address, 2*time.Second)

	return &serverHandle{
		addr:     cfg.Network.Address,
		stop:     cancel,
		done:     done,
		dataPath: cfg.Wal.SegmentStoragePath,
	}
}

// Stop cancels the server's context and waits for StartDatabase to return.
func (h *serverHandle) Stop(tb testing.TB) {
	tb.Helper()

	h.stop()
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		tb.Fatal("server did not shut down within 3s")
	}
}

// waitForPort polls addr until a TCP dial succeeds or deadline fires.
func waitForPort(tb testing.TB, addr string, deadline time.Duration) {
	tb.Helper()

	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	tb.Fatalf("server did not start listening on %s", addr)
}

// newClient opens a TCP client to addr with the caller's context.
func newClient(tb testing.TB, ctx context.Context, addr string) *network.TCPClient {
	tb.Helper()

	c, err := network.NewTCPClient(ctx, slogdiscard.NewDiscardLogger(),
		network.WithClientTCPAddress(addr),
		network.WithClientTCPMaxMessageSize(4096),
		network.WithClientTCPIdleTimeout(2*time.Second),
	)
	require.NoError(tb, err)
	return c
}
