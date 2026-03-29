package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ai-agent-gigachat/internal/logging"
	"ai-agent-gigachat/internal/storage"
	"ai-agent-gigachat/internal/storage/memory"
)

// HistoryConfig holds configuration for history compression.
type HistoryConfig struct {
	MaxMessages                 int    // HISTORY_MAX_MESSAGES
	SummaryPrompt               string // content of prompt file
	SlidingWindowSize           int    // SLIDING_WINDOW_SIZE
	StickyFactsWindowSize       int    // STICKY_FACTS_WINDOW_SIZE
	StickyFactsExtractionPrompt string // content of prompt file for fact extraction
}

// loadHistoryConfig reads environment variables and returns a HistoryConfig.
func loadHistoryConfig() HistoryConfig {
	maxMessages := 0
	if s := os.Getenv("HISTORY_MAX_MESSAGES"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			maxMessages = v
		}
	}
	prompt := defaultSummaryPrompt
	if path := os.Getenv("HISTORY_SUMMARY_PROMPT_FILE"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			prompt = strings.TrimSpace(string(data))
		}
		// If file reading fails, keep default (no error logging for now)
	}
	slidingWindowSize := 10 // default
	if s := os.Getenv("SLIDING_WINDOW_SIZE"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			slidingWindowSize = v
		}
	}
	stickyFactsWindowSize := 10 // default
	if s := os.Getenv("STICKY_FACTS_WINDOW_SIZE"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			stickyFactsWindowSize = v
		}
	}
	stickyFactsPrompt := defaultStickyFactsPrompt
	if path := os.Getenv("STICKY_FACTS_EXTRACTION_PROMPT_FILE"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			stickyFactsPrompt = strings.TrimSpace(string(data))
		}
		// If file reading fails, keep default (no error logging for now)
	}
	return HistoryConfig{
		MaxMessages:                 maxMessages,
		SummaryPrompt:               prompt,
		SlidingWindowSize:           slidingWindowSize,
		StickyFactsWindowSize:       stickyFactsWindowSize,
		StickyFactsExtractionPrompt: stickyFactsPrompt,
	}
}

const defaultSummaryPrompt = "Summarize the following conversation concisely, preserving key points and decisions:"
const defaultStickyFactsPrompt = "Extract key facts from the conversation as JSON key-value pairs. Output only valid JSON."

// Agent is the main AI agent that manages sessions and communicates with GigaChat.
type Agent struct {
	client        *GigaChatClient
	storage       storage.Storage
	logger        *logging.Logger
	historyConfig HistoryConfig
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
		client:        NewGigaChatClient(apiKey),
		storage:       store,
		logger:        logging.Default(),
		historyConfig: loadHistoryConfig(),
	}
}

// toAgentMessage converts a storage.Message to an agent.Message.
func toAgentMessage(m storage.Message) Message {
	return Message{
		Role:             m.Role,
		Content:          m.Content,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
	}
}

// toStorageMessage converts an agent.Message to a storage.Message.
func toStorageMessage(m Message) storage.Message {
	return storage.Message{
		Role:             m.Role,
		Content:          m.Content,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
	}
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
	return &Session{ID: s.ID, History: history, Strategy: s.Strategy}
}

