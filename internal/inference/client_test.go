package inference

import (
	"context"
	"testing"
	"time"
)

// TestClientInitialization tests client creation with default and custom config
func TestClientInitialization(t *testing.T) {
	// Test default config
	client := NewClient(nil)
	if client == nil {
		t.Fatal("Expected client to be created with default config")
	}

	if client.config.OllamaURL != "http://localhost:11434" {
		t.Errorf("Expected default URL, got %s", client.config.OllamaURL)
	}

	// Test custom config
	customConfig := &Config{
		OllamaURL:   "http://custom:11434",
		Model:       "qwen2:72b",
		ContextSize: 65536,
		Temperature: 0.5,
		Timeout:     10 * time.Minute,
	}

	client = NewClient(customConfig)
	if client.config.Model != "qwen2:72b" {
		t.Errorf("Expected custom model, got %s", client.config.Model)
	}
}

// TestGenerateSync tests synchronous generation (requires running Ollama)
func TestGenerateSync(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.GenerateSync(ctx, "Say 'hello' and nothing else.")
	if err != nil {
		t.Logf("Skipping test - Ollama not available: %v", err)
		t.Skip()
	}

	if result.Response == "" {
		t.Error("Expected non-empty response")
	}

	if result.Latency == 0 {
		t.Error("Expected non-zero latency")
	}

	t.Logf("Response: %s", result.Response)
	t.Logf("Latency: %v", result.Latency)
	t.Logf("Tokens/sec: %.2f", result.TokensPerSec)
}

// TestListModels tests model listing (requires running Ollama)
func TestListModels(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil {
		t.Logf("Skipping test - Ollama not available: %v", err)
		t.Skip()
	}

	if len(models) == 0 {
		t.Log("No models available - please pull a model first")
	}

	for _, model := range models {
		t.Logf("Available model: %s", model)
	}
}

// BenchmarkGenerateSync benchmarks synchronous generation
func BenchmarkGenerateSync(b *testing.B) {
	client := NewClient(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.GenerateSync(ctx, "Count to 3")
		if err != nil {
			b.Logf("Skipping benchmark - error: %v", err)
			b.Skip()
		}
	}
}
