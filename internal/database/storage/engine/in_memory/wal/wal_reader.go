package wal

import (
	"log/slog"
	"os"
)

// segmentRead is the read-side contract the WAL needs from the file-system
// segment. It is unexported so tests can substitute it via mockery.
type segmentRead interface {
	Read(filePath string) ([]byte, error)
	GetList() ([]string, error)
	CreateFile() (*os.File, error)
}

// WalReader is a thin adapter around a segment reader; it exists so the
// WAL can keep read and write responsibilities on separate types.
type WalReader struct {
	log     *slog.Logger
	segment segmentRead
}

// NewWalReader returns a [WalReader] that reads segments through seg.
func NewWalReader(log *slog.Logger, seg segmentRead) *WalReader {
	return &WalReader{
		log:     log,
		segment: seg,
	}
}
