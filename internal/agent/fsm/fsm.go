package fsm

import (
	"errors"
	"fmt"
	"time"

	"ai-agent-gigachat/internal/logging"
	"ai-agent-gigachat/internal/storage"
)

// TransitionHistory records a state transition.
type TransitionHistory struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
}

// StateInfo provides information about the current state for UI display.
type StateInfo struct {
	StepNumber  int    `json:"step_number"`
	StepsCount  int    `json:"steps_count"`
	Description string `json:"description"`
	State       string `json:"state"`
	Done        bool   `json:"done"`
	Error       bool   `json:"error"` // True if state not found in config
}

// FSM manages the finite state machine for task processing.
type FSM struct {
	config    *FSMConfig
	storage   storage.Storage
	logger    *logging.Logger
	validator Validator
}

// NewFSM creates a new FSM instance with the given configuration path and storage.
func NewFSM(configPath string, storage storage.Storage) (*FSM, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Create validator (default to stub validator)
	validator := NewStubValidator()

	return &FSM{
		config:    config,
		storage:   storage,
		logger:    logging.Default(),
		validator: validator,
	}, nil
}

// NewFSMWithValidator creates a new FSM instance with a custom validator.
func NewFSMWithValidator(configPath string, storage storage.Storage, validator Validator) (*FSM, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	return &FSM{
		config:    config,
		storage:   storage,
		logger:    logging.Default(),
		validator: validator,
	}, nil
}

// InitializeSession initializes the FSM for a new session with the first user message.
func (f *FSM) InitializeSession(sessionID, firstMessage string) error {
	// Check if session already has task context
	existing, err := f.storage.GetTaskContext(sessionID)
	if err != nil && err != storage.ErrSessionNotFound {
		return fmt.Errorf("failed to check existing context: %w", err)
	}

	if existing != nil {
		// Session already initialized
		return nil
	}

	// Create initial task context
	context := &storage.TaskContext{
		State: f.config.GetInitialState(),
		Task:  firstMessage,
		Done:  false,
		Metadata: map[string]interface{}{
			"step_number":        f.config.GetStepNumber(f.config.GetInitialState()),
			"validation_results": []interface{}{},
			"transition_history": []TransitionHistory{
				{
					From:      "",
					To:        f.config.GetInitialState(),
					Timestamp: time.Now(),
					Success:   true,
				},
			},
		},
	}

	// Save to storage
	if err := f.storage.UpdateTaskContext(sessionID, context); err != nil {
		return fmt.Errorf("failed to save task context: %w", err)
	}

	f.logger.Info("FSM initialized", "session", sessionID, "state", context.State)
	return nil
}

