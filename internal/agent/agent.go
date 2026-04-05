package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ai-agent-gigachat/internal/agent/fsm"
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
const defaultStickyFactsPrompt = "Extract key facts from the conversation as plain text. Output each fact on a new line."

// Agent is the main AI agent that manages sessions and communicates with GigaChat.
type Agent struct {
	client        *GigaChatClient
	storage       storage.Storage
	logger        *logging.Logger
	historyConfig HistoryConfig
	fsm           *fsm.FSM // optional FSM for task state management
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

	// Try to load FSM config if available
	var fsmInstance *fsm.FSM
	if configPath := os.Getenv("FSM_CONFIG_PATH"); configPath != "" {
		fsmInstance, _ = fsm.NewFSM(configPath, store)
		// Log warning if FSM fails to load, but don't fail agent creation
		if fsmInstance == nil {
			logging.Default().Warn("Failed to load FSM config", "path", configPath)
		} else {
			logging.Default().Info("FSM loaded successfully", "path", configPath)
		}
	}

	return &Agent{
		client:        NewGigaChatClient(apiKey),
		storage:       store,
		logger:        logging.Default(),
		historyConfig: loadHistoryConfig(),
		fsm:           fsmInstance,
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
	return &Session{ID: s.ID, History: history, Strategy: s.Strategy, ProfileID: s.ProfileID}
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

// cleanJSONResponse removes Markdown code fences and extra whitespace from a JSON string.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	// Remove leading ``` and optional language
	if strings.HasPrefix(s, "```") {
		// Find the first newline after the backticks
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		} else {
			// No newline, remove the backticks entirely
			s = strings.TrimPrefix(s, "```")
		}
		// Remove trailing ```
		s = strings.TrimSuffix(s, "```")
	}
	// Also remove any trailing ``` that may be on its own line
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
		s = strings.Join(lines, "\n")
	}
	s = strings.TrimSpace(s)
	return s
}

