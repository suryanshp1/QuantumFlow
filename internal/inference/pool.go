package inference

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// Request represents an inference request with priority
type Request struct {
	ID       string
	Prompt   string
	Priority int                    // Higher = more important
	Callback func(*InferenceResult) // Called when completed
	Context  context.Context
}

// Pool manages a pool of workers for concurrent inference requests
type Pool struct {
	client    *Client
	workers   int
	queue     chan *Request
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	semaphore chan struct{} // Limits concurrent requests
	metrics   *PoolMetrics
	mu        sync.RWMutex
}

// PoolMetrics tracks pool performance
type PoolMetrics struct {
	TotalRequests   int64
	CompletedOK     int64
	CompletedError  int64
	AverageLatency  time.Duration
	TotalLatency    time.Duration
	CurrentInflight int
	mu              sync.RWMutex
}

// PoolConfig holds pool configuration
type PoolConfig struct {
	Workers          int // Number of worker goroutines
	QueueSize        int // Size of request queue
	MaxConcurrent    int // Maximum concurrent requests
	InferenceConfig  *Config
}

// DefaultPoolConfig returns default pool configuration
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		Workers:         runtime.NumCPU() * 2, // Scale with hardware
		QueueSize:       1000,                  // Reasonable queue size
		MaxConcurrent:   4,                     // Match typical Ollama defaults
		InferenceConfig: DefaultConfig(),
	}
}

// NewPool creates a new inference pool
func NewPool(config *PoolConfig) *Pool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &Pool{
		client:    NewClient(config.InferenceConfig),
		workers:   config.Workers,
		queue:     make(chan *Request, config.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
		semaphore: make(chan struct{}, config.MaxConcurrent),
		metrics:   &PoolMetrics{},
	}

	// Start workers
	for i := 0; i < pool.workers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

// worker processes requests from the queue
func (p *Pool) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case req, ok := <-p.queue:
			if !ok {
				return
			}
			p.processRequest(req)
		}
	}
}

// processRequest handles a single inference request
func (p *Pool) processRequest(req *Request) {
	// Acquire semaphore slot
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-req.Context.Done():
		// Request cancelled while waiting for semaphore
		if req.Callback != nil {
			req.Callback(&InferenceResult{
				Error: req.Context.Err(),
			})
		}
		return
	}

	// Update metrics
	p.metrics.mu.Lock()
	p.metrics.CurrentInflight++
	p.metrics.mu.Unlock()

	defer func() {
		p.metrics.mu.Lock()
		p.metrics.CurrentInflight--
		p.metrics.mu.Unlock()
	}()

	// Perform inference
	startTime := time.Now()
	result, err := p.client.GenerateSync(req.Context, req.Prompt)
	latency := time.Since(startTime)

	// Update result
	if result == nil {
		result = &InferenceResult{}
	}
	if err != nil {
		result.Error = err
	}
	result.Latency = latency

	// Update metrics
	p.updateMetrics(latency, err == nil)

	// Call callback
	if req.Callback != nil {
		req.Callback(result)
	}
}

// updateMetrics updates pool metrics
func (p *Pool) updateMetrics(latency time.Duration, success bool) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	p.metrics.TotalRequests++
	if success {
		p.metrics.CompletedOK++
	} else {
		p.metrics.CompletedError++
	}

	p.metrics.TotalLatency += latency
	if p.metrics.CompletedOK > 0 {
		p.metrics.AverageLatency = p.metrics.TotalLatency / time.Duration(p.metrics.CompletedOK)
	}
}

// Submit submits a request to the pool
func (p *Pool) Submit(req *Request) error {
	if req.Context == nil {
		req.Context = p.ctx
	}

	select {
	case p.queue <- req:
		return nil
	case <-req.Context.Done():
		return req.Context.Err()
	default:
		return fmt.Errorf("queue full")
	}
}

// SubmitSync submits a request and waits for the result
func (p *Pool) SubmitSync(ctx context.Context, prompt string, priority int) (*InferenceResult, error) {
	resultChan := make(chan *InferenceResult, 1)

	req := &Request{
		ID:       fmt.Sprintf("sync-%d", time.Now().UnixNano()),
		Prompt:   prompt,
		Priority: priority,
		Context:  ctx,
		Callback: func(result *InferenceResult) {
			resultChan <- result
		},
	}

	if err := p.Submit(req); err != nil {
		return nil, err
	}

	select {
	case result := <-resultChan:
		return result, result.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetMetrics returns current pool metrics
func (p *Pool) GetMetrics() PoolMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	return PoolMetrics{
		TotalRequests:   p.metrics.TotalRequests,
		CompletedOK:     p.metrics.CompletedOK,
		CompletedError:  p.metrics.CompletedError,
		AverageLatency:  p.metrics.AverageLatency,
		TotalLatency:    p.metrics.TotalLatency,
		CurrentInflight: p.metrics.CurrentInflight,
	}
}

// QueueLength returns the current queue length
func (p *Pool) QueueLength() int {
	return len(p.queue)
}

// Shutdown gracefully shuts down the pool
func (p *Pool) Shutdown(timeout time.Duration) error {
	// Stop accepting new requests
	close(p.queue)

	// Create timeout context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		return fmt.Errorf("shutdown timeout exceeded")
	}
}
