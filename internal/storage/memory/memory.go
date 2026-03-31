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
	profiles map[string]*storage.Profile
}

// New creates a new MemoryStorage.
func New() *MemoryStorage {
	ms := &MemoryStorage{
		sessions: make(map[string]*storage.Session),
		profiles: make(map[string]*storage.Profile),
	}
	// Add a default profile
	defaultProfile := &storage.Profile{
		ID:          "00000000-0000-0000-0000-000000000000",
		Name:        "Default",
		Style:       "Respond in a helpful, friendly, and professional manner.",
		Constraints: "Be accurate, concise, and avoid harmful content.",
		Context:     "You are an AI assistant helping users with their questions.",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsDefault:   true,
	}
	ms.profiles[defaultProfile.ID] = defaultProfile
	return ms
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
	facts := session.Facts // string copy
	return &storage.Session{
		ID:        id,
		History:   history,
		Strategy:  session.Strategy,
		Facts:     facts,
		ProfileID: session.ProfileID,
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
			Strategy:  storage.StrategySummary,
			Facts:     "",
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

// UpdateStrategy updates the context management strategy for a session.
func (m *MemoryStorage) UpdateStrategy(sessionID string, strategy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionID]
	if !exists {
		return storage.ErrSessionNotFound
	}
	session.Strategy = strategy
	session.UpdatedAt = time.Now()
	return nil
}

// UpdateFacts updates the facts text for a session.
func (m *MemoryStorage) UpdateFacts(sessionID string, facts string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionID]
	if !exists {
		return storage.ErrSessionNotFound
	}
	session.Facts = facts
	session.UpdatedAt = time.Now()
	return nil
}

// UpdateSessionProfile updates the profile associated with a session.
func (m *MemoryStorage) UpdateSessionProfile(sessionID string, profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, exists := m.sessions[sessionID]
	if !exists {
		return storage.ErrSessionNotFound
	}
	session.ProfileID = profileID
	session.UpdatedAt = time.Now()
	return nil
}

// ListProfiles returns all profiles.
func (m *MemoryStorage) ListProfiles() ([]storage.Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profiles := make([]storage.Profile, 0, len(m.profiles))
	for _, p := range m.profiles {
		profiles = append(profiles, *p)
	}
	return profiles, nil
}

// GetProfile retrieves a profile by ID.
func (m *MemoryStorage) GetProfile(id string) (*storage.Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, exists := m.profiles[id]
	if !exists {
		return nil, storage.ErrProfileNotFound
	}
	// Return a copy
	copyProfile := *p
	return &copyProfile, nil
}

// CreateProfile creates a new profile.
func (m *MemoryStorage) CreateProfile(profile storage.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.profiles[profile.ID]; exists {
		// Profile already exists
		return nil
	}
	// Create a copy
	p := profile
	m.profiles[profile.ID] = &p
	return nil
}

// UpdateProfile updates an existing profile.
func (m *MemoryStorage) UpdateProfile(id string, profile storage.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.profiles[id]; !exists {
		return storage.ErrProfileNotFound
	}
	// Update with new values
	p := profile
	p.ID = id // Ensure ID doesn't change
	m.profiles[id] = &p
	return nil
}

// DeleteProfile deletes a profile.
func (m *MemoryStorage) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if profile is in use
	for _, session := range m.sessions {
		if session.ProfileID == id {
			return storage.ErrProfileInUse
		}
	}

	delete(m.profiles, id)
	return nil
}

// SetDefaultProfile sets a profile as default and unsets any other default.
func (m *MemoryStorage) SetDefaultProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if profile exists
	p, exists := m.profiles[id]
	if !exists {
		return storage.ErrProfileNotFound
	}

	// Unset all defaults
	for _, profile := range m.profiles {
		profile.IsDefault = false
	}

	// Set new default
	p.IsDefault = true
	return nil
}

// GetDefaultProfile returns the default profile, if any.
func (m *MemoryStorage) GetDefaultProfile() (*storage.Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.profiles {
		if p.IsDefault {
			// Return a copy
			copyProfile := *p
			return &copyProfile, nil
		}
	}
	return nil, nil // No default profile
}

// Close releases any resources held by the storage (no‑op for memory).
func (m *MemoryStorage) Close() error {
	return nil
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = storage.ErrSessionNotFound
