package agent

import (
	"testing"

	"ai-agent-gigachat/internal/storage/memory"
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
	store := memory.New()
	agent := NewAgent("dummy-key", store)
	sessionID := "session-1"

	// New session should be created
	response, err := agent.SendMessage(sessionID, "Hello")
	// Expect error because API key is dummy and HTTP client will fail
	// That's fine, we just test that session is created
	session, err2 := agent.GetSession(sessionID)
	if err2 != nil {
		t.Fatalf("GetSession error: %v", err2)
	}
	if session == nil {
		t.Error("Session should exist after SendMessage")
	}

	// Clear session
	agent.ClearSession(sessionID)
	session, _ = agent.GetSession(sessionID)
	if session != nil {
		t.Error("Session should be cleared")
	}

	// Clear all sessions
	agent.SendMessage("s1", "msg1")
	agent.SendMessage("s2", "msg2")
	agent.ClearAllSessions()
	s1, _ := agent.GetSession("s1")
	s2, _ := agent.GetSession("s2")
	if s1 != nil || s2 != nil {
		t.Error("All sessions should be cleared")
	}

	_ = response
	_ = err
}