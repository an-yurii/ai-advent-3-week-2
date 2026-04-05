package storage

import (
	"errors"
	"time"
)

// Message represents a single message in a conversation.
type Message struct {
	Role             string `json:"role"` // "user" or "assistant"
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`     // tokens used in the request (for assistant messages)
	CompletionTokens int    `json:"completion_tokens,omitempty"` // tokens used in the response (for assistant messages)
	TotalTokens      int    `json:"total_tokens,omitempty"`      // total tokens (prompt + completion)
}

// Strategy constants
const (
	StrategySummary       = "summary"
	StrategySlidingWindow = "sliding_window"
	StrategyStickyFacts   = "sticky_facts"
)

// Profile holds user profile configuration for AI interactions.
type Profile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Style       string    `json:"style"`
	Constraints string    `json:"constraints"`
	Context     string    `json:"context"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsDefault   bool      `json:"is_default"`
}

// TaskContext holds the finite state machine context for a session.
type TaskContext struct {
	State    string                 `json:"state"`
	Task     string                 `json:"task"` // First user message
	Done     bool                   `json:"done"`
	Metadata map[string]interface{} `json:"metadata"` // step_number, transition_history, etc.
}

// Session holds the conversation history for a single user session.
type Session struct {
	ID          string       `json:"id"`
	History     []Message    `json:"history"`
	Strategy    string       `json:"strategy"`               // one of StrategySummary, StrategySlidingWindow, StrategyStickyFacts
	Facts       string       `json:"facts"`                  // plain text facts extracted from conversation
	ProfileID   string       `json:"profile_id"`             // optional profile ID
	TaskContext *TaskContext `json:"task_context,omitempty"` // optional FSM context
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
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

	// ReplaceHistory replaces the entire message history of a session with the given messages.
	// The session must exist; if not, ErrSessionNotFound is returned.
	ReplaceHistory(sessionID string, messages []Message) error

	// UpdateStrategy updates the context management strategy for a session.
	// The session must exist; if not, ErrSessionNotFound is returned.
	UpdateStrategy(sessionID string, strategy string) error

	// UpdateFacts updates the facts text for a session.
	// The session must exist; if not, ErrSessionNotFound is returned.
	UpdateFacts(sessionID string, facts string) error

	// UpdateSessionProfile updates the profile associated with a session.
	// The session must exist; if not, ErrSessionNotFound is returned.
	UpdateSessionProfile(sessionID string, profileID string) error

	// Profile management methods
	ListProfiles() ([]Profile, error)
	GetProfile(id string) (*Profile, error)
	CreateProfile(profile Profile) error
	UpdateProfile(id string, profile Profile) error
	DeleteProfile(id string) error
	SetDefaultProfile(id string) error
	GetDefaultProfile() (*Profile, error)

	// TaskContext management methods
	UpdateTaskContext(sessionID string, context *TaskContext) error
	GetTaskContext(sessionID string) (*TaskContext, error)

	// Close releases any resources held by the storage.
	Close() error
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// ErrProfileNotFound is returned when a profile does not exist.
var ErrProfileNotFound = errors.New("profile not found")

// ErrProfileInUse is returned when trying to delete a profile that is in use by sessions.
var ErrProfileInUse = errors.New("profile is in use by sessions")
