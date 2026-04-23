package network

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/concurrency"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

type acceptResult struct {
	conn net.Conn
	err  error
}

type stubListener struct {
	mu      sync.Mutex
	results []acceptResult
	closed  bool
}

func (s *stubListener) Accept() (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, net.ErrClosed
	}
	if len(s.results) == 0 {
		return nil, net.ErrClosed
	}

	res := s.results[0]
	s.results = s.results[1:]
	return res.conn, res.err
}

func (s *stubListener) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *stubListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

type deadlineErrConn struct {
	net.Conn
	readErr  error
	writeErr error
}

func (c *deadlineErrConn) SetReadDeadline(t time.Time) error {
	if c.readErr != nil {
		return c.readErr
	}
	return c.Conn.SetReadDeadline(t)
}

func (c *deadlineErrConn) SetWriteDeadline(t time.Time) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	return c.Conn.SetWriteDeadline(t)
}

func isOpNotPermitted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "operation not permitted")
}

func TestNewTCPServer_Table(t *testing.T) {
	t.Parallel()

	type wantFields struct {
		maxConnections int
		bufferSize     int
		idleTimeout    time.Duration
	}

	tests := []struct {
		name      string
		addr      string
		logNil    bool
		options   []TCPServeOption
		wantErr   bool
		errTarget error
		want      *wantFields
	}{
		{
			name:      "nil logger",
			addr:      "127.0.0.1:0",
			logNil:    true,
			options:   []TCPServeOption{WithServerTCPBufferSize(1), WithServerTCPMaxConnectionNumber(1)},
			wantErr:   true,
			errTarget: ErrInvalidLogger,
		},
		{
			name:    "invalid address format",
			addr:    "127.0.0.1",
			options: []TCPServeOption{WithServerTCPBufferSize(1), WithServerTCPMaxConnectionNumber(1)},
			wantErr: true,
		},
		{
			name: "address already in use",
			addr: func() string {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil && isOpNotPermitted(err) {
					t.Skip("net.Listen not permitted in this environment")
				}
				require.NoError(t, err)
				t.Cleanup(func() { _ = ln.Close() })
				return ln.Addr().String()
			}(),
			options: []TCPServeOption{WithServerTCPBufferSize(1), WithServerTCPMaxConnectionNumber(1)},
			wantErr: true,
		},
		{
			name:      "zero buffer size",
			addr:      "127.0.0.1:0",
			options:   []TCPServeOption{WithServerTCPBufferSize(0), WithServerTCPMaxConnectionNumber(1)},
			wantErr:   true,
			errTarget: ErrInvalidBufferSize,
		},
		{
			name:      "zero max connections",
			addr:      "127.0.0.1:0",
			options:   []TCPServeOption{WithServerTCPBufferSize(1), WithServerTCPMaxConnectionNumber(0)},
			wantErr:   true,
			errTarget: ErrInvalidMaxConn,
		},
		{
			name:    "success minimal",
			addr:    "127.0.0.1:0",
			options: []TCPServeOption{WithServerTCPBufferSize(8), WithServerTCPMaxConnectionNumber(1)},
			want: &wantFields{
				maxConnections: 1,
				bufferSize:     8,
			},
		},
		{
			name:    "success with idle timeout",
			addr:    "127.0.0.1:0",
			options: []TCPServeOption{WithServerTCPBufferSize(16), WithServerTCPMaxConnectionNumber(2), WithServerTCPIdleTimeout(time.Second)},
			want: &wantFields{
				maxConnections: 2,
				bufferSize:     16,
				idleTimeout:    time.Second,
			},
		},
		{
			name:    "negative values fallback",
			addr:    "127.0.0.1:0",
			options: []TCPServeOption{WithServerTCPBufferSize(-1), WithServerTCPMaxConnectionNumber(-1), WithServerTCPIdleTimeout(-time.Second)},
			want: &wantFields{
				maxConnections: 100,
				bufferSize:     4098,
				idleTimeout:    5 * time.Minute,
			},
		},
		{
			name:      "missing buffer size option",
			addr:      "127.0.0.1:0",
			options:   []TCPServeOption{WithServerTCPMaxConnectionNumber(5)},
			wantErr:   true,
			errTarget: ErrInvalidBufferSize,
		},
		{
			name:      "missing max connections option",
			addr:      "127.0.0.1:0",
			options:   []TCPServeOption{WithServerTCPBufferSize(128)},
			wantErr:   true,
			errTarget: ErrInvalidMaxConn,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := slogdiscard.NewDiscardLogger()
			if tc.logNil {
				log = nil
			}

			srv, err := NewTCPServer(t.Context(), tc.addr, log, tc.options...)
			if tc.wantErr {
				if isOpNotPermitted(err) {
					t.Skip("net.Listen not permitted in this environment")
				}
				require.Error(t, err)
				if tc.errTarget != nil {
					require.ErrorIs(t, err, tc.errTarget)
				}
				return
			}

			if isOpNotPermitted(err) {
				t.Skip("net.Listen not permitted in this environment")
			}
			require.NoError(t, err)
			require.NotNil(t, srv)
			if tc.want != nil {
				require.Equal(t, tc.want.maxConnections, srv.maxConnections)
				require.Equal(t, tc.want.bufferSize, srv.bufferSize)
				require.Equal(t, tc.want.idleTimeout, srv.idleTimeout)
			}
			_ = srv.listener.Close()
		})
	}
}

