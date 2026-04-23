// Package wal implements the write-ahead log used by the in-memory engine.
//
// Writes are appended to an in-memory batch, which is flushed to disk when
// any of three thresholds fires — elapsed time, operation count, or byte
// volume. A flush persists the batch to a segment file, applies the
// operations to the hash table, and only then unblocks the caller waiting
// on the per-write [concurrency.Future]. On startup, [Wal.Recovery] replays
// all existing segment files in LSN order so the hash table is rebuilt to
// its last durable state.
package wal

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"io"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/concurrency"
	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/compute"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	contextid "github.com/YanBatytskiy/in_memory_base/internal/lib/context_util"
)

// Sentinel errors returned by the WAL.
var (
	// ErrFailedToReadDirectory wraps any I/O failure while listing the WAL
	// segment directory during recovery.
	ErrFailedToReadDirectory = errors.New("failed to read directory")
	// ErrFailedOpenLastSegment is returned when recovery cannot reopen the
	// last segment for append.
	ErrFailedOpenLastSegment = errors.New("failed to open last segment")
	// ErrInvalidLogger is returned by [NewWal] when the logger is nil.
	ErrInvalidLogger = errors.New("invalid logger")
	// ErrFailedDecode is returned when a segment contains a record that
	// cannot be decoded as a [Log].
	ErrFailedDecode = errors.New("failed decode wal log")
	// ErrApplyRecord is returned when a recovered record has missing or
	// malformed arguments for its command id.
	ErrApplyRecord = errors.New("failed apply wal record")
)

// CommandHashTable is the subset of the hash-table API the WAL uses when
// applying recovered or just-flushed records.
type CommandHashTable interface {
	Set(key, value string)
	Del(key string)
}

// Wal buffers write requests, flushes them to segment files in batches, and
// replays them on startup. All exported methods are safe for concurrent use.
type Wal struct {
	log       *slog.Logger
	walConfig walConfig

	walWriter *WalWriter
	walReader *WalReader

	batch []WriteRequest
	mutex sync.Mutex

	batches           chan []WriteRequest
	activeBatchVolume int64

	commandHashTable CommandHashTable
}
type walConfig struct {
	flushingBatchTimeout time.Duration
	flushingBatchCount   int
	flushingBatchVolume  int
	maxSegmentSize       int64
	segmentStoragePath   string
	maskName             string
}

// NewWal builds a Wal from its dependencies. The returned value is not yet
// running — call [Wal.Recovery] to replay existing segments and
// [Wal.Start] to launch the background flusher.
func NewWal(
	log *slog.Logger,
	cfg *config.WalConfig,
	segment *filesystem.Segment,
	cmd CommandHashTable,
) (*Wal, error) {
	if log == nil {
		return nil, ErrInvalidLogger
	}

	walWriter := NewWalWriter(log, segment)
	walReader := NewWalReader(log, segment)

	return &Wal{
		log: log,
		walConfig: walConfig{
			flushingBatchTimeout: cfg.FlushingBatchTimeout,
			flushingBatchCount:   cfg.FlushingBatchCount,
			flushingBatchVolume:  cfg.FlushingBatchVolume,
			maxSegmentSize:       cfg.MaxSegmentSize,
			segmentStoragePath:   cfg.SegmentStoragePath,
			maskName:             cfg.MaskName,
		},
		batch:            make([]WriteRequest, 0),
		batches:          make(chan []WriteRequest, 5),
		walWriter:        walWriter,
		walReader:        walReader,
		commandHashTable: cmd,
	}, nil
}

// Recovery replays every segment file in the configured directory, applying
// each record to the hash table in LSN order. After replay it opens the
// most recent segment in append mode (or creates the first one) so that
// subsequent flushes continue writing to it.
//
//nolint:gocognit // WAL recovery keeps decode-sort-apply inline for readability
func (wal *Wal) Recovery(_ string) error {
	const op = "wal/Wal.Recovery"

	filesList, err := wal.walReader.segment.GetList()
	if err != nil {
		wal.log.Debug("failed to get wal segment files list",
			slog.String("operation", op),
			slog.String("error", err.Error()))
		return ErrFailedToReadDirectory
	}

	for _, file := range filesList {
		data, err := wal.walReader.segment.Read(file)
		if err != nil {
			wal.log.Debug("failed to read wal segment file",
				slog.String("operation", op),
				slog.String("file", file),
				slog.String("error", err.Error()))
			return err
		}
		buffer := bytes.NewBuffer(data)
		decoder := gob.NewDecoder(buffer)

		logs := make([]Log, 0)

		for {
			var record Log
			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				wal.log.Debug("failed to decode wal log",
					slog.String("operation", op),
					slog.String("error", err.Error()))

				return ErrFailedDecode
			}
			logs = append(logs, record)
		}

		sort.Slice(logs, func(i, j int) bool {
			return logs[i].LSN < logs[j].LSN
		})

		for i := range logs {
			err = wal.apply(&logs[i])
			if err != nil {
				return err
			}
		}
	}

	var file *os.File
	if len(filesList) == 0 {
		file, err = wal.walReader.segment.CreateFile()
		if err != nil {
			return err
		}

	} else {
		file, err = os.OpenFile(filesList[len(filesList)-1], os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			wal.log.Debug("failed to open last segment",
				slog.String("operation", op),
				slog.String("error", err.Error()))

			return ErrFailedOpenLastSegment
		}
	}

	wal.walWriter.segment.SetFile(file)

	return nil
}

