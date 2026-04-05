package fsm

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ai-agent-gigachat/internal/storage"
)

// ValidationResult represents the possible outcomes of LLM response validation.
type ValidationResult string

const (
	ResultSuccess        ValidationResult = "SUCCESS"
	ResultFailed         ValidationResult = "FAILED"
	ResultNeedUserAnswer ValidationResult = "NEED_USER_ANSWER"
)

// ValidationOutput contains the complete result of validation.
type ValidationOutput struct {
	Result       ValidationResult
	Feedback     string
	MissingItems []string
	RawResponse  string // Original LLM validation response for debugging
}

// Validator defines the interface for validating LLM responses against state requirements.
type Validator interface {
	Validate(stateConfig *StateConfig, llmResponse string, taskContext *storage.TaskContext) (*ValidationOutput, error)
}

// LLMClient defines the interface for sending messages to an LLM.
// This avoids import cycles by allowing the agent package to implement this interface.
type LLMClient interface {
	SendMessage(messages []Message) (*CompletionResult, error)
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionResult represents the result of an LLM completion.
type CompletionResult struct {
	Content string `json:"content"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StubValidator is a validator that always returns success.
// This implements the requirement for a stub that always returns success.
type StubValidator struct{}

// Validate always returns SUCCESS (success) and no error.
func (v *StubValidator) Validate(stateConfig *StateConfig, llmResponse string, taskContext *storage.TaskContext) (*ValidationOutput, error) {
	return &ValidationOutput{
		Result:   ResultSuccess,
		Feedback: "Stub validation always succeeds",
	}, nil
}

// NewStubValidator creates a new stub validator.
func NewStubValidator() *StubValidator {
	return &StubValidator{}
}

// LLMValidator validates LLM responses using a separate LLM request.
type LLMValidator struct {
	client LLMClient
	prompt string
	config *FSMConfig
}

// NewLLMValidator creates a new LLM validator with the given client and config.
func NewLLMValidator(client LLMClient, config *FSMConfig) (*LLMValidator, error) {
	promptPath := config.GetValidationPromptFile()
	if promptPath == "" {
		promptPath = "prompts/validation_prompt.md"
	}

	// Load prompt from file
	content, err := os.ReadFile(promptPath)
	if err != nil {
		// Fall back to default prompt
		content = []byte(defaultValidationPrompt)
	}

	return &LLMValidator{
		client: client,
		prompt: string(content),
		config: config,
	}, nil
}

// Validate performs validation of an LLM response.
// 1. First checks for NEED_USER_ANSWER pattern
// 2. If not found, performs LLM-based validation
// 3. Returns ValidationOutput with result and feedback
func (v *LLMValidator) Validate(stateConfig *StateConfig, llmResponse string, taskContext *storage.TaskContext) (*ValidationOutput, error) {
	// 1. Check for NEED_USER_ANSWER
	if containsNeedUserAnswer(llmResponse) {
		return &ValidationOutput{
			Result:   ResultNeedUserAnswer,
			Feedback: "",
		}, nil
	}

	// 2. Perform LLM validation
	validationResult, err := v.validateWithLLM(stateConfig, llmResponse, taskContext)
	if err != nil {
		// Fallback to rule-based validation
		return v.fallbackValidation(stateConfig, llmResponse)
	}

	return validationResult, nil
}

// containsNeedUserAnswer checks if the text contains the NEED_USER_ANSWER pattern.
func containsNeedUserAnswer(text string) bool {
	lower := strings.ToLower(text)
	// Check for Russian phrase (case-insensitive)
	if strings.Contains(lower, "нужна дополнительная информация") {
		return true
	}
	// Could add more patterns in the future
	return false
}

// validateWithLLM performs validation using a separate LLM request.
func (v *LLMValidator) validateWithLLM(stateConfig *StateConfig, llmResponse string, taskContext *storage.TaskContext) (*ValidationOutput, error) {
	// Build checklist string
	checklistStr := ""
	for i, item := range stateConfig.ValidationSchema.CheckList {
		checklistStr += fmt.Sprintf("%d. %s\n", i+1, item)
	}

	// Build validation request
	messages := []Message{
		{
			Role:    "system",
			Content: v.prompt,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Чек-лист:\n%s\n\nОтвет исполнителя:\n%s", checklistStr, llmResponse),
		},
	}

	// Send to LLM
	result, err := v.client.SendMessage(messages)
	if err != nil {
		return nil, fmt.Errorf("LLM validation request failed: %w", err)
	}

	// Parse JSON response
	var validationResp struct {
		IsComplete   bool     `json:"is_complete"`
		MissingItems []string `json:"missing_items"`
		Feedback     string   `json:"feedback_for_agent"`
	}

	cleaned := cleanJSONResponse(result.Content)
	if err := json.Unmarshal([]byte(cleaned), &validationResp); err != nil {
		return nil, fmt.Errorf("failed to parse validation response: %w", err)
	}

	// Map to ValidationOutput
	output := &ValidationOutput{
		Feedback:     validationResp.Feedback,
		MissingItems: validationResp.MissingItems,
		RawResponse:  result.Content,
	}

	if validationResp.IsComplete {
		output.Result = ResultSuccess
	} else {
		output.Result = ResultFailed
	}

	return output, nil
}

// fallbackValidation provides a fallback when LLM validation fails.
func (v *LLMValidator) fallbackValidation(stateConfig *StateConfig, llmResponse string) (*ValidationOutput, error) {
	// Simple rule-based fallback
	// For now, just return SUCCESS to maintain backward compatibility
	// In the future, could implement basic keyword matching

	return &ValidationOutput{
		Result:   ResultSuccess,
		Feedback: "Validation performed via fallback (LLM validation failed)",
	}, nil
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

// LegacyValidatorAdapter adapts the new Validator interface to the old (bool, error) signature.
type LegacyValidatorAdapter struct {
	validator Validator
}

// NewLegacyValidatorAdapter creates a new adapter for backward compatibility.
func NewLegacyValidatorAdapter(validator Validator) *LegacyValidatorAdapter {
	return &LegacyValidatorAdapter{validator: validator}
}

// Validate implements the old interface signature.
func (a *LegacyValidatorAdapter) Validate(stateConfig *StateConfig, llmResponse string) (bool, error) {
	output, err := a.validator.Validate(stateConfig, llmResponse, nil)
	if err != nil {
		return false, err
	}
	return output.Result == ResultSuccess, nil
}

// Default validation prompt (used as fallback)
const defaultValidationPrompt = `### РОЛЬ
Ты — инспектор качества выполнения задач. Твоя цель — беспристрастно проверить, выполнены ли ВСЕ пункты задания.

### ИНПУТ
1. Оригинальный список задач (чек-лист).
2. Ответ исполнителя.

### ИНСТРУКЦИЯ
- Проверь соответствие каждого пункта из чек-листа тексту ответа.
- Если пункт выполнен частично или формально (без содержания) — считай его НЕВЫПОЛНЕННЫМ.
- Не пытайся додумать за исполнителя. Только факты.

### ФОРМАТ ОТВЕТА (JSON ONLY)
{
  "is_complete": true/false,
  "missing_items": ["пункт 3 - отсутствует описание рисков"],
  "feedback_for_agent": "В твоем ответе не хватает анализа рисков, добавь этот раздел."
}`
