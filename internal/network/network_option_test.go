package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithClientOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apply       func(*TCPClient)
		wantAddress string
		wantIdle    time.Duration
		wantMaxSize int
	}{
		{
			name:        "address",
			apply:       WithClientTCPAddress("addr"),
			wantAddress: "addr",
		},
		{
			name:     "idle timeout positive",
			apply:    WithClientTCPIdleTimeout(2 * time.Second),
			wantIdle: 2 * time.Second,
		},
		{
			name:     "idle timeout negative falls back",
			apply:    WithClientTCPIdleTimeout(-time.Second),
			wantIdle: 5 * time.Minute,
		},
		{
			name:        "max message size positive",
			apply:       WithClientTCPMaxMessageSize(16),
			wantMaxSize: 16,
		},
		{
			name:        "max message size negative falls back",
			apply:       WithClientTCPMaxMessageSize(-1),
			wantMaxSize: 4098,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := &TCPClient{}
			tc.apply(client)
			if tc.wantAddress != "" {
				require.Equal(t, tc.wantAddress, client.address)
			}
			if tc.wantIdle != 0 {
				require.Equal(t, tc.wantIdle, client.idleTimeout)
			}
			if tc.wantMaxSize != 0 {
				require.Equal(t, tc.wantMaxSize, client.maxMessageSize)
			}
		})
	}
}

func TestWithServerOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apply       func(*TCPServer)
		wantIdle    time.Duration
		wantMaxConn int
		wantBuf     int
	}{
		{
			name:     "idle timeout positive",
			apply:    WithServerTCPIdleTimeout(3 * time.Second),
			wantIdle: 3 * time.Second,
		},
		{
			name:     "idle timeout negative falls back",
			apply:    WithServerTCPIdleTimeout(-time.Second),
			wantIdle: 5 * time.Minute,
		},
		{
			name:        "max connections positive",
			apply:       WithServerTCPMaxConnectionNumber(10),
			wantMaxConn: 10,
		},
		{
			name:        "max connections negative falls back",
			apply:       WithServerTCPMaxConnectionNumber(-1),
			wantMaxConn: 100,
		},
		{
			name:    "buffer size positive",
			apply:   WithServerTCPBufferSize(64),
			wantBuf: 64,
		},
		{
			name:    "buffer size negative falls back",
			apply:   WithServerTCPBufferSize(-1),
			wantBuf: 4098,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := &TCPServer{}
			tc.apply(server)
			if tc.wantIdle != 0 {
				require.Equal(t, tc.wantIdle, server.idleTimeout)
			}
			if tc.wantMaxConn != 0 {
				require.Equal(t, tc.wantMaxConn, server.maxConnections)
			}
			if tc.wantBuf != 0 {
				require.Equal(t, tc.wantBuf, server.bufferSize)
			}
		})
	}
}