// extractFacts sends the given messages to GigaChat with a fact extraction prompt and returns facts as plain text.
func (a *Agent) extractFacts(messages []Message) (string, error) {
	extractionPrompt := a.historyConfig.StickyFactsExtractionPrompt
	requestMessages := append(messages, Message{Role: "user", Content: extractionPrompt})
	result, err := a.client.SendMessage(requestMessages)
	if err != nil {
		return "", err
	}
	// Clean the response (remove markdown code fences, extra whitespace)
	cleaned := cleanJSONResponse(result.Content)
	return cleaned, nil
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

	// Initialize FSM if this is the first user message and FSM is available
	if a.fsm != nil {
		// Check if this is the first user message in the session
		storageSession, err := a.storage.GetSession(sessionID)
		if err != nil {
			a.logger.LogError(err, "failed to get session for FSM initialization")
		} else if storageSession != nil && len(storageSession.History) == 1 {
			// This is the first message (we just added it)
			if err := a.fsm.InitializeSession(sessionID, userMessage); err != nil {
				a.logger.LogError(err, "failed to initialize FSM")
			}
		}
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
		// Build system message with facts if not empty
		if facts != "" {
			factsContent := "Facts:\n" + facts
			factsMsg := Message{Role: "system", Content: strings.TrimSpace(factsContent)}
			// Prepend facts message to history for the request
			history = append([]Message{factsMsg}, history...)
		}
	default:
		a.logger.LogError(nil, "unknown strategy, using default (summary)", "strategy", strategy)
	}

	// Add profile context if session has a profile
	if storageSession.ProfileID != "" {
		profile, err := a.storage.GetProfile(storageSession.ProfileID)
		if err != nil {
			a.logger.LogError(err, "failed to get profile", "profile_id", storageSession.ProfileID)
		} else if profile != nil {
			// Build profile context message
			var contextBuilder strings.Builder
			contextBuilder.WriteString("You are using the following profile:\n")
			contextBuilder.WriteString("Name: " + profile.Name + "\n")

			if profile.Style != "" {
				contextBuilder.WriteString("\nStyle: " + profile.Style + "\n")
			}
			if profile.Constraints != "" {
				contextBuilder.WriteString("\nConstraints: " + profile.Constraints + "\n")
			}
			if profile.Context != "" {
				contextBuilder.WriteString("\nContext: " + profile.Context + "\n")
			}

			profileMsg := Message{
				Role:    "system",
				Content: contextBuilder.String(),
			}
			// Prepend profile message to history
			history = append([]Message{profileMsg}, history...)
		}
	}

	// Add FSM context if available
	fsmContextMsg, err := a.buildFSMContext(sessionID)
	if err != nil {
		a.logger.LogError(err, "failed to build FSM context")
	} else if fsmContextMsg != nil {
		// Prepend FSM context message to history
		history = append([]Message{*fsmContextMsg}, history...)
	}

	// Merge consecutive system messages at the beginning to avoid GigaChat API error
	history = mergeSystemMessages(history)

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

	// Process FSM transition if FSM is available
	var taskContext *storage.TaskContext
	if a.fsm != nil {
		ctx, err := a.fsm.ProcessResponse(sessionID, result.Content)
		if err != nil {
			a.logger.LogError(err, "FSM processing failed")
		} else {
			taskContext = ctx
		}
	}

	// Trigger automatic LLM request if FSM transition occurred and task is not done
	if taskContext != nil && !taskContext.Done {
		// Check if we should make an automatic request
		if a.shouldMakeAutomaticRequest(sessionID, taskContext) {
			go a.makeAutomaticRequest(sessionID, taskContext)
		}
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

// CopySession creates a copy of an existing session with a new ID.
// Returns the new session ID, or an error if the source session does not exist.
func (a *Agent) CopySession(sourceID string, newID string) error {
	// Retrieve source session
	source, err := a.storage.GetSession(sourceID)
	if err != nil {
		return err
	}
	if source == nil {
		return ErrSessionNotFound
	}
	// Create new session
	if err := a.storage.CreateSession(newID); err != nil {
		return err
	}
	// Copy history if any
	if len(source.History) > 0 {
		// Deep copy of messages
		messages := make([]storage.Message, len(source.History))
		copy(messages, source.History)
		if err := a.storage.ReplaceHistory(newID, messages); err != nil {
			// If replace fails, we should delete the newly created session? For simplicity, just return error.
			return err
		}
	}
	// Copy strategy
	if source.Strategy != "" {
		if err := a.storage.UpdateStrategy(newID, source.Strategy); err != nil {
			return err
		}
	}
	// Copy facts
	if source.Facts != "" {
		if err := a.storage.UpdateFacts(newID, source.Facts); err != nil {
			return err
		}
	}
	// Copy profile
	if source.ProfileID != "" {
		if err := a.storage.UpdateSessionProfile(newID, source.ProfileID); err != nil {
			return err
		}
	}
	return nil
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

// GetFSMStateInfo returns FSM state information for a session.
func (a *Agent) GetFSMStateInfo(sessionID string) (*fsm.StateInfo, error) {
	if a.fsm == nil {
		return nil, errors.New("FSM not configured")
	}
	return a.fsm.GetStateInfo(sessionID)
}

// buildFSMContext builds a system message with FSM context (task, instructions, feedback)
// to be included in LLM requests.
func (a *Agent) buildFSMContext(sessionID string) (*Message, error) {
	if a.fsm == nil {
		return nil, nil // No FSM, no context to add
	}

	// Get task context from storage
	taskContext, err := a.storage.GetTaskContext(sessionID)
	if err != nil {
		a.logger.LogError(err, "failed to get task context for FSM context building")
		return nil, nil
	}

	if taskContext == nil || taskContext.Done {
		return nil, nil // No active task or task is done
	}

	// Get current state config
	stateConfig, exists := a.fsm.GetStateConfig(taskContext.State)
	if !exists {
		a.logger.LogError(nil, "state not found in config", "state", taskContext.State)
		return nil, nil
	}

	// Build context message
	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Текущая задача\n")
	contextBuilder.WriteString(taskContext.Task + "\n\n")

	contextBuilder.WriteString("## Текущий шаг\n")
	contextBuilder.WriteString(fmt.Sprintf("Шаг %d: %s\n", stateConfig.StepNumber, stateConfig.Description))
	contextBuilder.WriteString("\n")

	contextBuilder.WriteString("## Инструкции для текущего шага\n")
	contextBuilder.WriteString(stateConfig.Instructions + "\n\n")

	// Add validation feedback if available
	if feedback, ok := taskContext.Metadata["validation_feedback"].(string); ok && feedback != "" {
		contextBuilder.WriteString("## Обратная связь от валидатора\n")
		contextBuilder.WriteString(feedback + "\n\n")
	}

	// Add missing items if available
	if missingItems, ok := taskContext.Metadata["missing_items"].([]interface{}); ok && len(missingItems) > 0 {
		contextBuilder.WriteString("## Недостающие элементы (требуют доработки)\n")
		for i, item := range missingItems {
			if str, ok := item.(string); ok {
				contextBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, str))
			}
		}
		contextBuilder.WriteString("\n")
	}

	return &Message{
		Role:    "system",
		Content: contextBuilder.String(),
	}, nil
}

// shouldMakeAutomaticRequest determines if an automatic LLM request should be made
// after a state transition.
func (a *Agent) shouldMakeAutomaticRequest(sessionID string, taskContext *storage.TaskContext) bool {
	if taskContext == nil || taskContext.Done {
		return false
	}

	// Check if task is in error state
	if errorFlag, ok := taskContext.Metadata["error"].(bool); ok && errorFlag {
		return false
	}

	// Check max attempts for the current state
	attemptsKey := fmt.Sprintf("validation_attempts_%s", taskContext.State)
	if attempts, ok := taskContext.Metadata[attemptsKey].(int); ok {
		maxAttempts := 3 // Default
		if a.fsm != nil {
			maxAttempts = a.fsm.GetMaxAttemptsForState(taskContext.State)
		}
		if attempts >= maxAttempts {
			a.logger.Debug("Max attempts reached, skipping automatic request",
				"session", sessionID,
				"state", taskContext.State,
				"attempts", attempts,
				"max_attempts", maxAttempts)
			return false
		}
	}

	// Check if last validation result was NEED_USER_ANSWER
	if lastResult, ok := taskContext.Metadata["last_validation_result"].(string); ok {
		if lastResult == string(fsm.ResultNeedUserAnswer) {
			return false
		}
	}

	// Check if we've made too many automatic requests already
	autoRequestKey := "automatic_request_count"
	autoRequestCount := 0
	if count, ok := taskContext.Metadata[autoRequestKey].(int); ok {
		autoRequestCount = count
	}

	// Limit total automatic requests to prevent infinite loops
	maxAutoRequests := 10
	if autoRequestCount >= maxAutoRequests {
		a.logger.Debug("Max automatic requests reached",
			"session", sessionID,
			"count", autoRequestCount)
		return false
	}

	return true
}

// mergeSystemMessages merges consecutive system messages at the beginning of the history
// into a single system message. This is required because GigaChat API expects
// at most one system message at the beginning.
func mergeSystemMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	// Collect system messages from the beginning
	var systemContents []string
	var i int
	for i = 0; i < len(messages); i++ {
		if messages[i].Role == "system" {
			systemContents = append(systemContents, messages[i].Content)
		} else {
			break
		}
	}

	// If we found 0 or 1 system messages at the beginning, no change needed
	if len(systemContents) <= 1 {
		return messages
	}

	// Merge system messages with double newline separator
	mergedContent := strings.Join(systemContents, "\n\n")

	// Create new messages slice with merged system message followed by the rest
	result := make([]Message, 0, len(messages)-len(systemContents)+1)
	result = append(result, Message{
		Role:    "system",
		Content: mergedContent,
	})

	// Append the non-system messages
	result = append(result, messages[i:]...)

	return result
}

