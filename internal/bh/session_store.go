// Package bh session persistence: stores sessions as JSON files in DataDir.
package bh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Session represents a persistent conversation session.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

// SessionStore persists sessions as JSON files in DataDir.
type SessionStore struct {
	mu  syncMutex
	dir string
}

// syncMutex is a simple mutex for the session store.
type syncMutex struct {
	mu sync.Mutex
}

func (m *syncMutex) Lock() {
	m.mu.Lock()
}

func (m *syncMutex) Unlock() {
	m.mu.Unlock()
}

// NewSessionStore opens a persistent session store at dir.
func NewSessionStore(dir string) *SessionStore {
	os.MkdirAll(dir, 0755)
	return &SessionStore{dir: dir}
}

// Create creates a new session with the given title.
func (s *SessionStore) Create(title string) *Session {
	now := time.Now()
	id := genID()
	ss := &Session{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	s.save(ss)
	return ss
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(id string) (*Session, error) {
	file := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	ss := &Session{}
	if err := json.Unmarshal(data, ss); err != nil {
		return nil, err
	}
	return ss, nil
}

// Update updates an existing session.
func (s *SessionStore) Update(ss *Session) error {
	s.save(ss)
	return nil
}

// List returns all sessions, sorted by most recently updated.
func (s *SessionStore) List() ([]*Session, error) {
	files, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []*Session
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		ss, err := s.Get(f.Name()[:len(f.Name())-5])
		if err != nil {
			continue
		}
		sessions = append(sessions, ss)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Save saves a session to disk.
func (s *SessionStore) save(ss *Session) {
	data, err := json.MarshalIndent(ss, "", "\t")
	if err != nil {
		return
	}
	file := filepath.Join(s.dir, ss.ID+".json")
	os.WriteFile(file, data, 0644)
}

// Delete removes a session from disk.
func (s *SessionStore) Delete(id string) error {
	file := filepath.Join(s.dir, id+".json")
	return os.Remove(file)
}
