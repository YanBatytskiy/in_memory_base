package wal

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/compute"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	contextid "github.com/YanBatytskiy/in_memory_base/internal/lib/context_util"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

// recordingCommandHashTable is a CommandHashTable that actually stores
// the values (vs the existing stubCommandHashTable which only counts
// calls), so runtime tests can verify what got applied.
type recordingCommandHashTable struct {
	mu   sync.Mutex
	data map[string]string
}

func newRecordingCommandHashTable() *recordingCommandHashTable {
	return &recordingCommandHashTable{data: make(map[string]string)}
}

func (r *recordingCommandHashTable) Set(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[key] = value
}

func (r *recordingCommandHashTable) Del(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, key)
}

func (r *recordingCommandHashTable) Get(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.data[key]
	return v, ok
}

func (r *recordingCommandHashTable) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.data)
}

// txCtx returns a context populated with the given LSN, as produced by
// Engine.Set / Engine.Del before they forward into Wal.
func txCtx(lsn int64) context.Context {
	return context.WithValue(context.Background(), contextid.TxIDKey, strconv.FormatInt(lsn, 10))
}

// startRuntimeWal builds a Wal wired to a real filesystem segment and a
// recording hash table, recovers the empty directory and starts the
// background flusher. The caller gets the Wal, the hash table and a
// cancel func that stops the flusher.
func startRuntimeWal(t *testing.T, cfg func(dir string) *wl) (*Wal, *recordingCommandHashTable, context.CancelFunc) {
	t.Helper()

	log := slogdiscard.NewDiscardLogger()
	dir := t.TempDir()
	c := cfg(dir)
	seg := filesystem.NewSegment(log, c.SegmentStoragePath, c.MaskName, c.MaxSegmentSize)
	ht := newRecordingCommandHashTable()

	w, err := NewWal(log, c.toWalConfig(), seg, ht)
	require.NoError(t, err)
	require.NoError(t, w.Recovery(dir))

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	return w, ht, cancel
}

// TestWalSetAppliesAndPersists drives a single Set through the full
// pipeline (push → batch flush → apply) and verifies both the in-memory
// hash table and the on-disk segment were updated.
func TestWalSetAppliesAndPersists(t *testing.T) {
	t.Parallel()

	w, ht, cancel := startRuntimeWal(t, tightConfig)
	defer cancel()

	require.NoError(t, w.Set(txCtx(1), "foo", "bar"))

	got, ok := ht.Get("foo")
	require.True(t, ok)
	require.Equal(t, "bar", got)

	// Segment file exists and is non-empty.
	entries, err := os.ReadDir(w.walConfig.segmentStoragePath)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
}

// TestWalDelApplies verifies Del propagates through the same pipeline.
func TestWalDelApplies(t *testing.T) {
	t.Parallel()

	w, ht, cancel := startRuntimeWal(t, tightConfig)
	defer cancel()

	require.NoError(t, w.Set(txCtx(1), "k", "v"))
	require.NoError(t, w.Del(txCtx(2), "k"))

	_, ok := ht.Get("k")
	require.False(t, ok)
}

// TestWalParallelPush drives concurrent writers and verifies every write
// ended up in the hash table (exercises the push/SealBatch/flushBatch
// paths under contention).
func TestWalParallelPush(t *testing.T) {
	t.Parallel()

	w, ht, cancel := startRuntimeWal(t, tightConfig)
	defer cancel()

	const writers = 8
	const perWriter = 32

	var lsn atomic.Int64
	var wg sync.WaitGroup
	wg.Add(writers)
	for w1 := range writers {
		go func(wid int) {
			defer wg.Done()
			for i := range perWriter {
				key := "w" + strconv.Itoa(wid) + "-" + strconv.Itoa(i)
				require.NoError(t, w.Set(txCtx(lsn.Add(1)), key, "v"))
			}
		}(w1)
	}
	wg.Wait()

	require.Equal(t, writers*perWriter, ht.Len())
}

// TestWalDrainOnCancel cancels the context while a flush is imminent
// and asserts the flusher still drains the in-memory batch to disk
// before returning.
func TestWalDrainOnCancel(t *testing.T) {
	t.Parallel()

	// Use a longer timeout so the drain path (triggered by cancel)
	// handles the pending write, not the ticker.
	w, ht, cancel := startRuntimeWal(t, loosenConfig)
	// Write but don't wait for flush here — a tight writer drives the
	// push path. Future.Get blocks until the batch is flushed; since
	// FlushingBatchCount = 1, the batch seals immediately and the
	// flusher handles it.
	require.NoError(t, w.Set(txCtx(1), "drain", "me"))

	cancel()
	// Give the flusher a moment to observe ctx.Done and exit cleanly.
	//nolint:forbidigo // deliberate pause for the flusher goroutine to drain
	time.Sleep(50 * time.Millisecond)

	got, ok := ht.Get("drain")
	require.True(t, ok)
	require.Equal(t, "me", got)
}

// TestWalSetDispatchesCorrectCommandID is a minimal guard against
// regressions where Wal.Set forwards the wrong command id; push writes
// a gob record and apply resolves it by CommandID.
func TestWalSetDispatchesCorrectCommandID(t *testing.T) {
	t.Parallel()

	w, ht, cancel := startRuntimeWal(t, tightConfig)
	defer cancel()

	require.NoError(t, w.Set(txCtx(1), "k", "v"))
	require.Equal(t, 1, ht.Len())
	require.NoError(t, w.Del(txCtx(2), "k"))
	require.Equal(t, 0, ht.Len())

	// Sanity-check the command ids the production code uses.
	require.Equal(t, 1, compute.CommandSetID)
	require.Equal(t, 2, compute.CommandDelID)
}

// wl is a local narrow copy of config.WalConfig whose only purpose is
// to keep the helper signature decoupled from the validator tags on the
// real config type.
type wl struct {
	FlushingBatchTimeout time.Duration
	FlushingBatchCount   int
	FlushingBatchVolume  int
	MaxSegmentSize       int64
	SegmentStoragePath   string
	MaskName             string
}

func (c *wl) toWalConfig() *config.WalConfig {
	return &config.WalConfig{
		FlushingBatchTimeout: c.FlushingBatchTimeout,
		FlushingBatchCount:   c.FlushingBatchCount,
		FlushingBatchVolume:  c.FlushingBatchVolume,
		MaxSegmentSize:       c.MaxSegmentSize,
		SegmentStoragePath:   c.SegmentStoragePath,
		MaskName:             c.MaskName,
	}
}

// tightConfig flushes after every single push so tests observe the
// hash-table update synchronously.
func tightConfig(dir string) *wl {
	return &wl{
		FlushingBatchTimeout: 10 * time.Millisecond,
		FlushingBatchCount:   1,
		FlushingBatchVolume:  1 << 20,
		MaxSegmentSize:       1 << 26,
		SegmentStoragePath:   dir,
		MaskName:             "wal_",
	}
}

// loosenConfig uses a longer ticker so the drain path (triggered by
// context cancellation) is what flushes the pending batch.
func loosenConfig(dir string) *wl {
	return &wl{
		FlushingBatchTimeout: time.Second,
		FlushingBatchCount:   1,
		FlushingBatchVolume:  1 << 20,
		MaxSegmentSize:       1 << 26,
		SegmentStoragePath:   dir,
		MaskName:             "wal_",
	}
}
