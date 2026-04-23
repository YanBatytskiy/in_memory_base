package database_test

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
	"github.com/YanBatytskiy/in_memory_base/internal/database"
	"github.com/YanBatytskiy/in_memory_base/internal/database/compute"
	databasemocks "github.com/YanBatytskiy/in_memory_base/internal/database/mocks"
	storagepkg "github.com/YanBatytskiy/in_memory_base/internal/database/storage"
	inmemory "github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/filesystem"
	"github.com/YanBatytskiy/in_memory_base/internal/database/storage/engine/in_memory/wal"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

var (
	errSetFailed = errors.New("set failed")
	errGetFailed = errors.New("get failed")
	errDelFailed = errors.New("del failed")
)

func newDatabaseWithMocks(
	t *testing.T,
) (*database.Database, *databasemocks.MockComputeLayer, *databasemocks.MockStorageLayer) {
	t.Helper()

	computeMock := databasemocks.NewMockComputeLayer(t)
	storageMock := databasemocks.NewMockStorageLayer(t)
	logger := slogdiscard.NewDiscardLogger()

	db, err := database.NewDatabase(logger, computeMock, storageMock)
	require.NoError(t, err)

	return db, computeMock, storageMock
}

func TestDatabaseHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		parsed   []string
		parseErr error
		setup    func(ctx context.Context, storage *databasemocks.MockStorageLayer)
		want     string
	}{
		{
			name:   "set ok",
			raw:    "SET key value",
			parsed: []string{compute.CommandSet, "key", "value"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Set(ctx, "key", "value").Return(nil)
			},
			want: "OK",
		},
		{
			name:   "set error",
			raw:    "SET key value",
			parsed: []string{compute.CommandSet, "key", "value"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Set(ctx, "key", "value").Return(errSetFailed)
			},
			want: "failed SET",
		},
		{
			name:   "get ok",
			raw:    "GET key",
			parsed: []string{compute.CommandGet, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Get(ctx, "key").Return("123", nil)
			},
			want: "VALUE 123",
		},
		{
			name:   "get not found",
			raw:    "GET key",
			parsed: []string{compute.CommandGet, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Get(ctx, "key").Return("", storagepkg.ErrKeyNotFound)
			},
			want: "NOT_FOUND",
		},
		{
			name:   "get error",
			raw:    "GET key",
			parsed: []string{compute.CommandGet, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Get(ctx, "key").Return("", errGetFailed)
			},
			want: "failed GET",
		},
		{
			name:   "del ok",
			raw:    "DEL key",
			parsed: []string{compute.CommandDel, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Del(ctx, "key").Return(nil)
			},
			want: "DELETED",
		},
		{
			name:   "del not found treated as error",
			raw:    "DEL key",
			parsed: []string{compute.CommandDel, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Del(ctx, "key").Return(errors.New("HashTable.Get: not found"))
			},
			want: "failed DEL",
		},
		{
			name:   "del error",
			raw:    "DEL key",
			parsed: []string{compute.CommandDel, "key"},
			setup: func(ctx context.Context, storage *databasemocks.MockStorageLayer) {
				storage.EXPECT().Del(ctx, "key").Return(errDelFailed)
			},
			want: "failed DEL",
		},
		{
			name:   "invalid command token",
			raw:    "BAD key",
			parsed: []string{"BAD", "key"},
			want:   "invalid command",
		},
		{
			name:   "invalid quantity set",
			raw:    "SET onlykey",
			parsed: []string{compute.CommandSet, "onlykey"},
			want:   "must be two arguments",
		},
		{
			name:   "invalid quantity del",
			raw:    "DEL",
			parsed: []string{compute.CommandDel},
			want:   "must be one argument",
		},
		{
			name:   "invalid quantity get",
			raw:    "GET",
			parsed: []string{compute.CommandGet},
			want:   "must be one argument",
		},
		{
			name:     "parse invalid command",
			raw:      "S-ET key value",
			parseErr: compute.ErrInvalidCommand,
			want:     "failed to parse command",
		},
		{
			name:     "parse empty command",
			raw:      "   ",
			parseErr: compute.ErrEmptyCommand,
			want:     "failed to parse command",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, computeMock, storageMock := newDatabaseWithMocks(t)
			ctx := context.Background()

			computeMock.EXPECT().ParseAndValidate(ctx, tc.raw).Return(tc.parsed, tc.parseErr)

			if tc.setup != nil {
				tc.setup(ctx, storageMock)
			}

			got := db.DatabaseHandler(ctx, tc.raw)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDatabaseHandlerWithRealStorage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogdiscard.NewDiscardLogger()

	walDir := t.TempDir()
	walFile := filepath.Join(walDir, "segment_0000.log")
	require.NoError(t, os.WriteFile(walFile, nil, 0o644))

	cfg := &config.Config{
		EngineType: "in_memory",
		Logger:     &config.LoggingConfig{Level: "prod"},
		Wal: &config.WalConfig{
			FlushingBatchTimeout: time.Millisecond,
			FlushingBatchCount:   1,
			FlushingBatchVolume:  1024,
			MaxSegmentSize:       1024,
			SegmentStoragePath:   walDir,
			MaskName:             "segment_",
		},
	}

	hashTable := inmemory.NewHashTable()
	segment := filesystem.NewSegment(logger, cfg.Wal.SegmentStoragePath, cfg.Wal.MaskName, cfg.Wal.MaxSegmentSize)
	writeAheadLog, err := wal.NewWal(logger, cfg.Wal, segment, hashTable)
	require.NoError(t, err)
	require.NoError(t, writeAheadLog.Recovery(cfg.Wal.SegmentStoragePath))

	walCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	writeAheadLog.Start(walCtx)

	engine, err := inmemory.NewEngine(ctx, logger, cfg, writeAheadLog, hashTable)
	require.NoError(t, err)

	storageLayer, err := storagepkg.NewStorage(logger, engine)
	require.NoError(t, err)

	computeLayer, err := compute.NewCompute(logger)
	require.NoError(t, err)

	db, err := database.NewDatabase(logger, computeLayer, storageLayer)
	require.NoError(t, err)

	require.Equal(t, "OK", db.DatabaseHandler(ctx, "set key value"))
	require.Equal(t, "VALUE value", db.DatabaseHandler(ctx, "GET key"))
	require.Equal(t, "NOT_FOUND", db.DatabaseHandler(ctx, "GET missing"))
	require.Equal(t, "DELETED", db.DatabaseHandler(ctx, "DEL key"))
	require.Equal(t, "NOT_FOUND", db.DatabaseHandler(ctx, "GET key"))
}

func TestNewDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		logNil    bool
		compute   bool
		storage   bool
		wantError bool
	}{
		{
			name:    "success",
			compute: true,
			storage: true,
		},
		{
			name:      "nil logger",
			logNil:    true,
			compute:   true,
			storage:   true,
			wantError: true,
		},
		{
			name:      "nil compute",
			compute:   false,
			storage:   true,
			wantError: true,
		},
		{
			name:      "nil storage",
			compute:   true,
			storage:   false,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogdiscard.NewDiscardLogger()
			if tc.logNil {
				logger = nil
			}

			var computeMock database.ComputeLayer
			var storageMock database.StorageLayer
			if tc.compute {
				computeMock = databasemocks.NewMockComputeLayer(t)
			}
			if tc.storage {
				storageMock = databasemocks.NewMockStorageLayer(t)
			}

			db, err := database.NewDatabase(logger, computeMock, storageMock)
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, db)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, db)
		})
	}
}