// summarizeMessages sends the given messages to GigaChat with a summarization prompt and returns the summary text.
func (a *Agent) summarizeMessages(messages []Message) (string, error) {
	// Build request: original messages plus a user message with the summarization prompt
	summaryPrompt := a.historyConfig.SummaryPrompt
	requestMessages := append(messages, Message{Role: "user", Content: summaryPrompt})
	result, err := a.client.SendMessage(requestMessages)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// extractFacts sends the given messages to GigaChat with a fact extraction prompt and returns a map of facts.
func (a *Agent) extractFacts(messages []Message) (map[string]string, error) {
	extractionPrompt := a.historyConfig.StickyFactsExtractionPrompt
	requestMessages := append(messages, Message{Role: "user", Content: extractionPrompt})
	result, err := a.client.SendMessage(requestMessages)
	if err != nil {
		return nil, err
	}
	// Parse JSON response
	var facts map[string]string
	if err := json.Unmarshal([]byte(result.Content), &facts); err != nil {
		// If parsing fails, return empty map and log error
		a.logger.LogError(err, "failed to parse facts JSON", "response", result.Content)
		return make(map[string]string), nil
	}
	return facts, nil
}

// SendMessage processes a user message for a given session ID and returns the assistant's response and token usage.
func (a *Agent) SendMessage(sessionID, userMessage string) (*CompletionResult, error) {
	// Ensure session exists
	err := a.storage.CreateSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Add user message to storage (no token counts yet)
	userMsg := Message{Role: "user", Content: userMessage}
	if err := a.storage.AddMessage(sessionID, toStorageMessage(userMsg)); err != nil {
		return nil, err
	}

	// Retrieve the full conversation history (including the newly added message)
	storageSession, err := a.storage.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if storageSession == nil {
		// Should not happen because we just created the session
		return nil, ErrSessionNotFound
	}

	// Convert history to agent messages
	history := make([]Message, len(storageSession.History))
	for i, m := range storageSession.History {
		history[i] = toAgentMessage(m)
	}

	// Determine session strategy (default to summary)
	strategy := storageSession.Strategy
	if strategy == "" {
		strategy = storage.StrategySummary
	}

	// Apply history compression based on strategy
	switch strategy {
	case storage.StrategySummary:
		if a.historyConfig.MaxMessages > 0 && len(history) > a.historyConfig.MaxMessages {
			// Keep the last message (the user's current message) as is
			if len(history) < 2 {
				// Should not happen because we have at least the user message and maybe earlier messages
				a.logger.LogError(nil, "history length too short for compression", "session_id", sessionID)
			} else {
				lastMessage := history[len(history)-1]
				olderMessages := history[:len(history)-1]

				// Summarize older messages
				summaryText, err := a.summarizeMessages(olderMessages)
				if err != nil {
					a.logger.LogError(err, "failed to summarize history, proceeding with original history")
				} else {
					// Build new history: combined summary (as system message) and last message
					combinedContent := summaryText + "\n\n(History has been summarized to reduce length.)"
					summaryMsg := Message{Role: "system", Content: combinedContent}
					newHistory := []Message{summaryMsg, lastMessage}

					// Convert to storage messages and replace session history
					storageMessages := make([]storage.Message, len(newHistory))
					for i, msg := range newHistory {
						storageMessages[i] = toStorageMessage(msg)
					}
					if err := a.storage.ReplaceHistory(sessionID, storageMessages); err != nil {
						a.logger.LogError(err, "failed to replace history after summarization")
					} else {
						// Update local history for the upcoming request
						history = newHistory
					}
				}
			}
		}
	case storage.StrategySlidingWindow:
		windowSize := a.historyConfig.SlidingWindowSize
		if windowSize > 0 && len(history) > windowSize {
			// Keep only the last windowSize messages
			truncated := history[len(history)-windowSize:]
			// Convert to storage messages and replace session history
			storageMessages := make([]storage.Message, len(truncated))
			for i, msg := range truncated {
				storageMessages[i] = toStorageMessage(msg)
			}
			if err := a.storage.ReplaceHistory(sessionID, storageMessages); err != nil {
				a.logger.LogError(err, "failed to replace history after sliding window truncation")
			} else {
				// Update local history for the upcoming request
				history = truncated
			}
		}
	case storage.StrategyStickyFacts:
		windowSize := a.historyConfig.StickyFactsWindowSize
		if windowSize > 0 && len(history) > windowSize {
			// Keep only the last windowSize messages
			truncated := history[len(history)-windowSize:]
			// Convert to storage messages and replace session history
			storageMessages := make([]storage.Message, len(truncated))
			for i, msg := range truncated {
				storageMessages[i] = toStorageMessage(msg)
			}
			if err := a.storage.ReplaceHistory(sessionID, storageMessages); err != nil {
				a.logger.LogError(err, "failed to replace history after sticky facts window truncation")
			} else {
				// Update local history for the upcoming request
				history = truncated
			}
		}
		// Retrieve facts from session
		facts := storageSession.Facts
		if facts == nil {
			facts = make(map[string]string)
		}
		// Build system message with facts
		if len(facts) > 0 {
			factsContent := "Facts:\n"
			for k, v := range facts {
				factsContent += fmt.Sprintf("- %s: %s\n", k, v)
			}
			factsMsg := Message{Role: "system", Content: strings.TrimSpace(factsContent)}
			// Prepend facts message to history for the request
			history = append([]Message{factsMsg}, history...)
		}
	default:
		a.logger.LogError(nil, "unknown strategy, using default (summary)", "strategy", strategy)
	}

	// Send the (possibly compressed) history to GigaChat
	result, err := a.client.SendMessage(history)
	if err != nil {
		return nil, err
	}

	// Add assistant response to storage with token counts
	assistantMsg := Message{
		Role:             "assistant",
		Content:          result.Content,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
		TotalTokens:      result.Usage.TotalTokens,
	}
	if err := a.storage.AddMessage(sessionID, toStorageMessage(assistantMsg)); err != nil {
		// We still return the response, but log the error
		a.logger.LogError(err, "failed to store assistant message")
	}

	// If strategy is sticky_facts, update facts after the new message
	if strategy == storage.StrategyStickyFacts {
		// Retrieve updated session history (including the newly added assistant message)
		updatedSession, err := a.storage.GetSession(sessionID)
		if err != nil {
			a.logger.LogError(err, "failed to retrieve session for fact extraction")
		} else if updatedSession != nil {
			// Convert history to agent messages
			updatedHistory := make([]Message, len(updatedSession.History))
			for i, m := range updatedSession.History {
				updatedHistory[i] = toAgentMessage(m)
			}
			// Extract facts from the updated conversation
			newFacts, err := a.extractFacts(updatedHistory)
			if err != nil {
				a.logger.LogError(err, "failed to extract facts")
			} else {
				// Update facts in storage
				if err := a.storage.UpdateFacts(sessionID, newFacts); err != nil {
					a.logger.LogError(err, "failed to update session facts")
				}
			}
		}
	}

	return result, nil
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