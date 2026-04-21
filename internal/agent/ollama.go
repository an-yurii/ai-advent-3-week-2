package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-agent-gigachat/internal/logging"
)

// OllamaClient implements LLMClient for local Ollama API.
type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
	logger  *logging.Logger
}

// NewOllamaClient creates a new Ollama client with the given base URL and model.
func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama2"
	}
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.Default(),
	}
}

// generateRequest is the JSON structure for Ollama's generate API.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the JSON structure returned by Ollama's generate API.
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// formatMessages converts a slice of Message to a single prompt string.
// Ollama expects a plain text prompt, so we format as:
// <role>: <content>
//
// Example:
// user: Hello
// assistant: Hi there!
// user: How are you?
func formatMessages(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(msg.Role)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// SendMessage sends a conversation history to Ollama and returns the assistant's reply.
func (c *OllamaClient) SendMessage(messages []Message) (*CompletionResult, error) {
	prompt := formatMessages(messages)

	reqBody := generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	url := c.baseURL + "/api/generate"
	c.logger.Debug("Sending request to Ollama", "url", url, "model", c.model)

	resp, err := c.client.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(body))
	}

	var genResp generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	// Ollama doesn't provide token usage information, so we return empty usage
	return &CompletionResult{
		Content: strings.TrimSpace(genResp.Response),
		Usage:   &CompletionUsage{}, // Zero token counts
	}, nil
}

// Ping checks if the Ollama service is reachable.
func (c *OllamaClient) Ping() error {
	url := c.baseURL + "/api/tags"
	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to ping Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama ping returned status %d", resp.StatusCode)
	}

	return nil
}
