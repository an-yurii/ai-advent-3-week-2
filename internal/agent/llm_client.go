package agent

// LLMClient defines the common interface for all LLM providers.
type LLMClient interface {
	// SendMessage sends a conversation history to the LLM and returns the assistant's reply and token usage.
	SendMessage(messages []Message) (*CompletionResult, error)
}