func (wal *Wal) apply(record *Log) error {
	switch record.CommandID {
	case 1:
		if len(record.Arguments) < 2 {
			wal.log.Debug("invalid set command in wal log", slog.Any("record", record))
			return ErrApplyRecord
		}
		wal.commandHashTable.Set(record.Arguments[0], record.Arguments[1])
	case 2:
		if len(record.Arguments) < 1 {
			wal.log.Debug("invalid del command in wal log", slog.Any("record", record))
			return ErrApplyRecord
		}
		wal.commandHashTable.Del(record.Arguments[0])
	}
	return nil
}

// Start launches the background flusher goroutine. It flushes whenever a
// sealed batch arrives on the internal channel or the ticker fires, and
// drains everything in flight before returning when ctx is cancelled.
//
//nolint:gocognit // ticker + context drain logic is intentionally kept in one flusher loop
func (wal *Wal) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(wal.walConfig.flushingBatchTimeout)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				for {
					select {
					case batch := <-wal.batches:
						err := wal.flushBatch(batch)
						if err != nil {
							wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
						}
					default:
						batch := wal.SealBatch()
						err := wal.flushBatch(batch)
						if err != nil {
							wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
						}
						return
					}
				}
			default:
			}

			select {
			case <-ctx.Done():
				for {
					select {
					case batch := <-wal.batches:
						err := wal.flushBatch(batch)
						if err != nil {
							wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
						}
					default:
						batch := wal.SealBatch()
						err := wal.flushBatch(batch)
						if err != nil {
							wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
						}
						return
					}
				}
			case batch := <-wal.batches:
				ticker.Reset(wal.walConfig.flushingBatchTimeout)
				err := wal.flushBatch(batch)
				if err != nil {
					wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
				}
			case <-ticker.C:
				batch := wal.SealBatch()
				err := wal.flushBatch(batch)
				if err != nil {
					wal.log.Error("failed to flush wal batch", slog.String("error", err.Error()))
				}
			}
		}
	}()
}

// SealBatch atomically takes ownership of the in-memory batch and returns
// it, resetting the batch state. The returned slice may be empty.
func (wal *Wal) SealBatch() []WriteRequest {
	var sealedBatch []WriteRequest
	concurrency.WithLock(&wal.mutex, func() {
		sealedBatch = wal.batch
		wal.batch = nil
		wal.activeBatchVolume = 0
	})

	return sealedBatch
}

func (wal *Wal) flushBatch(batch []WriteRequest) error {
	if len(batch) > 0 {
		err := wal.walWriter.Write(batch)
		if err != nil {
			return err
		}

		sort.Slice(batch, func(i, j int) bool {
			return batch[i].log.LSN < batch[j].log.LSN
		})

		for i := range batch {
			err := wal.apply(&batch[i].log)
			if err != nil {
				wal.log.Error("failed to apply wal record",
					slog.Int64("lsn", batch[i].log.LSN),
					slog.String("error", err.Error()))
			}
		}
	}
	return nil
}

func (wal *Wal) push(ctx context.Context, commandID int, args []string) error {
	lsn := contextid.GetTxIDFromContext(ctx)
	record := NewWriteRequest(lsn, commandID, args)
	sealedBatch := make([]WriteRequest, 0)

	concurrency.WithLock(&wal.mutex, func() {
		wal.batch = append(wal.batch, *record)

		element := Log{
			LSN:       lsn,
			CommandID: commandID,
			Arguments: args[0:],
		}
		buf := &bytes.Buffer{}
		//nolint:gosec // size estimate only; real encode happens in walWriter.Write
		_ = element.Encode(buf)

		wal.activeBatchVolume += int64(buf.Len())

		if (len(wal.batch) >= wal.walConfig.flushingBatchCount) ||
			(wal.activeBatchVolume >= int64(wal.walConfig.flushingBatchVolume)) {
			sealedBatch = wal.batch
			wal.batch = nil
			wal.activeBatchVolume = 0
		}
	})
	if len(sealedBatch) > 0 {
		wal.batches <- sealedBatch
	}

	future := record.FutureResponse()
	err := future.Get()
	return err
}

// Set logs a SET(key, value) operation. The call blocks until the record
// is durably flushed, then returns the flush result.
func (wal *Wal) Set(ctx context.Context, key, value string) error {
	return wal.push(ctx, compute.CommandSetID, []string{key, value})
}

// Del logs a DEL(key) operation. The call blocks until the record is
// durably flushed, then returns the flush result.
func (wal *Wal) Del(ctx context.Context, key string) error {
	return wal.push(ctx, compute.CommandDelID, []string{key})
}
