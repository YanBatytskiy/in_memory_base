package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/YanBatytskiy/in_memory_base/internal/database/storage"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
)

type mapStorage struct {
	data map[string]string
}

func newMapStorage() *mapStorage {
	return &mapStorage{data: make(map[string]string)}
}

func (m *mapStorage) Set(ctx context.Context, key, value string) error {
	_ = ctx
	m.data[key] = value
	return nil
}

func (m *mapStorage) Del(ctx context.Context, key string) error {
	_ = ctx
	delete(m.data, key)
	return nil
}

func (m *mapStorage) Get(ctx context.Context, key string) (string, error) {
	_ = ctx
	value, ok := m.data[key]
	if !ok {
		return "", storage.ErrKeyNotFound
	}
	return value, nil
}

func TestStorage(t *testing.T) {
	t.Parallel()

	type (
		setupFn func(ctx context.Context, s *storage.Storage)
	)

	tests := []struct {
		name    string
		setup   setupFn
		run     func(ctx context.Context, s *storage.Storage) (string, error)
		want    string
		wantErr error
	}{
		{
			name: "set ok",
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return "", s.Set(ctx, "k", "v")
			},
		},
		{
			name: "get ok after set",
			setup: func(ctx context.Context, s *storage.Storage) {
				require.NoError(t, s.Set(ctx, "k", "v"))
			},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return s.Get(ctx, "k")
			},
			want: "v",
		},
		{
			name:    "get not found returns error",
			run:     func(ctx context.Context, s *storage.Storage) (string, error) { return s.Get(ctx, "missing") },
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name: "del ok after set",
			setup: func(ctx context.Context, s *storage.Storage) {
				require.NoError(t, s.Set(ctx, "k", "v"))
			},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return "", s.Del(ctx, "k")
			},
		},
		{
			name: "del not found",
			run:  func(ctx context.Context, s *storage.Storage) (string, error) { return "", s.Del(ctx, "missing") },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := slogdiscard.NewDiscardLogger()

			eng := newMapStorage()
			s, err := storage.NewStorage(logger, eng)
			require.NoError(t, err)

			if tc.setup != nil {
				tc.setup(ctx, s)
			}

			got, err := tc.run(ctx, s)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

type errorStorage struct {
	setErr error
	getErr error
	delErr error
	value  string
}

func (e *errorStorage) Set(ctx context.Context, key, value string) error {
	_ = ctx
	_ = key
	_ = value
	return e.setErr
}

func (e *errorStorage) Del(ctx context.Context, key string) error {
	_ = ctx
	_ = key
	return e.delErr
}

func (e *errorStorage) Get(ctx context.Context, key string) (string, error) {
	_ = ctx
	_ = key
	return e.value, e.getErr
}

func TestStorageErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	tests := []struct {
		name    string
		engine  *errorStorage
		run     func(ctx context.Context, s *storage.Storage) (string, error)
		wantErr error
	}{
		{
			name:   "set error",
			engine: &errorStorage{setErr: boom},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return "", s.Set(ctx, "k", "v")
			},
			wantErr: boom,
		},
		{
			name:   "get not found error",
			engine: &errorStorage{getErr: storage.ErrKeyNotFound},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return s.Get(ctx, "k")
			},
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name:   "get other error",
			engine: &errorStorage{getErr: boom},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return s.Get(ctx, "k")
			},
			wantErr: boom,
		},
		{
			name:   "del not found error",
			engine: &errorStorage{delErr: storage.ErrKeyNotFound},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return "", s.Del(ctx, "k")
			},
			wantErr: storage.ErrKeyNotFound,
		},
		{
			name:   "del other error",
			engine: &errorStorage{delErr: boom},
			run: func(ctx context.Context, s *storage.Storage) (string, error) {
				return "", s.Del(ctx, "k")
			},
			wantErr: boom,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			logger := slogdiscard.NewDiscardLogger()

			s, err := storage.NewStorage(logger, tc.engine)
			require.NoError(t, err)

			_, err = tc.run(ctx, s)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}
