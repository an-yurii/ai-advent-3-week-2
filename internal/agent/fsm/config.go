package fsm

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ValidationSchema defines the validation rules for a state.
type ValidationSchema struct {
	CheckList []string `yaml:"check_list"`
}

// StateConfig defines the configuration for a single state.
type StateConfig struct {
	StepNumber       int              `yaml:"step_number"`
	Description      string           `yaml:"description"`
	Instructions     string           `yaml:"instructions"`
	ValidationSchema ValidationSchema `yaml:"validation_schema"`
	OnSuccess        string           `yaml:"on_success"`
	OnFail           string           `yaml:"on_fail"`
	MaxAttempts      int              `yaml:"max_attempts,omitempty"` // Optional per-state override
}

// FSMConfig defines the complete FSM configuration.
type FSMConfig struct {
	InitialState         string                 `yaml:"initial_state"`
	States               map[string]StateConfig `yaml:"states"`
	MaxAttempts          int                    `yaml:"max_attempts,omitempty"`           // Global maximum attempts, default 3
	ValidationPromptFile string                 `yaml:"validation_prompt_file,omitempty"` // Path to validation prompt file
}

// LoadConfig loads and validates the FSM configuration from a YAML file.
func LoadConfig(path string) (*FSMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to unmarshal with wrapper first (for backward compatibility)
	var wrapper struct {
		FSMConfig *FSMConfig `yaml:"fsm_config"`
	}

	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// If wrapper contains config, use it
	var config *FSMConfig
	if wrapper.FSMConfig != nil {
		config = wrapper.FSMConfig
	} else {
		// Fallback: try direct unmarshal (for old format)
		config = &FSMConfig{}
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config (direct): %w", err)
		}
	}

	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// validateConfig performs validation on the loaded configuration.
func validateConfig(config *FSMConfig) error {
	if config.InitialState == "" {
		return errors.New("initial_state is required")
	}

	if _, exists := config.States[config.InitialState]; !exists {
		return fmt.Errorf("initial_state %s not defined in states", config.InitialState)
	}

	// Set defaults
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 3 // Default global maximum attempts
	}

	// Validate each state
	for stateName, state := range config.States {
		if state.StepNumber <= 0 {
			return fmt.Errorf("state %s: step_number must be positive", stateName)
		}

		// Validate max attempts if specified
		if state.MaxAttempts < 0 {
			return fmt.Errorf("state %s: max_attempts must be non-negative", stateName)
		}

		// Validate transitions
		if state.OnSuccess != "exit" {
			if _, exists := config.States[state.OnSuccess]; !exists {
				return fmt.Errorf("state %s: on_success state %s not defined", stateName, state.OnSuccess)
			}
		}

		if _, exists := config.States[state.OnFail]; !exists {
			return fmt.Errorf("state %s: on_fail state %s not defined", stateName, state.OnFail)
		}
	}

	return nil
}

// GetInitialState returns the initial state from the configuration.
func (c *FSMConfig) GetInitialState() string {
	return c.InitialState
}

// GetState returns the configuration for a specific state.
func (c *FSMConfig) GetState(state string) (*StateConfig, bool) {
	cfg, exists := c.States[state]
	return &cfg, exists
}

// GetStepNumber returns the step number for a state.
func (c *FSMConfig) GetStepNumber(state string) int {
	if cfg, exists := c.States[state]; exists {
		return cfg.StepNumber
	}
	return 0
}

// GetMaxAttempts returns the maximum attempts for a state.
// If the state has a specific max_attempts defined, use it.
// Otherwise, use the global MaxAttempts value.
func (c *FSMConfig) GetMaxAttempts(state string) int {
	if cfg, exists := c.States[state]; exists && cfg.MaxAttempts > 0 {
		return cfg.MaxAttempts
	}
	if c.MaxAttempts > 0 {
		return c.MaxAttempts
	}
	return 3 // Default fallback
}

// GetValidationPromptFile returns the path to the validation prompt file.
// Returns empty string if not configured.
func (c *FSMConfig) GetValidationPromptFile() string {
	return c.ValidationPromptFile
}

// GetStatesCount returns the total number of states.
func (c *FSMConfig) GetStatesCount() int {
	return len(c.States)
}
