package storage

import "errors"

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// Session holds the conversation history for a single user session.
type Session struct {
	ID      string
	History []Message
}

// Storage defines the interface for persisting sessions and messages.
type Storage interface {
	// GetSession retrieves a session by ID, including its message history.
	// If the session does not exist, returns nil, nil.
	GetSession(id string) (*Session, error)

	// CreateSession creates a new empty session with the given ID.
	// If a session with that ID already exists, it is a no-op.
	CreateSession(id string) error

	// AddMessage adds a message to the session's history.
	// The session must exist; if not, an error is returned.
	AddMessage(sessionID string, msg Message) error

	// DeleteSession deletes a session and all its messages.
	DeleteSession(id string) error

	// ListSessions returns a list of all session IDs, ordered by creation time (newest first).
	ListSessions() ([]string, error)

	// Close releases any resources held by the storage.
	Close() error
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")