//go:build integration

package integration_test

import (
	"context"
	"strconv"
	"testing"
)

// BenchmarkE2E_SetGet drives alternating SET and GET requests through
// the real stack — TCP client → server → database → engine → WAL with
// on-disk flush → response — and measures round-trip throughput per
// operation.
func BenchmarkE2E_SetGet(b *testing.B) {
	cfg := testConfig(b, freePort(b), 4)
	srv := startServer(b, cfg)
	defer srv.Stop(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newClient(b, ctx, srv.addr)
	defer client.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		key := "k" + strconv.Itoa(i)
		respSet, err := client.SendAndReceive([]byte("SET " + key + " v"))
		if err != nil {
			b.Fatalf("SET: %v", err)
		}
		if string(respSet) != "OK" {
			b.Fatalf("SET resp = %q, want OK", respSet)
		}

		respGet, err := client.SendAndReceive([]byte("GET " + key))
		if err != nil {
			b.Fatalf("GET: %v", err)
		}
		if string(respGet) != "VALUE v" {
			b.Fatalf("GET resp = %q, want VALUE v", respGet)
		}
	}
}
