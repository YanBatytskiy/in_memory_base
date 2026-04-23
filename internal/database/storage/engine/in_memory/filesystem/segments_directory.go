package filesystem

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
)

// Sentinel errors returned by [MakeDirectory].
var (
	// ErrFailedResolveSegmentPath wraps any failure while turning the
	// user-supplied path into an absolute one.
	ErrFailedResolveSegmentPath = errors.New("failed resolve segment path")
	// ErrInvalidWorkingFolder is returned when [os.Getwd] fails during
	// path resolution.
	ErrInvalidWorkingFolder = errors.New("invalid working folder")
	// ErrFailedToCreateDirectory wraps failures from [os.MkdirAll].
	ErrFailedToCreateDirectory = errors.New("failed to create directory")
)

// MakeDirectory resolves directory to an absolute path (defaulting to
// "./storage/wal" when empty), creates it with 0755 permissions, and
// returns the resolved path. Safe to call when the directory already
// exists.
func MakeDirectory(log *slog.Logger, directory string) (string, error) {
	path, err := resolveWalPath(log, directory)
	if err != nil {
		log.Debug("failed resolve segment path", slog.String("path", directory))
		return "", ErrFailedResolveSegmentPath
	}

	err = os.MkdirAll(path, 0o750)
	if err != nil {
		log.Debug("failed create storage directory", slog.String("path", directory), slog.Any("err", err))
		return "", ErrFailedToCreateDirectory
	}

	return path, nil
}

func resolveWalPath(log *slog.Logger, directory string) (string, error) {
	if directory == "" {
		directory = filepath.Join("storage", "wal")
	}
	if filepath.IsAbs(directory) {
		log.Debug("using absolute path for storage directory", slog.String("path", directory))
		return directory, nil
	}

	workDir, err := os.Getwd()
	if err != nil {
		log.Debug("failed get working directory", slog.String("error", err.Error()))
		return "", ErrInvalidWorkingFolder
	}
	directory = filepath.Join(workDir, directory)

	return directory, nil
}
