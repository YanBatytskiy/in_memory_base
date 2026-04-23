package inmemory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/wal"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

func TestNewEngine_Errors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	log := slogdiscard.NewDiscardLogger()

	tests := []struct {
		name      string
		logNil    bool
		cfgFn     func(t *testing.T) *config.Config
		wantError error
	}{
		{
			name:      "nil logger",
			logNil:    true,
			cfgFn:     minimalConfig,
			wantError: ErrInvalidLogger,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.cfgFn(t)
			logger := log
			if tc.logNil {
				logger = nil
			}

			engine, err := NewEngine(ctx, logger, cfg, nil, nil)
			require.Error(t, err)
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
			}
			require.Nil(t, engine)
		})
	}
}

func TestEngineOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name      string
		prepare   func(*Engine)
		action    func(*Engine) (string, error)
		wantValue string
		wantErr   error
	}{
		{
			name: "set ok",
			action: func(e *Engine) (string, error) {
				return "", e.Set(ctx, "foo", "bar")
			},
		},
		{
			name: "get missing returns not found",
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "missing")
			},
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name: "delete missing key is no-op",
			prepare: func(e *Engine) {
				require.NoError(t, e.Del(ctx, "missing"))
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "missing")
			},
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name: "get returns stored value",
			prepare: func(e *Engine) {
				e.queryEngine.hashTable.Set("foo", "bar")
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "foo")
			},
			wantValue: "bar",
		},
		{
			name: "overwrite existing key",
			prepare: func(e *Engine) {
				e.queryEngine.hashTable.Set("dup", "old")
				e.queryEngine.hashTable.Set("dup", "new")
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "dup")
			},
			wantValue: "new",
		},
		{
			name: "delete existing key",
			prepare: func(e *Engine) {
				e.queryEngine.hashTable.Set("foo", "bar")
				e.queryEngine.hashTable.Del("foo")
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "foo")
			},
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name: "multiple keys remain isolated",
			prepare: func(e *Engine) {
				e.queryEngine.hashTable.Set("foo", "bar")
				e.queryEngine.hashTable.Set("baz", "qux")
				e.queryEngine.hashTable.Del("foo")
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "baz")
			},
			wantValue: "qux",
		},
		{
			name: "empty key allowed",
			prepare: func(e *Engine) {
				e.queryEngine.hashTable.Set("", "empty")
			},
			action: func(e *Engine) (string, error) {
				return e.Get(ctx, "")
			},
			wantValue: "empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := newTestEngine(t)

			if tc.prepare != nil {
				tc.prepare(engine)
			}

			got, err := tc.action(engine)
			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, got)
		})
	}
}

type failingCommandEngine struct {
	err error
}

func (f failingCommandEngine) Set(context.Context, string, string) error {
	return f.err
}

func (f failingCommandEngine) Del(context.Context, string) error {
	return f.err
}

func TestEnginePropagatesCommandErrors(t *testing.T) {
	t.Parallel()

	errWrite := errors.New("wal write failed")
	engine := &Engine{
		log:           slogdiscard.NewDiscardLogger(),
		currentLSN:    NewIDGenerator(0),
		commandEngine: failingCommandEngine{err: errWrite},
	}

	require.ErrorIs(t, engine.Set(context.Background(), "key", "value"), errWrite)
	require.ErrorIs(t, engine.Del(context.Background(), "key"), errWrite)
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()

	ctx := context.Background()
	log := slogdiscard.NewDiscardLogger()

	cfg := minimalConfig(t)
	hashTable := NewHashTable()
	segment := filesystem.NewSegment(log, cfg.Wal.SegmentStoragePath, cfg.Wal.MaskName, cfg.Wal.MaxSegmentSize)
	w, err := wal.NewWal(log, cfg.Wal, segment, hashTable)
	require.NoError(t, err)
	require.NoError(t, w.Recovery(cfg.Wal.SegmentStoragePath))

	walCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	w.Start(walCtx)

	engine, err := NewEngine(ctx, log, cfg, w, hashTable)
	require.NoError(t, err)

	return engine
}

func minimalConfig(t *testing.T) *config.Config {
	t.Helper()

	walDir := t.TempDir()
	walFile := filepath.Join(walDir, "segment_0000.log")
	require.NoError(t, os.WriteFile(walFile, nil, 0o644))

	return &config.Config{
		EngineType: "in_memory",
		Logger: &config.LoggingConfig{
			Level: "prod",
		},
		Wal: &config.WalConfig{
			FlushingBatchTimeout: 10 * time.Millisecond,
			FlushingBatchCount:   1,
			MaxSegmentSize:       1024,
			SegmentStoragePath:   walDir,
			MaskName:             "segment_",
		},
	}
}
