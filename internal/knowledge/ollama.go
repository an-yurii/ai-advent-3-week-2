package knowledge

import (
	"ai-agent-gigachat/internal/logging"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient is an interface for getting embeddings from Ollama.
type OllamaClient interface {
	// GetEmbedding returns the embedding vector for the given text.
	GetEmbedding(text string) ([]float32, error)
	// Ping checks if the Ollama service is reachable.
	Ping() error
}

// ollamaClient implements OllamaClient using HTTP requests to Ollama's API.
type ollamaClient struct {
	baseURL string
	model   string
	client  *http.Client
	logger  *logging.Logger
}

// NewOllamaClient creates a new Ollama client with the given base URL and model.
func NewOllamaClient(baseURL, model string) OllamaClient {
	return &ollamaClient{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.Default(),
	}
}

// embeddingRequest is the JSON structure for Ollama's embedding API.
type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// embeddingResponse is the JSON structure returned by Ollama's embedding API.
type embeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// GetEmbedding sends a text to Ollama and returns its embedding vector.
func (c *ollamaClient) GetEmbedding(text string) ([]float32, error) {
	reqBody := embeddingRequest{
		Model:  c.model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	url := c.baseURL + "/api/embeddings"
	// Log request
	c.logger.LogOllamaRequest(url, map[string][]string{
		"Content-Type": {"application/json"},
	}, string(jsonData))

	resp, err := c.client.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send embedding request: %w", err)
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
		return nil, fmt.Errorf("ollama API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	if len(embResp.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding returned from Ollama")
	}

	return embResp.Embedding, nil
}

// Ping sends a simple request to check if Ollama is reachable.
func (c *ollamaClient) Ping() error {
	url := c.baseURL + "/api/tags"
	// Log request
	c.logger.LogOllamaRequest(url, map[string][]string{}, "")

	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to ping Ollama: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for logging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response
	c.logger.LogOllamaResponse(resp.StatusCode, map[string][]string{
		"Content-Type": resp.Header.Values("Content-Type"),
	}, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping returned status %d", resp.StatusCode)
	}

	return nil
}