// makeAutomaticRequest makes an automatic LLM request after a state transition.
// This method runs asynchronously (called with go).
func (a *Agent) makeAutomaticRequest(sessionID string, taskContext *storage.TaskContext) {
	// Increment automatic request counter
	autoRequestKey := "automatic_request_count"
	autoRequestCount := 0
	if count, ok := taskContext.Metadata[autoRequestKey].(int); ok {
		autoRequestCount = count
	}
	taskContext.Metadata[autoRequestKey] = autoRequestCount + 1

	// Save updated context with incremented counter
	if err := a.storage.UpdateTaskContext(sessionID, taskContext); err != nil {
		a.logger.LogError(err, "failed to update task context with automatic request count")
	}

	a.logger.Info("Making automatic LLM request after state transition",
		"session", sessionID,
		"state", taskContext.State,
		"auto_request_count", autoRequestCount+1)

	// Get current state config
	stateConfig, exists := a.fsm.GetStateConfig(taskContext.State)
	if !exists {
		a.logger.LogError(nil, "state config not found for automatic request", "state", taskContext.State)
		return
	}

	// Create a system message with instructions for the next step
	systemMsg := Message{
		Role:    "system",
		Content: fmt.Sprintf("Переход к следующему шагу выполнен. Выполни инструкции для текущего шага:\n\n%s", stateConfig.Instructions),
	}

	// Get current session history
	session, err := a.storage.GetSession(sessionID)
	if err != nil {
		a.logger.LogError(err, "failed to get session for automatic request")
		return
	}

	if session == nil {
		a.logger.LogError(nil, "session not found for automatic request", "session_id", sessionID)
		return
	}

	// Convert history to agent messages
	history := make([]Message, len(session.History))
	for i, m := range session.History {
		history[i] = toAgentMessage(m)
	}

	// Add the system message to history
	history = append([]Message{systemMsg}, history...)

	// Merge consecutive system messages to avoid GigaChat API error
	history = mergeSystemMessages(history)

	// Send to LLM
	result, err := a.client.SendMessage(history)
	if err != nil {
		a.logger.LogError(err, "automatic LLM request failed")
		return
	}

	// Add assistant response to storage
	assistantMsg := Message{
		Role:             "assistant",
		Content:          result.Content,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
		TotalTokens:      result.Usage.TotalTokens,
	}
	if err := a.storage.AddMessage(sessionID, toStorageMessage(assistantMsg)); err != nil {
		a.logger.LogError(err, "failed to store automatic assistant message")
	}

	// Process FSM transition for the automatic response
	if a.fsm != nil {
		if _, err := a.fsm.ProcessResponse(sessionID, result.Content); err != nil {
			a.logger.LogError(err, "FSM processing failed for automatic response")
		}
	}

	a.logger.Info("Automatic LLM request completed",
		"session", sessionID,
		"state", taskContext.State)
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = errors.New("session not found")
