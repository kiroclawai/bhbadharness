// Package bh persistent short-term memory across turns (e.g. user preferences,
// known facts, prior context). Stored as one JSON file per key in DataDir.
package bh

import (
	"os"
	"path/filepath"
	"sync"
)

// MemoryEntry is a single memory record.
type MemoryEntry struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
}

// MemoryStore persists short-term memory across turns as one JSON file per key.
type MemoryStore struct {
	mu     sync.Mutex
	dir    string
	store  map[string]string
	loaded bool
}

// NewMemoryStore opens a persistent memory store at dir.
func NewMemoryStore(dir string) *MemoryStore {
	m := &MemoryStore{dir: dir, store: map[string]string{}}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return m
	}
	m.load()
	return m
}

func (m *MemoryStore) load() {
	if m.loaded {
		return
	}
	m.loaded = true
	files, err := os.ReadDir(m.dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, f.Name()))
		if err != nil {
			continue
		}
		m.store[f.Name()] = string(data)
	}
}

// Set stores a memory entry as JSON. If value is a string, it is stored as-is.
func (m *MemoryStore) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	m.persist(key, value)
}

func (m *MemoryStore) persist(key, value string) {
	path := filepath.Join(m.dir, key+".json")
	_ = os.MkdirAll(m.dir, 0755)
	_ = os.WriteFile(path, []byte(value), 0644)
}

// Get returns a memory entry's value, or "" if absent.
func (m *MemoryStore) Get(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store[key]
}

// Delete removes a memory entry.
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	m.persist(key, "")
}