// ProcessResponse processes an LLM response and transitions to the next state if validation passes.
func (f *FSM) ProcessResponse(sessionID, llmResponse string) (*storage.TaskContext, error) {
	// Get current context
	context, err := f.storage.GetTaskContext(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task context: %w", err)
	}

	if context == nil {
		return nil, errors.New("session not initialized with FSM")
	}

	if context.Done {
		return context, nil // Task already completed
	}

	// Get current state config
	stateConfig, exists := f.config.GetState(context.State)
	if !exists {
		return nil, fmt.Errorf("state %s not found in config", context.State)
	}

	// Run validation using the validator
	validationOutput, validationErr := f.validator.Validate(stateConfig, llmResponse, context)
	if validationErr != nil {
		return nil, fmt.Errorf("validation failed: %w", validationErr)
	}

	// Store validation feedback in context metadata
	if validationOutput.Feedback != "" {
		context.Metadata["validation_feedback"] = validationOutput.Feedback
	}
	if len(validationOutput.MissingItems) > 0 {
		context.Metadata["missing_items"] = validationOutput.MissingItems
	}
	context.Metadata["last_validation_result"] = string(validationOutput.Result)

	// Track validation attempts for this state
	attemptsKey := fmt.Sprintf("validation_attempts_%s", context.State)
	currentAttempts := 1
	if attempts, ok := context.Metadata[attemptsKey].(int); ok {
		currentAttempts = attempts + 1
	}
	context.Metadata[attemptsKey] = currentAttempts

	// Check if we've exceeded max attempts for this state
	maxAttempts := f.config.GetMaxAttempts(context.State)
	if currentAttempts > maxAttempts {
		f.logger.Warn("Max validation attempts exceeded",
			"session", sessionID,
			"state", context.State,
			"attempts", currentAttempts,
			"max_attempts", maxAttempts)
		// Mark as error state
		context.Metadata["error"] = true
		context.Metadata["error_reason"] = fmt.Sprintf("Exceeded maximum validation attempts (%d)", maxAttempts)
	}

	// Handle NEED_USER_ANSWER result - no state transition
	if validationOutput.Result == ResultNeedUserAnswer {
		f.logger.Info("Validation result: NEED_USER_ANSWER",
			"session", sessionID,
			"state", context.State)
		// Save context with feedback (if any)
		if err := f.storage.UpdateTaskContext(sessionID, context); err != nil {
			return nil, fmt.Errorf("failed to update task context: %w", err)
		}
		return context, nil
	}

	// Determine next state based on validation result
	var nextState string
	var success bool
	if validationOutput.Result == ResultSuccess {
		nextState = stateConfig.OnSuccess
		success = true
	} else { // ResultFailed
		nextState = stateConfig.OnFail
		success = false
	}

	// Check if task is completed
	done := false
	if nextState == "exit" {
		done = true
		nextState = context.State // Stay in current state but mark as done
	}

	// Update context
	context.Done = done

	// Only transition state if not in NEED_USER_ANSWER and not in error state
	shouldTransition := !done && nextState != context.State && validationOutput.Result != ResultNeedUserAnswer
	if shouldTransition {
		context.State = nextState

		// Reset attempts for the new state
		newAttemptsKey := fmt.Sprintf("validation_attempts_%s", nextState)
		context.Metadata[newAttemptsKey] = 0

		// Update metadata
		if meta, ok := context.Metadata["transition_history"].([]TransitionHistory); ok {
			context.Metadata["transition_history"] = append(meta, TransitionHistory{
				From:      context.State,
				To:        nextState,
				Timestamp: time.Now(),
				Success:   success,
			})
		} else {
			// Initialize if not present
			context.Metadata["transition_history"] = []TransitionHistory{
				{
					From:      context.State,
					To:        nextState,
					Timestamp: time.Now(),
					Success:   success,
				},
			}
		}

		if nextStateConfig, exists := f.config.GetState(nextState); exists {
			context.Metadata["step_number"] = nextStateConfig.StepNumber
		}
	}

	// Save updated context
	if err := f.storage.UpdateTaskContext(sessionID, context); err != nil {
		return nil, fmt.Errorf("failed to update task context: %w", err)
	}

	f.logger.Info("FSM transition",
		"session", sessionID,
		"from", context.State,
		"to", nextState,
		"success", success,
		"result", validationOutput.Result,
		"done", done,
		"attempts", currentAttempts)

	return context, nil
}

// GetStateInfo returns information about the current state for UI display.
func (f *FSM) GetStateInfo(sessionID string) (*StateInfo, error) {
	context, err := f.storage.GetTaskContext(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task context: %w", err)
	}

	if context == nil {
		// No FSM context - return empty info
		return &StateInfo{
			Error: false,
			Done:  false,
		}, nil
	}

	info := &StateInfo{
		State:      context.State,
		Done:       context.Done,
		StepNumber: 0,
		StepsCount: f.config.GetStatesCount(),
	}

	// If task is done, show completion message
	if context.Done {
		return info, nil
	}

	// Get state config for current state
	stateConfig, exists := f.config.GetState(context.State)
	if !exists {
		info.Error = true
		return info, nil
	}

	info.StepNumber = stateConfig.StepNumber
	info.Description = stateConfig.Description
	info.Error = false

	return info, nil
}

// validate is a stub implementation that always returns success.
// In the future, this should validate against the check_list using LLM or rule-based validation.
func (f *FSM) validate(stateConfig *StateConfig, llmResponse string) (bool, error) {
	// Stub implementation - always return success
	// In the future, this should validate against check_list
	// using LLM or rule-based validation

	f.logger.Debug("Validation stub called",
		"state", stateConfig.Description,
		"check_list", stateConfig.ValidationSchema.CheckList)

	return true, nil
}

// GetCurrentState returns the current state for a session.
func (f *FSM) GetCurrentState(sessionID string) (string, error) {
	context, err := f.storage.GetTaskContext(sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get task context: %w", err)
	}

	if context == nil {
		return "", nil
	}

	return context.State, nil
}

// GetStateConfig returns the configuration for a specific state.
func (f *FSM) GetStateConfig(state string) (*StateConfig, bool) {
	return f.config.GetState(state)
}

// GetMaxAttemptsForState returns the maximum attempts for a state.
func (f *FSM) GetMaxAttemptsForState(state string) int {
	return f.config.GetMaxAttempts(state)
}

// IsDone checks if the task is completed for a session.
func (f *FSM) IsDone(sessionID string) (bool, error) {
	context, err := f.storage.GetTaskContext(sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to get task context: %w", err)
	}

	if context == nil {
		return false, nil
	}

	return context.Done, nil
}
