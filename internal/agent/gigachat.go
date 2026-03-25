package agent

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai-agent-gigachat/internal/logging"

	"github.com/google/uuid"
)

const (
	oauthURL  = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	authScope = "GIGACHAT_API_PERS"
)

// tokenManager handles OAuth token acquisition and refresh.
type tokenManager struct {
	apiKey     string
	httpClient *http.Client
	logger     *logging.Logger
	mu         sync.RWMutex
	token      string
	expiry     time.Time
}

// getToken returns a valid access token, fetching a new one if necessary.
func (tm *tokenManager) getToken() (string, error) {
	tm.mu.RLock()
	token := tm.token
	expiry := tm.expiry
	tm.mu.RUnlock()

	if token != "" && time.Now().Before(expiry) {
		return token, nil
	}

	// Need to fetch new token
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring lock
	if tm.token != "" && time.Now().Before(tm.expiry) {
		return tm.token, nil
	}

	// Prepare request
	body := strings.NewReader("scope=" + authScope)
	req, err := http.NewRequest("POST", oauthURL, body)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tm.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", uuid.New().String())

	// Log request (mask auth header)
	tm.logger.LogGigaChatRequest(oauthURL, map[string][]string{
		"Authorization": {"Bearer ***"},
		"Content-Type":  {"application/x-www-form-urlencoded"},
		"RqUID":         {req.Header.Get("RqUID")},
	}, "scope="+authScope)

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		tm.logger.LogError(err, "token request failed")
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response body: %w", err)
	}

	tm.logger.LogGigaChatResponse(resp.StatusCode, map[string][]string{
		"Content-Type": resp.Header.Values("Content-Type"),
	}, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth endpoint returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	// Set token and expiry (with a small buffer, e.g., 10 seconds earlier)
	expiresIn := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 30 * time.Minute // default
	}
	tm.token = tokenResp.AccessToken
	tm.expiry = time.Now().Add(expiresIn - 10*time.Second)

	return tm.token, nil
}

// GigaChatClient handles communication with the GigaChat API.
type GigaChatClient struct {
	tokenManager *tokenManager
	baseURL      string
	httpClient   *http.Client
	logger       *logging.Logger
}

// NewGigaChatClient creates a new GigaChat client.
func NewGigaChatClient(apiKey string) *GigaChatClient {
	// Create HTTP client with custom transport to skip TLS verification
	// (necessary for the OAuth endpoint with self‑signed certificate)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
	logger := logging.Default()
	return &GigaChatClient{
		tokenManager: &tokenManager{
			apiKey:     apiKey,
			httpClient: httpClient,
			logger:     logger,
		},
		baseURL:    "https://gigachat.devices.sberbank.ru/api/v1/chat/completions",
		httpClient: httpClient,
		logger:     logger,
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

// CompletionUsage represents token usage from GigaChat API.
type CompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionResponse represents the response from GigaChat.
type ChatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage *CompletionUsage `json:"usage,omitempty"`
}

// CompletionResult holds the result of a chat completion.
type CompletionResult struct {
	Content string
	Usage   *CompletionUsage
}

// SendMessage sends a conversation history to GigaChat and returns the assistant's reply and token usage.
func (c *GigaChatClient) SendMessage(messages []Message) (*CompletionResult, error) {
	// Obtain access token
	token, err := c.tokenManager.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain access token: %w", err)
	}

	reqPayload := ChatCompletionRequest{
		Model:       "GigaChat",
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Log request (mask token)
	c.logger.LogGigaChatRequest(c.baseURL, map[string][]string{
		"Authorization": {"Bearer ***"},
		"Content-Type":  {"application/json"},
	}, string(body))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.LogError(err, "GigaChat request failed")
		return nil, fmt.Errorf("GigaChat API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log response
	c.logger.LogGigaChatResponse(resp.StatusCode, map[string][]string{
		"Content-Type": resp.Header.Values("Content-Type"),
	}, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GigaChat API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var completionResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &CompletionResult{
		Content: completionResp.Choices[0].Message.Content,
		Usage:   completionResp.Usage,
	}, nil
}
