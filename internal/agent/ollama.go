package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
	options *chatOptions
}

// parseOptionsFromEnv reads environment variables and returns a chatOptions struct.
func parseOptionsFromEnv() *chatOptions {
	opts := &chatOptions{}

	// Temperature
	if s := os.Getenv("OLLAMA_TEMPERATURE"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			opts.Temperature = &v
		}
	}
	// TopK
	if s := os.Getenv("OLLAMA_TOP_K"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			opts.TopK = &v
		}
	}
	// TopP
	if s := os.Getenv("OLLAMA_TOP_P"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			opts.TopP = &v
		}
	}
	// NumCtx
	if s := os.Getenv("OLLAMA_NUM_CTX"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			opts.NumCtx = &v
		}
	}
	// NumPredict
	if s := os.Getenv("OLLAMA_NUM_PREDICT"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			opts.NumPredict = &v
		}
	}
	// Seed
	if s := os.Getenv("OLLAMA_SEED"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			opts.Seed = &v
		}
	}

	// Return nil if no options were set
	if opts.Temperature == nil && opts.TopK == nil && opts.TopP == nil &&
		opts.NumCtx == nil && opts.NumPredict == nil && opts.Seed == nil {
		return nil
	}
	return opts
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
		logger:  logging.Default(),
		options: parseOptionsFromEnv(),
	}
}

// chatOptions represents the optional parameters for Ollama's chat API.
type chatOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	NumCtx      *int     `json:"num_ctx,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"`
	Seed        *int     `json:"seed,omitempty"`
}

// chatRequest is the JSON structure for Ollama's chat API.
type chatRequest struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Stream   bool         `json:"stream"`
	Options  *chatOptions `json:"options,omitempty"`
}

// chatResponse is the JSON structure returned by Ollama's chat API.
type chatResponse struct {
	Model              string  `json:"model"`
	CreatedAt          string  `json:"created_at"`
	Message            Message `json:"message"`
	Done               bool    `json:"done"`
	DoneReason         string  `json:"done_reason,omitempty"`
	TotalDuration      int64   `json:"total_duration,omitempty"`
	LoadDuration       int64   `json:"load_duration,omitempty"`
	PromptEvalCount    int     `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64   `json:"prompt_eval_duration,omitempty"`
	EvalCount          int     `json:"eval_count,omitempty"`
	EvalDuration       int64   `json:"eval_duration,omitempty"`
}

// SendMessage sends a conversation history to Ollama and returns the assistant's reply.
func (c *OllamaClient) SendMessage(messages []Message) (*CompletionResult, error) {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Options:  c.options,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	url := c.baseURL + "/api/chat"
	c.logger.Debug("Sending request to Ollama", "url", url, "model", c.model)

	// Log request
	c.logger.LogOllamaRequest(url, map[string][]string{
		"Content-Type": {"application/json"},
	}, string(jsonData))

	resp, err := c.client.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for logging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response
	c.logger.LogOllamaResponse(resp.StatusCode, map[string][]string{
		"Content-Type": resp.Header.Values("Content-Type"),
	}, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	// Extract token usage from Ollama response
	usage := &CompletionUsage{
		PromptTokens:     chatResp.PromptEvalCount,
		CompletionTokens: chatResp.EvalCount,
		TotalTokens:      chatResp.PromptEvalCount + chatResp.EvalCount,
	}

	return &CompletionResult{
		Content: strings.TrimSpace(chatResp.Message.Content),
		Usage:   usage,
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
