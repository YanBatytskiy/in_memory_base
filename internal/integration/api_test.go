//go:build integration

package integration_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAPI_SetGetDel drives the happy path of the text protocol end-to-end
// through a real TCP server: SET → OK, GET → VALUE, DEL → DELETED, GET
// → NOT_FOUND.
func TestAPI_SetGetDel(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, freePort(t), 4)
	srv := startServer(t, cfg)
	t.Cleanup(func() { srv.Stop(t) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := newClient(t, ctx, srv.addr)
	t.Cleanup(client.Close)

	cases := []struct {
		request string
		want    string
	}{
		{"SET user/1 alice", "OK"},
		{"SET user/2 bob", "OK"},
		{"GET user/1", "VALUE alice"},
		{"GET user/2", "VALUE bob"},
		{"DEL user/1", "DELETED"},
		{"GET user/1", "NOT_FOUND"},
	}

	for _, tc := range cases {
		resp, err := client.SendAndReceive([]byte(tc.request))
		require.NoError(t, err, tc.request)
		require.Equal(t, tc.want, string(resp), tc.request)
	}
}

// TestAPI_InvalidCommand checks that a malformed request yields an error
// response but does not drop the TCP connection: the next valid request
// on the same connection still succeeds.
func TestAPI_InvalidCommand(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, freePort(t), 2)
	srv := startServer(t, cfg)
	t.Cleanup(func() { srv.Stop(t) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := newClient(t, ctx, srv.addr)
	t.Cleanup(client.Close)

	// Unknown command → error string, connection stays alive.
	resp, err := client.SendAndReceive([]byte("FOO bar"))
	require.NoError(t, err)
	require.NotEqual(t, "OK", string(resp), "FOO should not be accepted")

	// A valid request on the same connection still works.
	resp, err = client.SendAndReceive([]byte("SET hello world"))
	require.NoError(t, err)
	require.Equal(t, "OK", string(resp))
}

// TestAPI_Recovery writes a key, stops the server, starts a fresh server
// pointed at the same WAL directory, and verifies the key survived via
// WAL replay.
//
// The scenario uses a single SET because the current WAL recovery path
// wraps the whole segment file in one gob.Decoder (see [wal.Wal.Recovery]),
// while every batch flush writes a fresh gob stream (new gob.NewEncoder
// per flush in wal_writer.go). Multiple flushes into the same segment
// therefore cannot be replayed by a single decoder — that's a known
// limitation of the existing WAL format, not something this integration
// test should paper over.
func TestAPI_Recovery(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, freePort(t), 2)
	srv := startServer(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := newClient(t, ctx, srv.addr)

	resp, err := client.SendAndReceive([]byte("SET persisted yes"))
	require.NoError(t, err)
	require.Equal(t, "OK", string(resp))
	client.Close()

	// Stop the first server; its WAL flusher drains the pending batch.
	srv.Stop(t)

	// Start a fresh server on a new address, re-using the WAL directory.
	cfg2 := testConfig(t, freePort(t), 2)
	cfg2.Wal.SegmentStoragePath = srv.dataPath
	srv2 := startServer(t, cfg2)
	t.Cleanup(func() { srv2.Stop(t) })

	client2 := newClient(t, ctx, srv2.addr)
	t.Cleanup(client2.Close)

	resp, err = client2.SendAndReceive([]byte("GET persisted"))
	require.NoError(t, err)
	require.Equal(t, "VALUE yes", string(resp), "key lost after restart")
}

// TestAPI_MaxConnections verifies that the semaphore in TCPServer bounds
// concurrent connection handlers: once the limit is hit, a follow-up
// client's request stays in flight until an earlier one releases the slot.
func TestAPI_MaxConnections(t *testing.T) {
	t.Parallel()

	const limit = 1
	cfg := testConfig(t, freePort(t), limit)
	srv := startServer(t, cfg)
	t.Cleanup(func() { srv.Stop(t) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Client A opens and holds the single allowed slot via a long idle
	// gap: the connection stays up, so the server handler stays occupied.
	clientA := newClient(t, ctx, srv.addr)
	t.Cleanup(clientA.Close)
	respA, err := clientA.SendAndReceive([]byte("SET a 1"))
	require.NoError(t, err)
	require.Equal(t, "OK", string(respA))

	// Client B dials successfully (TCP accept is not gated), but its
	// request will not be served until clientA disconnects.
	clientB := newClient(t, ctx, srv.addr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = clientB.SendAndReceive([]byte("SET b 2"))
	}()

	// B should not finish while A holds the slot.
	select {
	case <-done:
		t.Fatal("second client served while max_connections=1 was saturated")
	case <-time.After(200 * time.Millisecond):
	}

	// Release the slot by closing A; B should complete shortly after.
	clientA.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second client did not complete after A released the slot")
	}
	clientB.Close()
}

// TestAPI_GracefulShutdown verifies that cancelling the server context
// causes StartDatabase to return and closes the listener so follow-up
// dials fail.
func TestAPI_GracefulShutdown(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, freePort(t), 2)
	srv := startServer(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := newClient(t, ctx, srv.addr)
	resp, err := client.SendAndReceive([]byte("SET warm up"))
	require.NoError(t, err)
	require.Equal(t, "OK", string(resp))
	client.Close()

	// Trigger graceful shutdown: StartDatabase should return within a
	// short window and the listener should be closed.
	srv.Stop(t)

	conn, dialErr := net.DialTimeout("tcp", srv.addr, 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("server still accepts connections after context cancellation")
	}
}
