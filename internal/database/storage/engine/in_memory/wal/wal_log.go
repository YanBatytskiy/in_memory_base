package wal

import (
	"bytes"
	"encoding/gob"
)

// Log is the on-disk representation of a single WAL record. Records are
// encoded back-to-back inside a segment using [encoding/gob].
type Log struct {
	// LSN is the monotonically increasing Log Sequence Number assigned by
	// the engine when the operation was accepted.
	LSN int64
	// CommandID identifies the operation (see compute.CommandSetID etc).
	CommandID int
	// Arguments are the command arguments in token order.
	Arguments []string
}

// Encode serialises the record into buffer using a fresh [gob.Encoder].
func (log *Log) Encode(buffer *bytes.Buffer) error {
	encoder := gob.NewEncoder(buffer)
	return log.EncodeTo(encoder)
}

// Decode deserialises the record from buffer using a fresh [gob.Decoder].
func (log *Log) Decode(buffer *bytes.Buffer) error {
	decoder := gob.NewDecoder(buffer)
	return log.DecodeFrom(decoder)
}

// EncodeTo writes the record using the given encoder. Use this when batching
// several records into one gob stream.
func (log *Log) EncodeTo(encoder *gob.Encoder) error {
	return encoder.Encode(*log)
}

// DecodeFrom reads the next record from decoder.
func (log *Log) DecodeFrom(decoder *gob.Decoder) error {
	return decoder.Decode(log)
}
