package wal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/concurrency"
)

func TestNewWriteRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lsn   int64
		cmdID int
		args  []string
	}{
		{
			name:  "basic",
			lsn:   10,
			cmdID: 1,
			args:  []string{"a", "b"},
		},
		{
			name:  "empty args",
			lsn:   0,
			cmdID: 2,
			args:  nil,
		},
		{
			name:  "single arg",
			lsn:   42,
			cmdID: 3,
			args:  []string{"only"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := NewWriteRequest(tc.lsn, tc.cmdID, tc.args)
			require.NotNil(t, req)
			assert.Equal(t, tc.lsn, req.log.LSN)
			assert.Equal(t, tc.cmdID, req.log.CommandID)
			assert.Equal(t, tc.args, req.log.Arguments)
		})
	}
}

func TestWriteRequestFutureResponse(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first")
	errSecond := errors.New("second")

	tests := []struct {
		name               string
		getFutureBeforeSet bool
		firstErr           error
		secondErr          error
		wantErr            error
	}{
		{
			name:               "future before set, nil response",
			getFutureBeforeSet: true,
			firstErr:           nil,
			wantErr:            nil,
		},
		{
			name:               "future before set, error response",
			getFutureBeforeSet: true,
			firstErr:           errFirst,
			wantErr:            errFirst,
		},
		{
			name:               "future after set, error response",
			getFutureBeforeSet: false,
			firstErr:           errFirst,
			wantErr:            errFirst,
		},
		{
			name:               "double set keeps first",
			getFutureBeforeSet: true,
			firstErr:           errFirst,
			secondErr:          errSecond,
			wantErr:            errFirst,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := NewWriteRequest(1, 1, []string{"a"})
			var future concurrency.FutureError
			if tc.getFutureBeforeSet {
				future = req.FutureResponse()
			}

			req.SetResponse(tc.firstErr)
			if tc.secondErr != nil {
				req.SetResponse(tc.secondErr)
			}

			if !tc.getFutureBeforeSet {
				future = req.FutureResponse()
			}

			got := future.Get()
			if tc.wantErr != nil {
				require.ErrorIs(t, got, tc.wantErr)
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestWriteRequestMultipleFutures(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first")

	tests := []struct {
		name       string
		order      []int
		wantFirst  error
		wantSecond error
	}{
		{
			name:       "read first then second",
			order:      []int{0, 1},
			wantFirst:  errFirst,
			wantSecond: nil,
		},
		{
			name:       "read second then first",
			order:      []int{1, 0},
			wantFirst:  errFirst,
			wantSecond: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := NewWriteRequest(1, 1, []string{"a"})
			futures := []concurrency.FutureError{
				req.FutureResponse(),
				req.FutureResponse(),
			}

			req.SetResponse(errFirst)

			first := futures[tc.order[0]].Get()
			second := futures[tc.order[1]].Get()

			if tc.wantFirst != nil {
				require.ErrorIs(t, first, tc.wantFirst)
			} else {
				require.NoError(t, first)
			}
			if tc.wantSecond != nil {
				require.ErrorIs(t, second, tc.wantSecond)
			} else {
				require.NoError(t, second)
			}
		})
	}
}
