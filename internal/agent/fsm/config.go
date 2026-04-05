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
}

// FSMConfig defines the complete FSM configuration.
type FSMConfig struct {
	InitialState string                 `yaml:"initial_state"`
	States       map[string]StateConfig `yaml:"states"`
}

// LoadConfig loads and validates the FSM configuration from a YAML file.
func LoadConfig(path string) (*FSMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config FSMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// Validate config
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// validateConfig performs validation on the loaded configuration.
func validateConfig(config *FSMConfig) error {
	if config.InitialState == "" {
		return errors.New("initial_state is required")
	}

	if _, exists := config.States[config.InitialState]; !exists {
		return fmt.Errorf("initial_state %s not defined in states", config.InitialState)
	}

	// Validate each state
	for stateName, state := range config.States {
		if state.StepNumber <= 0 {
			return fmt.Errorf("state %s: step_number must be positive", stateName)
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

// GetStatesCount returns the total number of states.
func (c *FSMConfig) GetStatesCount() int {
	return len(c.States)
}
