package agent

import (
	"errors"

	"ai-agent-gigachat/internal/logging"
	"ai-agent-gigachat/internal/storage"
	"ai-agent-gigachat/internal/storage/memory"
)

// Agent is the main AI agent that manages sessions and communicates with GigaChat.
type Agent struct {
	client  *GigaChatClient
	storage storage.Storage
	logger  *logging.Logger
}

// NewAgent creates a new Agent with the given API key and optional storage.
// If storage is not provided, an in‑memory storage is used.
func NewAgent(apiKey string, storageOpt ...storage.Storage) *Agent {
	var store storage.Storage
	if len(storageOpt) > 0 {
		store = storageOpt[0]
	} else {
		store = memory.New()
	}
	return &Agent{
		client:  NewGigaChatClient(apiKey),
		storage: store,
		logger:  logging.Default(),
	}
}

// toAgentMessage converts a storage.Message to an agent.Message.
func toAgentMessage(m storage.Message) Message {
	return Message{Role: m.Role, Content: m.Content}
}

// toStorageMessage converts an agent.Message to a storage.Message.
func toStorageMessage(m Message) storage.Message {
	return storage.Message{Role: m.Role, Content: m.Content}
}

// toAgentSession converts a storage.Session to an agent.Session.
func toAgentSession(s *storage.Session) *Session {
	if s == nil {
		return nil
	}
	history := make([]Message, len(s.History))
	for i, m := range s.History {
		history[i] = toAgentMessage(m)
	}
	return &Session{ID: s.ID, History: history}
}

// SendMessage processes a user message for a given session ID and returns the assistant's response.
func (a *Agent) SendMessage(sessionID, userMessage string) (string, error) {
	// Ensure session exists
	err := a.storage.CreateSession(sessionID)
	if err != nil {
		return "", err
	}

	// Add user message to storage
	userMsg := Message{Role: "user", Content: userMessage}
	if err := a.storage.AddMessage(sessionID, toStorageMessage(userMsg)); err != nil {
		return "", err
	}

	// Retrieve the full conversation history (including the newly added message)
	storageSession, err := a.storage.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	if storageSession == nil {
		// Should not happen because we just created the session
		return "", ErrSessionNotFound
	}

	// Convert history to agent messages
	history := make([]Message, len(storageSession.History))
	for i, m := range storageSession.History {
		history[i] = toAgentMessage(m)
	}

	// Send the whole history to GigaChat
	assistantResponse, err := a.client.SendMessage(history)
	if err != nil {
		return "", err
	}

	// Add assistant response to storage
	assistantMsg := Message{Role: "assistant", Content: assistantResponse}
	if err := a.storage.AddMessage(sessionID, toStorageMessage(assistantMsg)); err != nil {
		// We still return the response, but log the error
		a.logger.LogError(err, "failed to store assistant message")
	}

	return assistantResponse, nil
}

// GetSession returns the session for the given ID, or nil if not found.
func (a *Agent) GetSession(sessionID string) (*Session, error) {
	storageSession, err := a.storage.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return toAgentSession(storageSession), nil
}

// ClearSession removes the session for the given ID.
func (a *Agent) ClearSession(sessionID string) error {
	return a.storage.DeleteSession(sessionID)
}

// ClearAllSessions removes all sessions.
func (a *Agent) ClearAllSessions() error {
	sessionIDs, err := a.storage.ListSessions()
	if err != nil {
		return err
	}
	for _, id := range sessionIDs {
		if err := a.storage.DeleteSession(id); err != nil {
			// Log error but continue deleting others
			a.logger.LogError(err, "failed to delete session", "session_id", id)
		}
	}
	return nil
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")