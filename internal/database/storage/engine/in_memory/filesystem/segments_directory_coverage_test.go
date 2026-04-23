package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

// TestMakeDirectoryAbsolutePath covers the absolute-path branch of
// resolveWalPath.
func TestMakeDirectoryAbsolutePath(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()
	root := t.TempDir()
	target := filepath.Join(root, "wal-abs")

	resolved, err := MakeDirectory(log, target)
	require.NoError(t, err)
	require.Equal(t, target, resolved)

	info, err := os.Stat(target)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// TestMakeDirectoryEmptyPath covers the default-path branch
// (fallback to "storage/wal" relative to cwd).
func TestMakeDirectoryEmptyPath(t *testing.T) {
	log := slogdiscard.NewDiscardLogger()

	// Run in an isolated cwd so we don't litter the repo.
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	sandbox := t.TempDir()
	require.NoError(t, os.Chdir(sandbox))

	resolved, err := MakeDirectory(log, "")
	require.NoError(t, err)

	// os.Getwd evaluates symlinks on macOS (/var → /private/var), so
	// resolve the expected path the same way before comparing.
	sandboxReal, err := filepath.EvalSymlinks(sandbox)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(sandboxReal, "storage", "wal"), resolved)

	info, err := os.Stat(resolved)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// TestMakeDirectoryFailsWhenPathIsAFile ensures the os.MkdirAll branch
// returns ErrFailedToCreateDirectory when the target path points at an
// existing regular file instead of a directory.
func TestMakeDirectoryFailsWhenPathIsAFile(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-dir")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

	_, err := MakeDirectory(log, filePath)
	require.ErrorIs(t, err, ErrFailedToCreateDirectory)
}
