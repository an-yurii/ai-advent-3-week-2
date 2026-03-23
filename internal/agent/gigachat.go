package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-agent-gigachat/internal/logging"
)

// GigaChatClient handles communication with the GigaChat API.
type GigaChatClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	logger     *logging.Logger
}

// NewGigaChatClient creates a new GigaChat client.
func NewGigaChatClient(apiKey string) *GigaChatClient {
	return &GigaChatClient{
		apiKey:  apiKey,
		baseURL: "https://gigachat.devices.sberbank.ru/api/v1/chat/completions",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.Default(),
	}
}

// ChatCompletionRequest represents the request payload for GigaChat.
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	// Optional parameters
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// ChatCompletionResponse represents the response from GigaChat.
type ChatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// SendMessage sends a conversation history to GigaChat and returns the assistant's reply.
func (c *GigaChatClient) SendMessage(messages []Message) (string, error) {
	reqPayload := ChatCompletionRequest{
		Model:       "GigaChat",
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   1024,
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Log request
	c.logger.LogGigaChatRequest(c.baseURL, map[string][]string{
		"Authorization": {"Bearer ***"},
		"Content-Type":  {"application/json"},
	}, string(body))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.LogError(err, "GigaChat request failed")
		return "", fmt.Errorf("GigaChat API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response
	c.logger.LogGigaChatResponse(resp.StatusCode, map[string][]string{
		"Content-Type": resp.Header.Values("Content-Type"),
	}, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GigaChat API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var completionResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return completionResp.Choices[0].Message.Content, nil
}