func TestHandleConnections_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buffer      int
		idleTimeout time.Duration
		wrapConn    func(net.Conn) net.Conn
		handler     TCPHandler
		write       []byte
		closeAfter  bool
		wantResp    []byte
		wantReadErr bool
		waitTimeout time.Duration
	}{
		{
			name:   "handler returns response",
			buffer: 32,
			handler: func(_ context.Context, data []byte) []byte {
				if string(data) == "PING" {
					return []byte("OK")
				}
				return []byte("BAD")
			},
			write:      []byte("PING"),
			wantResp:   []byte("OK"),
			closeAfter: true,
		},
		{
			name:        "read timeout",
			buffer:      8,
			idleTimeout: 10 * time.Millisecond,
		},
		{
			name:   "handler panic recovered",
			buffer: 16,
			handler: func(_ context.Context, _ []byte) []byte {
				panic("boom")
			},
			write:       []byte("PANIC"),
			wantReadErr: true,
		},
		{
			name:        "read deadline error",
			buffer:      16,
			idleTimeout: 10 * time.Millisecond,
			wrapConn: func(conn net.Conn) net.Conn {
				return &deadlineErrConn{Conn: conn, readErr: errors.New("read deadline error")}
			},
			waitTimeout: 200 * time.Millisecond,
		},
		{
			name:        "write deadline error",
			buffer:      16,
			idleTimeout: 10 * time.Millisecond,
			wrapConn: func(conn net.Conn) net.Conn {
				return &deadlineErrConn{Conn: conn, writeErr: errors.New("write deadline error")}
			},
			handler: func(_ context.Context, _ []byte) []byte {
				return []byte("RESP")
			},
			write:       []byte("PING"),
			wantReadErr: true,
		},
		{
			name:   "write error on closed client",
			buffer: 16,
			handler: func(_ context.Context, _ []byte) []byte {
				return []byte("RESP")
			},
			write:      []byte("CLOSE"),
			closeAfter: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogdiscard.NewDiscardLogger()
			handler := tc.handler
			if handler == nil {
				handler = func(_ context.Context, data []byte) []byte { return data }
			}

			server := &TCPServer{
				bufferSize:  tc.buffer,
				idleTimeout: tc.idleTimeout,
				log:         logger,
			}

			ctx := context.Background()

			srvConn, cliConn := net.Pipe()
			t.Cleanup(func() { _ = cliConn.Close() })

			conn := srvConn
			if tc.wrapConn != nil {
				conn = tc.wrapConn(srvConn)
			}

			done := make(chan struct{})
			go func() {
				server.HandleConnections(ctx, conn, handler)
				close(done)
			}()

			if len(tc.write) > 0 {
				_ = cliConn.SetWriteDeadline(time.Now().Add(time.Second))
				_, _ = cliConn.Write(tc.write)
			}

			if tc.closeAfter {
				_ = cliConn.Close()
			}

			if tc.wantResp != nil && !tc.closeAfter {
				_ = cliConn.SetReadDeadline(time.Now().Add(time.Second))
				buf := make([]byte, len(tc.wantResp))
				n, err := cliConn.Read(buf)
				require.NoError(t, err)
				require.Equal(t, tc.wantResp, buf[:n])
			} else if tc.wantReadErr {
				_ = cliConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				buf := make([]byte, tc.buffer)
				_, err := cliConn.Read(buf)
				require.Error(t, err)
			}

			waitTimeout := tc.waitTimeout
			if waitTimeout == 0 {
				waitTimeout = time.Second
			}

			select {
			case <-done:
			case <-time.After(waitTimeout):
				t.Fatal("HandleConnections did not return")
			}
		})
	}
}

