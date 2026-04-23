// Package filesystem stores WAL segments on disk. A segment is a plain file
// that holds a back-to-back gob stream of [wal.Log] records. Segments rotate
// whenever the configured max size is exceeded.
package filesystem

import (
	"errors"
	"log/slog"
	"os"
)

// Sentinel errors returned by the segment API.
var (
	// ErrFailedSaveFile wraps any write failure against an open segment.
	ErrFailedSaveFile = errors.New("failed to save file")
	// ErrFailedFileCreate wraps failures while opening a new segment file.
	ErrFailedFileCreate = errors.New("failed to create file")
)

// Segment represents the currently-active on-disk WAL file. It tracks its
// own size so the caller can rotate to a new segment once the configured
// maximum is reached. Not safe for concurrent use; the WAL serialises
// access externally.
type Segment struct {
	log *slog.Logger

	file      *os.File
	directory string

	maskName       string
	segmentSize    int64
	maxSegmentSize int64
}

// NewSegment returns a Segment bound to directory. Files created in this
// directory will be named maskName + "<nanosecond timestamp>.log". The
// segment itself is not created until the first write (or an explicit
// [Segment.CreateFile] call).
func NewSegment(log *slog.Logger, directory, maskName string, maxSegmentSize int64) *Segment {
	return &Segment{
		log:            log,
		directory:      directory,
		maskName:       maskName,
		maxSegmentSize: maxSegmentSize,
	}
}

// CreateFile opens a fresh segment file in append-exclusive mode and makes
// it the segment's current write target.
func (seg *Segment) CreateFile() (*os.File, error) {
	file, err := CreateFile(seg.directory, seg.maskName)
	if err != nil {
		seg.log.Debug("failed to create file",
			slog.String("dir", seg.directory), slog.Any("err", err))

		return nil, ErrFailedFileCreate
	}
	seg.file = file
	seg.segmentSize = 0
	return file, nil
}

// Read loads the entire contents of filePath into memory.
func (seg *Segment) Read(filePath string) ([]byte, error) {
	return ReadFile(filePath)
}

// GetList returns the sorted list of segment files in the segment's
// directory.
func (seg *Segment) GetList() ([]string, error) {
	return GetFileList(seg.directory)
}

// Write appends data to the current segment, rotating to a new file first
// if adding data would exceed maxSegmentSize. It creates the first segment
// lazily.
func (seg *Segment) Write(data []byte) error {
	if seg.file == nil {
		if _, err := seg.CreateFile(); err != nil {
			return err
		}
	}

	if seg.maxSegmentSize > 0 && seg.segmentSize > 0 && seg.segmentSize+int64(len(data)) > seg.maxSegmentSize {
		err := seg.getNextSegment()
		if err != nil {
			return err
		}
	}

	writtenBytes, err := WriteFile(seg.file, data)
	if err != nil {
		seg.log.Debug("failed to write data to segment file", slog.String("file", seg.file.Name()), slog.Any("err", err))
		return ErrFailedSaveFile
	}

	seg.segmentSize += int64(writtenBytes)

	return nil
}

func (seg *Segment) getNextSegment() error {
	if seg.file != nil {
		err := seg.file.Close()
		if err != nil {
			seg.log.Debug("failed to close previous segment during rotation",
				slog.String("file", seg.file.Name()),
				slog.String("error", err.Error()))
		}
	}

	file, err := CreateFile(seg.directory, seg.maskName)
	if err != nil {
		return err
	}

	seg.file = file
	seg.segmentSize = 0
	return nil
}

// SetFile adopts an already-open file as the active segment and initialises
// segmentSize from the file's on-disk size. Used by recovery to append to
// the last segment left behind by the previous run.
func (seg *Segment) SetFile(file *os.File) {
	seg.file = file
	seg.segmentSize = 0

	if file == nil {
		return
	}

	stat, err := file.Stat()
	if err != nil {
		seg.log.Debug("failed to read segment file size", slog.String("file", file.Name()), slog.Any("err", err))
		return
	}

	seg.segmentSize = stat.Size()
}
