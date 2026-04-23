package network

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

type serverFn func(conn net.Conn)

type serverHandle struct {
	addr string
	stop func()
}

func startServer(t *testing.T, fn serverFn) *serverHandle {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil && strings.Contains(err.Error(), "operation not permitted") {
		t.Skip("net.Listen not permitted in this environment")
	}
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fn(conn)
	}()

	return &serverHandle{
		addr: ln.Addr().String(),
		stop: func() {
			_ = ln.Close()
			<-done
		},
	}
}

func TestTCPClient_SendAndReceive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) *serverHandle
		address string
		options []TCPClientOption
		maxSize int
		hasSize bool
		request []byte
		want    []byte
		wantErr bool
	}{
		{
			name: "echo ok",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					buf := make([]byte, 32)
					n, _ := conn.Read(buf)
					_, _ = conn.Write(append([]byte("ok:"), buf[:n]...))
				})
			},
			request: []byte("ping"),
			want:    []byte("ok:ping"),
		},
		{
			name: "trims whitespace in response",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 1))
					_, _ = conn.Write([]byte("pong \n"))
				})
			},
			request: []byte("ping"),
			want:    []byte("pong"),
		},
		{
			name: "empty response",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 1))
					_, _ = conn.Write([]byte("\n"))
				})
			},
			request: []byte("ping"),
			want:    []byte(""),
		},
		{
			name: "server closes immediately",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_ = conn.Close()
				})
			},
			request: []byte("ping"),
			wantErr: true,
		},
		{
			name: "server closes after read before write",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 4))
					_ = conn.Close()
				})
			},
			request: []byte("ping"),
			wantErr: true,
		},
		{
			name:    "invalid address format",
			address: "bad:addr",
			request: []byte("ping"),
			wantErr: true,
		},
		{
			name:    "port refused",
			address: "127.0.0.1:1",
			request: []byte("ping"),
			wantErr: true,
		},
		{
			name: "small buffer truncates response",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 4))
					_, _ = conn.Write([]byte("longresponse"))
				})
			},
			options: []TCPClientOption{WithClientTCPMaxMessageSize(4)},
			maxSize: 4,
			hasSize: true,
			request: []byte("ping"),
			want:    []byte("long"),
		},
		{
			name: "multi line response preserved",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 2))
					_, _ = conn.Write([]byte("line1\nline2\n"))
				})
			},
			request: []byte("ok"),
			want:    []byte("line1\nline2"),
		},
		{
			name: "zero buffer size returns empty",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 1))
					_, _ = conn.Write([]byte("data"))
				})
			},
			options: []TCPClientOption{WithClientTCPMaxMessageSize(0)},
			maxSize: 0,
			hasSize: true,
			request: []byte("x"),
			want:    []byte(""),
		},
		{
			name: "read timeout",
			setup: func(t *testing.T) *serverHandle {
				return startServer(t, func(conn net.Conn) {
					_, _ = conn.Read(make([]byte, 4))
					//nolint:forbidigo // deliberate pause longer than client idle timeout
					time.Sleep(50 * time.Millisecond)
				})
			},
			options: []TCPClientOption{WithClientTCPIdleTimeout(10 * time.Millisecond)},
			request: []byte("ping"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var srv *serverHandle
			if tc.setup != nil {
				srv = tc.setup(t)
				defer srv.stop()
			}

			addr := tc.address
			if srv != nil {
				addr = srv.addr
			}

			opts := append([]TCPClientOption{}, tc.options...)
			size := tc.maxSize
			if !tc.hasSize {
				size = 1024
			}
			opts = append(opts, WithClientTCPMaxMessageSize(size))
			if addr != "" {
				opts = append(opts, WithClientTCPAddress(addr))
			}

			client, err := NewTCPClient(t.Context(), slogdiscard.NewDiscardLogger(), opts...)
			if tc.wantErr && client == nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer client.Close()

			resp, err := client.SendAndReceive(tc.request)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if len(tc.want) == 0 {
				require.Len(t, resp, 0)
			} else {
				require.Equal(t, tc.want, resp)
			}
		})
	}
}

func TestNewTCPClient_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		logNil  bool
		options []TCPClientOption
		wantErr bool
	}{
		{
			name:    "nil logger",
			logNil:  true,
			options: []TCPClientOption{WithClientTCPAddress("127.0.0.1:0"), WithClientTCPMaxMessageSize(8)},
			wantErr: true,
		},
		{
			name:    "invalid address",
			options: []TCPClientOption{WithClientTCPAddress("bad:addr")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := slogdiscard.NewDiscardLogger()
			if tc.logNil {
				log = nil
			}

			client, err := NewTCPClient(t.Context(), log, tc.options...)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			client.Close()
		})
	}
}
