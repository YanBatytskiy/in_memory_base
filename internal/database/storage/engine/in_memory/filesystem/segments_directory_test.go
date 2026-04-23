package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

func TestMakeDirectory(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()

	t.Run("creates directory", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "wal")
		path, err := MakeDirectory(log, dir)
		require.NoError(t, err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	})

	t.Run("existing directory is ok", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		_, err := MakeDirectory(log, dir)
		require.NoError(t, err)
	})

	t.Run("file path fails", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		filePath := filepath.Join(dir, "wal")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
		_, err := MakeDirectory(log, filePath)
		require.ErrorIs(t, err, ErrFailedToCreateDirectory)
	})
}
