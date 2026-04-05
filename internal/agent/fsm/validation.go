package fsm

// Validator defines the interface for validating LLM responses against state requirements.
type Validator interface {
	Validate(stateConfig *StateConfig, llmResponse string) (bool, error)
}

// StubValidator is a validator that always returns success.
// This implements the requirement for a stub that always returns success.
type StubValidator struct{}

// Validate always returns true (success) and no error.
func (v *StubValidator) Validate(stateConfig *StateConfig, llmResponse string) (bool, error) {
	// This is a stub implementation that always returns success
	// In a real implementation, this would validate against the check_list
	// using LLM or rule-based validation

	// Log the validation attempt (would be done by the FSM logger)
	return true, nil
}

// NewStubValidator creates a new stub validator.
func NewStubValidator() *StubValidator {
	return &StubValidator{}
}
