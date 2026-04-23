package wal

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/concurrency"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

type stubSegment struct {
	writes int
	data   []byte
	err    error
	file   *os.File
}

func (s *stubSegment) Write(data []byte) error {
	s.writes++
	s.data = append([]byte(nil), data...)
	return s.err
}

func (s *stubSegment) SetFile(file *os.File) {
	s.file = file
}

func TestWalWriterWritePanics(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()

	tests := []struct {
		name       string
		requests   []WriteRequest
		nilSegment bool
		wantPanic  bool
		wantEmpty  bool
	}{
		{
			name:      "nil requests panics",
			requests:  nil,
			wantEmpty: true,
		},
		{
			name:      "empty slice panics",
			requests:  []WriteRequest{},
			wantEmpty: true,
		},
		{
			name:     "single request panics",
			requests: []WriteRequest{*NewWriteRequest(1, 1, []string{"a"})},
		},
		{
			name:     "multiple requests panics",
			requests: []WriteRequest{*NewWriteRequest(1, 1, []string{"a"}), *NewWriteRequest(2, 2, []string{"b"})},
		},
		{
			name:       "nil segment panics",
			requests:   []WriteRequest{*NewWriteRequest(1, 1, []string{"a"})},
			nilSegment: true,
			wantPanic:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubSegment{}
			var seg segmentWrite = stub
			if tc.nilSegment {
				seg = nil
			}
			writer := NewWalWriter(log, seg)

			if tc.wantPanic {
				require.Panics(t, func() {
					_ = writer.Write(tc.requests)
				})
				return
			}

			require.NotPanics(t, func() {
				_ = writer.Write(tc.requests)
			})
			assert.Equal(t, 1, stub.writes)
			if tc.wantEmpty {
				assert.Empty(t, stub.data)
			} else {
				assert.NotEmpty(t, stub.data)
			}
		})
	}
}

func TestWalWriterNotification(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()
	errWrite := errors.New("write failed")
	errIgnored := errors.New("ignored")
	errFirst := errors.New("first")

	tests := []struct {
		name         string
		responseErr  error
		requestCount int
		preSet       bool
		preSetErr    error
		wantErr      error
	}{
		{
			name:         "propagates nil response",
			responseErr:  nil,
			requestCount: 2,
			wantErr:      nil,
		},
		{
			name:         "propagates error response",
			responseErr:  errWrite,
			requestCount: 3,
			wantErr:      errWrite,
		},
		{
			name:         "empty requests is no-op",
			responseErr:  errIgnored,
			requestCount: 0,
			wantErr:      nil,
		},
		{
			name:         "pre-set response is not overwritten",
			responseErr:  errIgnored,
			requestCount: 2,
			preSet:       true,
			preSetErr:    errFirst,
			wantErr:      errFirst,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			writer := NewWalWriter(log, &stubSegment{})
			requests := make([]WriteRequest, 0, tc.requestCount)
			futures := make([]concurrency.FutureError, 0, tc.requestCount)

			for i := range tc.requestCount {
				req := NewWriteRequest(int64(i+1), 1, []string{"a"})
				requests = append(requests, *req)
				futures = append(futures, requests[i].FutureResponse())
			}

			if tc.preSet {
				for i := range requests {
					requests[i].SetResponse(tc.preSetErr)
				}
			}

			writer.notification(tc.responseErr, requests)

			for _, future := range futures {
				err := future.Get()
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}