func TestHandleClientQueries_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxConn     int
		bufferSize  int
		makeConns   func(t *testing.T) ([]acceptResult, []net.Conn)
		handler     TCPHandler
		clientCheck func(t *testing.T, clients []net.Conn)
		waitTimeout time.Duration
	}{
		{
			name:       "accept error then handles connection",
			maxConn:    1,
			bufferSize: 8,
			makeConns: func(t *testing.T) ([]acceptResult, []net.Conn) {
				srvConn, cliConn := net.Pipe()
				results := []acceptResult{
					{err: errors.New("accept failed")},
					{conn: srvConn},
				}
				return results, []net.Conn{cliConn}
			},
			handler: func(_ context.Context, data []byte) []byte {
				if string(data) == "PING" {
					return []byte("PONG")
				}
				return []byte("BAD")
			},
			clientCheck: func(t *testing.T, clients []net.Conn) {
				require.Len(t, clients, 1)
				_ = clients[0].SetWriteDeadline(time.Now().Add(time.Second))
				_, _ = clients[0].Write([]byte("PING"))

				_ = clients[0].SetReadDeadline(time.Now().Add(time.Second))
				buf := make([]byte, 4)
				n, err := clients[0].Read(buf)
				require.NoError(t, err)
				require.Equal(t, "PONG", string(buf[:n]))
			},
		},
		{
			name:       "accept closed exits on cancel",
			maxConn:    1,
			bufferSize: 8,
			makeConns: func(t *testing.T) ([]acceptResult, []net.Conn) {
				return nil, nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results, clients := tc.makeConns(t)
			t.Cleanup(func() {
				for _, c := range clients {
					_ = c.Close()
				}
			})

			listener := &stubListener{results: results}
			server := &TCPServer{
				listener:       listener,
				semaphore:      concurrency.NewSemaphore(tc.maxConn),
				maxConnections: tc.maxConn,
				bufferSize:     tc.bufferSize,
				log:            slogdiscard.NewDiscardLogger(),
			}

			handler := tc.handler
			if handler == nil {
				handler = func(_ context.Context, data []byte) []byte { return data }
			}

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				server.HandleClientQueries(ctx, handler)
				close(done)
			}()

			if tc.clientCheck != nil {
				tc.clientCheck(t, clients)
			}

			cancel()

			waitTimeout := tc.waitTimeout
			if waitTimeout == 0 {
				waitTimeout = time.Second
			}

			select {
			case <-done:
			case <-time.After(waitTimeout):
				t.Fatal("HandleClientQueries did not return after cancel")
			}
		})
	}
}
