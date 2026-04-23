package inmemory

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashTableSetAndGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(*HashTable)
		key      string
		value    string
		wantOK   bool
		wantResp string
	}{
		{
			name:     "set and get",
			key:      "foo",
			value:    "bar",
			wantOK:   true,
			wantResp: "bar",
		},
		{
			name: "overwrite existing key",
			setup: func(h *HashTable) {
				h.Set("dup", "old")
			},
			key:      "dup",
			value:    "new",
			wantOK:   true,
			wantResp: "new",
		},
		{
			name:     "empty key allowed",
			key:      "",
			value:    "empty-key",
			wantOK:   true,
			wantResp: "empty-key",
		},
		{
			name:   "get missing returns false",
			key:    "missing",
			value:  "",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewHashTable()
			if tc.setup != nil {
				tc.setup(h)
			}

			if tc.wantOK {
				h.Set(tc.key, tc.value)
			}

			got, ok := h.Get(tc.key)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantResp, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestHashTableDel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*HashTable)
		delKey     string
		expectOK   bool
		expectSize int
	}{
		{
			name: "delete existing key",
			setup: func(h *HashTable) {
				h.Set("foo", "bar")
			},
			delKey:     "foo",
			expectOK:   false,
			expectSize: 0,
		},
		{
			name:       "delete missing key is no-op",
			delKey:     "missing",
			expectOK:   false,
			expectSize: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewHashTable()
			if tc.setup != nil {
				tc.setup(h)
			}

			h.Del(tc.delKey)
			_, ok := h.Get(tc.delKey)
			assert.Equal(t, tc.expectOK, ok)
			assert.Equal(t, tc.expectSize, len(h.data))
		})
	}
}

func TestHashTableConcurrentAccess_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		workers int
	}{
		{
			name:    "concurrent set and get",
			workers: 50,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewHashTable()

			var wg sync.WaitGroup
			wg.Add(tc.workers * 2)

			for i := range tc.workers {
				go func() {
					defer wg.Done()
					key := "k" + strconv.Itoa(i)
					val := "v" + strconv.Itoa(i)
					h.Set(key, val)
				}()

				go func() {
					defer wg.Done()
					_, _ = h.Get("k" + strconv.Itoa(i))
				}()
			}

			wg.Wait()
			require.Equal(t, tc.workers, len(h.data))

			for i := range tc.workers {
				key := "k" + strconv.Itoa(i)
				val := "v" + strconv.Itoa(i)
				got, ok := h.Get(key)
				require.True(t, ok)
				require.Equal(t, val, got)
			}
		})
	}
}
