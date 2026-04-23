package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

// TestSegmentRead covers Segment.Read (thin wrapper over ReadFile).
func TestSegmentRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.log")
	require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o600))

	seg := NewSegment(slogdiscard.NewDiscardLogger(), dir, "segment_", 1024)
	data, err := seg.Read(path)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), data)
}

// TestSegmentGetList covers Segment.GetList (thin wrapper over GetFileList).
func TestSegmentGetList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.log"), []byte("1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.log"), []byte("2"), 0o600))

	seg := NewSegment(slogdiscard.NewDiscardLogger(), dir, "segment_", 1024)
	files, err := seg.GetList()
	require.NoError(t, err)
	require.Len(t, files, 2)
}

// TestSegmentSetFile covers the three SetFile branches: nil, a readable
// file (Stat succeeds and segmentSize is populated), and a closed file
// (Stat fails — SetFile must not panic and segmentSize stays at 0).
func TestSegmentSetFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	seg := NewSegment(slogdiscard.NewDiscardLogger(), dir, "segment_", 1024)

	// nil — no-op, segmentSize stays zero.
	seg.SetFile(nil)
	require.Zero(t, seg.segmentSize)

	// Readable file: segmentSize reflects the on-disk length.
	path := filepath.Join(dir, "existing.log")
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o600))
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	seg.SetFile(file)
	require.Equal(t, int64(len("payload")), seg.segmentSize)

	// Closed file: Stat returns an error, SetFile logs and returns
	// without updating segmentSize.
	closed, err := os.Open(path)
	require.NoError(t, err)
	require.NoError(t, closed.Close())

	seg.SetFile(closed)
	require.Equal(t, int64(0), seg.segmentSize) // reset by SetFile before Stat
}

// TestReadFileUtility covers the package-level ReadFile helper (Segment.Read
// routes through it, but calling it directly ensures error-free path
// and error-path coverage).
func TestReadFileUtility(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.log")
	require.NoError(t, os.WriteFile(path, []byte("abc"), 0o600))

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		data, err := ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, []byte("abc"), data)
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		_, err := ReadFile(filepath.Join(dir, "does-not-exist"))
		require.Error(t, err)
	})
}

// TestSegmentWriteLazyCreate ensures Write lazily creates the first
// segment when it has not been explicitly created yet.
func TestSegmentWriteLazyCreate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	seg := NewSegment(slogdiscard.NewDiscardLogger(), dir, "segment_", 1024)

	// No explicit CreateFile call; Write must create the first segment.
	require.NoError(t, seg.Write([]byte("first")))

	files, err := GetFileList(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	content, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Equal(t, []byte("first"), content)
}
