package wal

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

type stubCommandHashTable struct {
	setCalls int
	delCalls int
	calls    []string
}

func (s *stubCommandHashTable) Set(_, _ string) {
	s.setCalls++
	s.calls = append(s.calls, "set")
}

func (s *stubCommandHashTable) Del(_ string) {
	s.delCalls++
	s.calls = append(s.calls, "del")
}

func TestNewWal(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()

	tests := []struct {
		name      string
		logNil    bool
		cfgFn     func(t *testing.T) *config.WalConfig
		wantError error
		wantPanic bool
	}{
		{
			name:      "nil logger",
			logNil:    true,
			cfgFn:     func(t *testing.T) *config.WalConfig { return baseWalConfig(t.TempDir()) },
			wantError: ErrInvalidLogger,
		},
		{
			name:      "nil config panics",
			cfgFn:     nil,
			wantPanic: true,
		},
		{
			name:      "nil logger with nil config returns ErrInvalidLogger",
			logNil:    true,
			cfgFn:     nil,
			wantError: ErrInvalidLogger,
		},
		{
			name: "success sets config and batch",
			cfgFn: func(t *testing.T) *config.WalConfig {
				cfg := baseWalConfig(t.TempDir())
				cfg.FlushingBatchCount = 3
				return cfg
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := log
			if tc.logNil {
				logger = nil
			}

			var cfg *config.WalConfig
			if tc.cfgFn != nil {
				cfg = tc.cfgFn(t)
			}

			cmd := &stubCommandHashTable{}
			var seg *filesystem.Segment
			if cfg != nil {
				seg = filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
			}

			if tc.wantPanic {
				require.Panics(t, func() {
					_, _ = NewWal(logger, cfg, seg, cmd)
				})
				return
			}

			w, err := NewWal(logger, cfg, seg, cmd)
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
				require.Nil(t, w)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, w)
			assert.Equal(t, cfg.FlushingBatchTimeout, w.walConfig.flushingBatchTimeout)
			assert.Equal(t, cfg.FlushingBatchCount, w.walConfig.flushingBatchCount)
			assert.Equal(t, cfg.FlushingBatchVolume, w.walConfig.flushingBatchVolume)
			assert.Equal(t, cfg.MaxSegmentSize, w.walConfig.maxSegmentSize)
			assert.Equal(t, cfg.SegmentStoragePath, w.walConfig.segmentStoragePath)
			assert.Equal(t, cfg.MaskName, w.walConfig.maskName)
			require.NotNil(t, w.batch)
			assert.Empty(t, w.batch)
		})
	}
}

func TestWalRecovery(t *testing.T) {
	t.Parallel()

	log := slogdiscard.NewDiscardLogger()

	t.Run("missing directory returns ErrFailedToReadDirectory", func(t *testing.T) {
		t.Parallel()

		cmd := &stubCommandHashTable{}
		dir := filepath.Join(t.TempDir(), "missing")
		cfg := baseWalConfig(dir)
		seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
		w, err := NewWal(log, cfg, seg, cmd)
		require.NoError(t, err)

		err = w.Recovery(dir)
		require.ErrorIs(t, err, ErrFailedToReadDirectory)
	})

	t.Run("invalid log data returns ErrFailedDecode", func(t *testing.T) {
		t.Parallel()

		cmd := &stubCommandHashTable{}
		dir := t.TempDir()
		file := filepath.Join(dir, "wal_0001.log")
		require.NoError(t, os.WriteFile(file, []byte("bad"), 0o644))

		cfg := baseWalConfig(dir)
		seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
		w, err := NewWal(log, cfg, seg, cmd)
		require.NoError(t, err)

		err = w.Recovery(dir)
		require.ErrorIs(t, err, ErrFailedDecode)
		require.Empty(t, cmd.calls)
	})

	t.Run("applies records in LSN order", func(t *testing.T) {
		t.Parallel()

		cmd := &stubCommandHashTable{}
		dir := t.TempDir()
		file := filepath.Join(dir, "wal_0001.log")

		records := []Log{
			{LSN: 2, CommandID: 1, Arguments: []string{"a", "1"}},
			{LSN: 1, CommandID: 2, Arguments: []string{"b"}},
		}
		var buf bytes.Buffer
		encoder := gob.NewEncoder(&buf)
		for _, rec := range records {
			require.NoError(t, encoder.Encode(rec))
		}
		require.NoError(t, os.WriteFile(file, buf.Bytes(), 0o644))

		cfg := baseWalConfig(dir)
		seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
		w, err := NewWal(log, cfg, seg, cmd)
		require.NoError(t, err)

		require.NoError(t, w.Recovery(dir))
		require.Equal(t, []string{"del", "set"}, cmd.calls)
	})

	t.Run("replays records from multiple segment files", func(t *testing.T) {
		t.Parallel()

		cmd := &stubCommandHashTable{}
		dir := t.TempDir()
		files := []string{
			filepath.Join(dir, "wal_0001.log"),
			filepath.Join(dir, "wal_0002.log"),
		}
		records := [][]Log{
			{{LSN: 1, CommandID: 1, Arguments: []string{"a", "1"}}},
			{{LSN: 2, CommandID: 2, Arguments: []string{"a"}}},
		}

		for idx, file := range files {
			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			for _, rec := range records[idx] {
				require.NoError(t, encoder.Encode(rec))
			}
			require.NoError(t, os.WriteFile(file, buf.Bytes(), 0o644))
		}

		cfg := baseWalConfig(dir)
		seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
		w, err := NewWal(log, cfg, seg, cmd)
		require.NoError(t, err)

		require.NoError(t, w.Recovery(dir))
		require.Equal(t, []string{"set", "del"}, cmd.calls)
	})

	t.Run("creates file when directory is empty", func(t *testing.T) {
		t.Parallel()

		cmd := &stubCommandHashTable{}
		dir := t.TempDir()
		cfg := baseWalConfig(dir)
		seg := filesystem.NewSegment(log, cfg.SegmentStoragePath, cfg.MaskName, cfg.MaxSegmentSize)
		w, err := NewWal(log, cfg, seg, cmd)
		require.NoError(t, err)

		require.NoError(t, w.Recovery(dir))

		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		require.NotEmpty(t, entries)
	})
}
