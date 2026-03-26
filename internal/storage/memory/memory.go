package memory

import (
	"sync"
	"time"

	"ai-agent-gigachat/internal/storage"
)

// MemoryStorage implements storage.Storage using an in‑memory map.
type MemoryStorage struct {
	mu       sync.RWMutex
	sessions map[string]*storage.Session
}

// New creates a new MemoryStorage.
func New() *MemoryStorage {
	return &MemoryStorage{
		sessions: make(map[string]*storage.Session),
	}
}

// GetSession retrieves a session by ID, including its message history.
func (m *MemoryStorage) GetSession(id string) (*storage.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[id]
	if !exists {
		return nil, nil
	}
	// Return a copy to avoid accidental mutation
	history := make([]storage.Message, len(session.History))
	copy(history, session.History)
	return &storage.Session{
		ID:        id,
		History:   history,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
	}, nil
}

// CreateSession creates a new empty session with the given ID.
func (m *MemoryStorage) CreateSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sessions[id]; !exists {
		now := time.Now()
		m.sessions[id] = &storage.Session{
			ID:        id,
			History:   []storage.Message{},
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	return nil
}

// AddMessage adds a message to the session's history.
func (m *MemoryStorage) AddMessage(sessionID string, msg storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionID]
	if !exists {
		return storage.ErrSessionNotFound
	}
	session.History = append(session.History, msg)
	session.UpdatedAt = time.Now()
	return nil
}

// DeleteSession deletes a session and all its messages.
func (m *MemoryStorage) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

// ListSessions returns a list of all session IDs, ordered by creation time (newest first).
// Since we don't track creation time, we return them in arbitrary order.
func (m *MemoryStorage) ListSessions() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}

// ReplaceHistory replaces the entire message history of a session with the given messages.
func (m *MemoryStorage) ReplaceHistory(sessionID string, messages []storage.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionID]
	if !exists {
		return storage.ErrSessionNotFound
	}
	// Create a copy of the slice to avoid external modifications
	newHistory := make([]storage.Message, len(messages))
	copy(newHistory, messages)
	session.History = newHistory
	session.UpdatedAt = time.Now()
	return nil
}

// Close releases any resources held by the storage (no‑op for memory).
func (m *MemoryStorage) Close() error {
	return nil
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = storage.ErrSessionNotFound