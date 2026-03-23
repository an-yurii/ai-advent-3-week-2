package agent

import (
	"testing"
)

func TestSessionAddMessages(t *testing.T) {
	s := NewSession("test-session")
	if len(s.History) != 0 {
		t.Errorf("Expected empty history, got %d", len(s.History))
	}

	s.AddUserMessage("Hello")
	if len(s.History) != 1 {
		t.Errorf("Expected 1 message, got %d", len(s.History))
	}
	if s.History[0].Role != "user" || s.History[0].Content != "Hello" {
		t.Errorf("Unexpected message: %+v", s.History[0])
	}

	s.AddAssistantMessage("Hi there")
	if len(s.History) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(s.History))
	}
	if s.History[1].Role != "assistant" || s.History[1].Content != "Hi there" {
		t.Errorf("Unexpected message: %+v", s.History[1])
	}

	s.ClearHistory()
	if len(s.History) != 0 {
		t.Errorf("Expected cleared history, got %d", len(s.History))
	}
}

func TestAgentSessionManagement(t *testing.T) {
	agent := NewAgent("dummy-key")
	sessionID := "session-1"

	// New session should be created
	response, err := agent.SendMessage(sessionID, "Hello")
	// Expect error because API key is dummy and HTTP client will fail
	// That's fine, we just test that session is created
	if agent.GetSession(sessionID) == nil {
		t.Error("Session should exist after SendMessage")
	}

	// Clear session
	agent.ClearSession(sessionID)
	if agent.GetSession(sessionID) != nil {
		t.Error("Session should be cleared")
	}

	// Clear all sessions
	agent.SendMessage("s1", "msg1")
	agent.SendMessage("s2", "msg2")
	agent.ClearAllSessions()
	if agent.GetSession("s1") != nil || agent.GetSession("s2") != nil {
		t.Error("All sessions should be cleared")
	}

	_ = response
	_ = err
}