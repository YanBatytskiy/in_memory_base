package filesystem

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

func TestSegmentWriteRotatesFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	segment := NewSegment(slogdiscard.NewDiscardLogger(), dir, "segment_", 8)

	require.NoError(t, segment.Write([]byte("abcdef")))
	require.NoError(t, segment.Write([]byte("ghijkl")))

	files, err := GetFileList(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	first, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Equal(t, []byte("abcdef"), first)

	second, err := os.ReadFile(files[1])
	require.NoError(t, err)
	require.Equal(t, []byte("ghijkl"), second)
}
