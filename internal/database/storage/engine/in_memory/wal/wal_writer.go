package wal

import (
	"bytes"
	"encoding/gob"
	"log/slog"
	"os"
)

// segmentWrite is the write-side contract the WAL needs from the file-system
// segment. It is unexported so tests can substitute it via mockery.
type segmentWrite interface {
	Write(data []byte) error
	SetFile(file *os.File)
}

// WalWriter serialises a batch of [WriteRequest] values into a gob stream
// and persists it via the injected segment writer. It is used by [Wal].
type WalWriter struct {
	segment segmentWrite
	log     *slog.Logger
}

// NewWalWriter returns a [WalWriter] that persists batches through seg.
func NewWalWriter(log *slog.Logger, seg segmentWrite) *WalWriter {
	return &WalWriter{
		log:     log,
		segment: seg,
	}
}

// Write encodes requests into a single gob stream, writes it to the
// underlying segment, and resolves every per-request Promise with the
// outcome — nil on success, the I/O error otherwise.
func (walWriter *WalWriter) Write(requests []WriteRequest) error {
	buffer := &bytes.Buffer{}
	encoder := gob.NewEncoder(buffer)

	for _, request := range requests {
		err := request.log.EncodeTo(encoder)
		if err != nil {
			walWriter.notification(err, requests)
			return err
		}
	}

	err := walWriter.segment.Write(buffer.Bytes())

	walWriter.notification(err, requests)

	if err != nil {
		return err
	}

	return nil
}

func (walWriter *WalWriter) notification(response error, requests []WriteRequest) {
	for idx := range requests {
		requests[idx].promise.Set(response)
	}
}
