package inmemory

import (
	"sync"
)

// HashTable is a concurrent key/value store backed by the built-in map and
// protected by a [sync.RWMutex]. Reads use an RLock; writes take the write
// lock. Zero value is not usable; construct with [NewHashTable].
type HashTable struct {
	mutex sync.RWMutex
	data  map[string]string
}

// NewHashTable returns an empty HashTable ready for use.
func NewHashTable() *HashTable {
	return &HashTable{
		data: make(map[string]string),
	}
}

// Set stores value under key, overwriting any existing entry.
func (h *HashTable) Set(key, value string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.data[key] = value
}

// Get returns the value stored under key and a boolean indicating whether
// the key was present.
func (h *HashTable) Get(key string) (string, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	result, ok := h.data[key]
	if !ok {
		return "", ok
	}
	return result, ok
}

// Del removes key from the table. Deleting an absent key is a no-op.
func (h *HashTable) Del(key string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	delete(h.data, key)
}
