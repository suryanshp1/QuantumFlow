package inference

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPoolCreation tests pool initialization
func TestPoolCreation(t *testing.T) {
	pool := NewPool(nil)
	if pool == nil {
		t.Fatal("Expected pool to be created")
	}

	if pool.workers != 100 {
		t.Errorf("Expected 100 workers, got %d", pool.workers)
	}

	// Cleanup
	pool.Shutdown(5 * time.Second)
}

// TestPoolSubmit tests request submission
func TestPoolSubmit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pool := NewPool(DefaultPoolConfig())
	defer pool.Shutdown(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := pool.SubmitSync(ctx, "Say hello", 1)
	if err != nil {
		t.Logf("Skipping test - Ollama not available: %v", err)
		t.Skip()
	}

	if result.Response == "" {
		t.Error("Expected non-empty response")
	}

	t.Logf("Response: %s", result.Response)
	t.Logf("Latency: %v", result.Latency)
}

// TestPoolConcurrency tests concurrent request handling
func TestPoolConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pool := NewPool(DefaultPoolConfig())
	defer pool.Shutdown(10 * time.Second)

	numRequests := 10
	var wg sync.WaitGroup
	wg.Add(numRequests)

	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := pool.SubmitSync(ctx, "Count to 3", 1)
			if err != nil {
				t.Logf("Request %d failed: %v", id, err)
				return
			}

			if result.Response != "" {
				mu.Lock()
				successCount++
				mu.Unlock()
			}

			t.Logf("Request %d completed in %v", id, result.Latency)
		}(i)
	}

	wg.Wait()

	if successCount == 0 {
		t.Skip("Skipping test - Ollama not available")
	}

	t.Logf("Completed %d/%d requests successfully", successCount, numRequests)
}

// TestPoolMetrics tests metrics tracking
func TestPoolMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pool := NewPool(DefaultPoolConfig())
	defer pool.Shutdown(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.SubmitSync(ctx, "Say hello", 1)
	if err != nil {
		t.Skip("Skipping test - Ollama not available")
	}

	metrics := pool.GetMetrics()
	if metrics.TotalRequests == 0 {
		t.Error("Expected non-zero total requests")
	}

	if metrics.CompletedOK == 0 {
		t.Error("Expected at least one successful request")
	}

	t.Logf("Total Requests: %d", metrics.TotalRequests)
	t.Logf("Completed OK: %d", metrics.CompletedOK)
	t.Logf("Average Latency: %v", metrics.AverageLatency)
}

// BenchmarkPoolThroughput benchmarks pool throughput
func BenchmarkPoolThroughput(b *testing.B) {
	pool := NewPool(DefaultPoolConfig())
	defer pool.Shutdown(30 * time.Second)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := pool.SubmitSync(ctx, "Count to 3", 1)
			if err != nil {
				b.Logf("Error: %v", err)
				b.Skip()
			}
		}
	})

	metrics := pool.GetMetrics()
	b.Logf("Total requests: %d", metrics.TotalRequests)
	b.Logf("Average latency: %v", metrics.AverageLatency)
}
