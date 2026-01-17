package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HuggingFaceEmbedding implements EmbeddingGenerator using local embedding models
type HuggingFaceEmbedding struct {
	apiURL     string
	model      string
	dimensions int
	httpClient *http.Client
}

// NewHuggingFaceEmbedding creates a new embedding generator
// Uses local sentence-transformers via HTTP API (typically running on localhost)
func NewHuggingFaceEmbedding(config *Config) (*HuggingFaceEmbedding, error) {
	return &HuggingFaceEmbedding{
		apiURL:     "http://localhost:8000", // sentence-transformers API
		model:      config.EmbeddingModel,
		dimensions: config.EmbeddingDimensions,
		httpClient: &http.Client{},
	}, nil
}

// Generate creates an embedding vector for text
func (e *HuggingFaceEmbedding) Generate(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.GenerateBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings generated")
	}
	return embeddings[0], nil
}

// GenerateBatch creates embeddings for multiple texts
func (e *HuggingFaceEmbedding) GenerateBatch(ctx context.Context, texts []string) ([][]float32, error) {
	requestBody := map[string]interface{}{
		"inputs": texts,
		"model":  e.model,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.apiURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// Dimensions returns the embedding vector dimensionality
func (e *HuggingFaceEmbedding) Dimensions() int {
	return e.dimensions
}

// SimpleEmbedding is a fallback embedding generator using simple word hashing
// Used when external embedding service is unavailable
type SimpleEmbedding struct {
	dimensions int
}

// NewSimpleEmbedding creates a simple hash-based embedding generator
func NewSimpleEmbedding(dimensions int) *SimpleEmbedding {
	return &SimpleEmbedding{dimensions: dimensions}
}

// Generate creates a simple hash-based embedding
func (e *SimpleEmbedding) Generate(ctx context.Context, text string) ([]float32, error) {
	// Normalize text
	text = strings.ToLower(strings.TrimSpace(text))
	words := strings.Fields(text)

	// Initialize embedding vector
	embedding := make([]float32, e.dimensions)

	// Simple word hashing with position weighting
	for i, word := range words {
		hash := simpleHash(word)
		position := float32(i) / float32(len(words))

		for j := 0; j < e.dimensions; j++ {
			// Distribute word hash across dimensions with position decay
			idx := (hash + uint32(j)) % uint32(e.dimensions)
			weight := 1.0 / (1.0 + position) // Earlier words weigh more
			embedding[idx] += weight
		}
	}

	// Normalize to unit vector
	magnitude := float32(0)
	for _, val := range embedding {
		magnitude += val * val
	}
	magnitude = float32(sqrt(float64(magnitude)))

	if magnitude > 0 {
		for i := range embedding {
			embedding[i] /= magnitude
		}
	}

	return embedding, nil
}

// GenerateBatch creates simple embeddings for multiple texts
func (e *SimpleEmbedding) GenerateBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.Generate(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = emb
	}
	return result, nil
}

// Dimensions returns the embedding vector dimensionality
func (e *SimpleEmbedding) Dimensions() int {
	return e.dimensions
}

// simpleHash computes a simple hash for a string
func simpleHash(s string) uint32 {
	hash := uint32(0)
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return hash
}

// sqrt computes square root (since math.Sqrt returns float64)
func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	// Newton's method for square root
	result := x
	for i := 0; i < 10; i++ {
		result = (result + x/result) / 2
	}
	return result
}
