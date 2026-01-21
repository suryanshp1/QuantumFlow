package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/quantumflow/quantumflow/internal/models"
)

// Config holds the inference client configuration
type Config struct {
	OllamaURL   string  // Default: http://localhost:11434
	Model       string  // Default: qwen2.5-coder:7b
	ContextSize int     // Default: 32768
	Temperature float64 // Default: 0.7
	Timeout     time.Duration
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		OllamaURL:   "http://localhost:11434",
		Model:       "qwen2.5-coder:7b",
		ContextSize: 32768,
		Temperature: 0.7,
		Timeout:     15 * time.Minute, // Increased for slow local models
	}
}

// Client is the main inference client for Ollama
type Client struct {
	config     *Config
	httpClient *http.Client
}

// NewClient creates a new inference client
func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig()
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GenerateRequest represents a request to Ollama
type GenerateRequest struct {
	Model       string          `json:"model"`
	Prompt      string          `json:"prompt"`
	Messages    []models.Message `json:"messages,omitempty"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// GenerateResponse represents a response from Ollama
type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Done               bool      `json:"done"`
	Context            []int     `json:"context,omitempty"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// InferenceResult holds the final result of an inference call
type InferenceResult struct {
	Response     string
	TokensPerSec float64
	Latency      time.Duration
	Error        error
}

// Generate generates a response using the configured model
func (c *Client) Generate(ctx context.Context, prompt string, streaming bool) (<-chan string, error) {
	req := GenerateRequest{
		Model:       c.config.Model,
		Prompt:      prompt,
		Stream:      streaming,
		Temperature: c.config.Temperature,
		Options: map[string]interface{}{
			"num_ctx": c.config.ContextSize,
		},
	}

	return c.generate(ctx, req)
}

// GenerateWithMessages generates a response using the chat API with message history
func (c *Client) GenerateWithMessages(ctx context.Context, messages []models.Message, streaming bool) (<-chan string, error) {
	req := GenerateRequest{
		Model:       c.config.Model,
		Messages:    messages,
		Stream:      streaming,
		Temperature: c.config.Temperature,
		Options: map[string]interface{}{
			"num_ctx": c.config.ContextSize,
		},
	}

	return c.generateChat(ctx, req)
}

// generate makes a request to Ollama's /api/generate endpoint
func (c *Client) generate(ctx context.Context, req GenerateRequest) (<-chan string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.OllamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create channel for streaming responses
	responseChan := make(chan string, 100)

	go func() {
		defer close(responseChan)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var genResp GenerateResponse
			if err := json.Unmarshal(scanner.Bytes(), &genResp); err != nil {
				// Log error but continue processing
				continue
			}

			if genResp.Response != "" {
				select {
				case responseChan <- genResp.Response:
				case <-ctx.Done():
					return
				}
			}

			if genResp.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			// Log error
		}
	}()

	return responseChan, nil
}

// generateChat makes a request to Ollama's /api/chat endpoint
func (c *Client) generateChat(ctx context.Context, req GenerateRequest) (<-chan string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.OllamaURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create channel for streaming responses
	responseChan := make(chan string, 100)

	go func() {
		defer close(responseChan)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var chatResp struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}

			if err := json.Unmarshal(scanner.Bytes(), &chatResp); err != nil {
				continue
			}

			if chatResp.Message.Content != "" {
				select {
				case responseChan <- chatResp.Message.Content:
				case <-ctx.Done():
					return
				}
			}

			if chatResp.Done {
				return
			}
		}
	}()

	return responseChan, nil
}

// GenerateSync performs a synchronous (non-streaming) generation
func (c *Client) GenerateSync(ctx context.Context, prompt string) (*InferenceResult, error) {
	startTime := time.Now()

	req := GenerateRequest{
		Model:       c.config.Model,
		Prompt:      prompt,
		Stream:      false,
		Temperature: c.config.Temperature,
		Options: map[string]interface{}{
			"num_ctx": c.config.ContextSize,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.OllamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var genResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	latency := time.Since(startTime)
	tokensPerSec := 0.0
	if genResp.EvalDuration > 0 && genResp.EvalCount > 0 {
		tokensPerSec = float64(genResp.EvalCount) / (float64(genResp.EvalDuration) / 1e9)
	}

	return &InferenceResult{
		Response:     genResp.Response,
		TokensPerSec: tokensPerSec,
		Latency:      latency,
	}, nil
}

// ListModels lists available models
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.config.OllamaURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	models := make([]string, len(result.Models))
	for i, m := range result.Models {
		models[i] = m.Name
	}

	return models, nil
}

// PullModel pulls a model from Ollama registry
func (c *Client) PullModel(ctx context.Context, modelName string) error {
	req := map[string]string{
		"name": modelName,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.OllamaURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Stream the pull progress
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		// Could parse and display progress here
		_ = scanner.Text()
	}

	return scanner.Err()
}
