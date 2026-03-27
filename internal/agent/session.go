package agent

// Message represents a single message in the conversation.
type Message struct {
	Role             string `json:"role"` // "user" or "assistant"
	Content          string `json:"content"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`     // tokens used in the request (for assistant messages)
	CompletionTokens int    `json:"completion_tokens,omitempty"` // tokens used in the response (for assistant messages)
	TotalTokens      int    `json:"total_tokens,omitempty"`      // total tokens (prompt + completion)
}

// Session holds the conversation history for a single user session.
type Session struct {
	ID      string    `json:"id"`
	History []Message `json:"history"`
}

// NewSession creates a new session with the given ID.
func NewSession(id string) *Session {
	return &Session{
		ID:      id,
		History: []Message{},
	}
}

// AddUserMessage adds a user message to the session history.
func (s *Session) AddUserMessage(content string) {
	s.History = append(s.History, Message{Role: "user", Content: content})
}

// AddAssistantMessage adds an assistant message to the session history.
func (s *Session) AddAssistantMessage(content string) {
	s.History = append(s.History, Message{Role: "assistant", Content: content})
}

// ClearHistory clears the session history.
func (s *Session) ClearHistory() {
	s.History = []Message{}
}