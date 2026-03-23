package agent

import (
	"sync"

	"ai-agent-gigachat/internal/logging"
)

// Agent is the main AI agent that manages sessions and communicates with GigaChat.
type Agent struct {
	client   *GigaChatClient
	sessions map[string]*Session
	mu       sync.RWMutex
	logger   *logging.Logger
}

// NewAgent creates a new Agent with the given API key.
func NewAgent(apiKey string) *Agent {
	return &Agent{
		client:   NewGigaChatClient(apiKey),
		sessions: make(map[string]*Session),
		logger:   logging.Default(),
	}
}

// SendMessage processes a user message for a given session ID and returns the assistant's response.
func (a *Agent) SendMessage(sessionID, userMessage string) (string, error) {
	a.mu.Lock()
	session, exists := a.sessions[sessionID]
	if !exists {
		session = NewSession(sessionID)
		a.sessions[sessionID] = session
	}
	a.mu.Unlock()

	// Add user message to session history
	session.AddUserMessage(userMessage)

	// Send the whole history to GigaChat
	a.mu.RLock()
	messages := make([]Message, len(session.History))
	copy(messages, session.History)
	a.mu.RUnlock()

	assistantResponse, err := a.client.SendMessage(messages)
	if err != nil {
		return "", err
	}

	// Add assistant response to session history
	session.AddAssistantMessage(assistantResponse)

	return assistantResponse, nil
}

// GetSession returns the session for the given ID, or nil if not found.
func (a *Agent) GetSession(sessionID string) *Session {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessions[sessionID]
}

// ClearSession removes the session for the given ID.
func (a *Agent) ClearSession(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
}

// ClearAllSessions removes all sessions.
func (a *Agent) ClearAllSessions() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions = make(map[string]*Session)
}