package wal

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogEncodeDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  Log
	}{
		{
			name: "basic",
			log: Log{
				LSN:       1,
				CommandID: 2,
				Arguments: []string{"a", "b"},
			},
		},
		{
			name: "nil arguments",
			log: Log{
				LSN:       0,
				CommandID: 3,
				Arguments: nil,
			},
		},
		{
			name: "empty arguments",
			log: Log{
				LSN:       7,
				CommandID: 4,
				Arguments: []string{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			require.NoError(t, tc.log.Encode(&buf))

			var got Log
			require.NoError(t, got.Decode(&buf))
			assert.Equal(t, tc.log.LSN, got.LSN)
			assert.Equal(t, tc.log.CommandID, got.CommandID)
			assert.ElementsMatch(t, tc.log.Arguments, got.Arguments)
		})
	}
}

func TestLogEncodePanics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  Log
	}{
		{
			name: "nil buffer panics",
			log: Log{
				LSN:       1,
				CommandID: 2,
				Arguments: []string{"a"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Panics(t, func() {
				_ = tc.log.Encode(nil)
			})
		})
	}
}

func TestLogDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty buffer",
			data: nil,
		},
		{
			name: "invalid data",
			data: []byte("not-gob"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			buf := bytes.NewBuffer(tc.data)
			var got Log
			require.Error(t, got.Decode(buf))
		})
	}
}
