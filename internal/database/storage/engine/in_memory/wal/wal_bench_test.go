package wal

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	contextid "github.com/YanBatytskiy/in_memory_base/internal/lib/context_util"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

// benchCommandHashTable is a no-op CommandHashTable that does not
// contend with the benchmark for locks; we want to measure the WAL
// pipeline, not the hash table.
type benchCommandHashTable struct{}

func (benchCommandHashTable) Set(_, _ string) {}
func (benchCommandHashTable) Del(_ string)    {}

// benchWalConfig picks batch thresholds so the flusher has room to
// coalesce writes (matches the example.yaml defaults scaled down for a
// tight benchmark loop).
func benchWalConfig(dir string) *config.WalConfig {
	return &config.WalConfig{
		FlushingBatchTimeout: 10 * time.Millisecond,
		FlushingBatchCount:   100,
		FlushingBatchVolume:  1 << 20,
		MaxSegmentSize:       1 << 27, // 128 MiB, far from hitting during a bench
		SegmentStoragePath:   dir,
		MaskName:             "wal_",
	}
}

// newBenchWal wires a Wal against a freshly allocated segment directory
// and starts its background flusher. The returned cancel stops the
// flusher and lets the benchmark return cleanly.
func newBenchWal(b *testing.B) (*Wal, context.CancelFunc) {
	b.Helper()

	log := slogdiscard.NewDiscardLogger()
	cfg := benchWalConfig(b.TempDir())
	seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)

	w, err := NewWal(log, cfg, seg, benchCommandHashTable{})
	require.NoError(b, err)
	require.NoError(b, w.Recovery(cfg.SegmentStoragePath))

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	return w, cancel
}

// pushCtx returns a context pre-populated with a unique, monotonically
// growing LSN so every call to Wal.Set / Wal.Del ends up with a
// distinct sequence number.
func pushCtx(counter *atomic.Int64) context.Context {
	lsn := counter.Add(1)
	return context.WithValue(context.Background(), contextid.TxIDKey, strconv.FormatInt(lsn, 10))
}

// BenchmarkWal_Push measures the end-to-end throughput of a serial
// producer calling Wal.Set: enqueue → batch flush (gob encode + fsync
// on a t.TempDir-backed segment) → apply → caller unblocks.
func BenchmarkWal_Push(b *testing.B) {
	w, cancel := newBenchWal(b)
	defer cancel()

	var counter atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		err := w.Set(pushCtx(&counter), "k"+strconv.Itoa(i), "v")
		if err != nil {
			b.Fatalf("wal.Set: %v", err)
		}
	}
}

// BenchmarkWal_PushParallel measures the same pipeline under
// concurrent producers: the flusher should amortise fsync across the
// larger batches that form when several goroutines push simultaneously.
func BenchmarkWal_PushParallel(b *testing.B) {
	w, cancel := newBenchWal(b)
	defer cancel()

	var counter atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			err := w.Set(pushCtx(&counter), "k"+strconv.Itoa(i), "v")
			if err != nil {
				b.Fatalf("wal.Set: %v", err)
			}
			i++
		}
	})
}
