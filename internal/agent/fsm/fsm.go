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
	config  *FSMConfig
	storage storage.Storage
	logger  *logging.Logger
}

// NewFSM creates a new FSM instance with the given configuration path and storage.
func NewFSM(configPath string, storage storage.Storage) (*FSM, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	return &FSM{
		config:  config,
		storage: storage,
		logger:  logging.Default(),
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

	// Run validation (stub implementation)
	success, validationErr := f.validate(stateConfig, llmResponse)
	if validationErr != nil {
		return nil, fmt.Errorf("validation failed: %w", validationErr)
	}

	// Determine next state
	var nextState string
	if success {
		nextState = stateConfig.OnSuccess
	} else {
		nextState = stateConfig.OnFail
	}

	// Check if task is completed
	done := false
	if nextState == "exit" {
		done = true
		nextState = context.State // Stay in current state but mark as done
	}

	// Update context
	context.Done = done
	if !done && nextState != context.State {
		context.State = nextState

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
		"done", done)

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
