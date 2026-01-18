package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/quantumflow/quantumflow/internal/inference"
	"github.com/quantumflow/quantumflow/internal/memory"
	"github.com/quantumflow/quantumflow/internal/models"
)

// AgentOrchestrator manages multiple agents and routes queries
type AgentOrchestrator struct {
	agents     map[models.AgentType]Agent
	classifier Classifier
	resolver   ConflictResolver
	propagator SummaryPropagator
	memory     memory.Service
	config     *OrchestratorConfig
	mu         sync.RWMutex
}

// NewAgentOrchestrator creates a new agent orchestrator
func NewAgentOrchestrator(
	config *OrchestratorConfig,
	memoryService memory.Service,
	inferenceClient *inference.Client,
) *AgentOrchestrator {
	if config == nil {
		config = DefaultOrchestratorConfig()
	}

	orchestrator := &AgentOrchestrator{
		agents:     make(map[models.AgentType]Agent),
		classifier: NewQuantumRouter(inferenceClient),
		resolver:   NewSimpleConflictResolver(),
		propagator: NewQwenSummaryPropagator(inferenceClient),
		memory:     memoryService,
		config:     config,
	}

	return orchestrator
}

// RegisterAgent adds an agent to the orchestrator
func (o *AgentOrchestrator) RegisterAgent(agent Agent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if agent == nil {
		return fmt.Errorf("cannot register nil agent")
	}

	agentType := agent.Type()
	if _, exists := o.agents[agentType]; exists {
		return fmt.Errorf("agent of type %s already registered", agentType)
	}

	o.agents[agentType] = agent
	return nil
}

// GetAgents returns all registered agents
func (o *AgentOrchestrator) GetAgents() []Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()

	agents := make([]Agent, 0, len(o.agents))
	for _, agent := range o.agents {
		agents = append(agents, agent)
	}
	return agents
}

// Route determines which agent(s) should handle a query
func (o *AgentOrchestrator) Route(ctx context.Context, query string, context *Context) ([]Agent, error) {
	// Classify query
	agentType, confidence, err := o.classifier.Classify(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("classification failed: %w", err)
	}

	// Check if we have an agent for this type
	o.mu.RLock()
	agent, exists := o.agents[agentType]
	o.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no agent registered for type %s (confidence: %.2f)", agentType, confidence)
	}

	// For now, return single agent (non-parallel execution)
	return []Agent{agent}, nil
}

// Execute runs a query through the appropriate agent(s)
func (o *AgentOrchestrator) Execute(ctx context.Context, request *Request) (*Response, error) {
	start := time.Now()

	// Set default timeout if not specified
	if request.Timeout == 0 {
		request.Timeout = o.config.DefaultTimeout
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()

	// Retrieve relevant memories if needed
	if o.memory != nil && request.Memories == nil {
		memories, err := o.memory.Retrieve(execCtx, request.Query, 5)
		if err == nil {
			request.Memories = memories
		}
	}

	// Route to appropriate agent(s)
	agents, err := o.Route(execCtx, request.Query, request.Context)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available to handle query")
	}

	// Execute with agent(s)
	var responses []*Response

	if o.config.ParallelExecution && len(agents) > 1 {
		// Parallel execution (future enhancement)
		responses, err = o.executeParallel(execCtx, agents, request)
	} else {
		// Sequential execution (current implementation)
		responses, err = o.executeSequential(execCtx, agents, request)
	}

	if err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	// Handle multiple responses
	var finalResponse *Response
	if len(responses) == 1 {
		finalResponse = responses[0]
	} else if o.config.ConflictResolution && o.resolver.DetectConflict(responses) {
		// Resolve conflicts
		finalResponse, err = o.resolver.Resolve(execCtx, responses)
		if err != nil {
			return nil, fmt.Errorf("conflict resolution failed: %w", err)
		}
	} else {
		// Combine responses
		finalResponse = responses[0] // Use first response for now
	}

	// Add execution metadata
	finalResponse.Duration = time.Since(start)

	return finalResponse, nil
}

// executeSequential runs agents one at a time
func (o *AgentOrchestrator) executeSequential(ctx context.Context, agents []Agent, request *Request) ([]*Response, error) {
	responses := make([]*Response, 0, len(agents))

	for _, agent := range agents {
		response, err := agent.Execute(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("agent %s failed: %w", agent.Name(), err)
		}
		responses = append(responses, response)
	}

	return responses, nil
}

// executeParallel runs agents concurrently using goroutines
func (o *AgentOrchestrator) executeParallel(ctx context.Context, agents []Agent, request *Request) ([]*Response, error) {
	var wg sync.WaitGroup
	responses := make([]*Response, len(agents))
	errors := make([]error, len(agents))

	for i, agent := range agents {
		wg.Add(1)
		go func(idx int, a Agent) {
			defer wg.Done()
			resp, err := a.Execute(ctx, request)
			responses[idx] = resp
			errors[idx] = err
		}(i, agent)
	}

	wg.Wait()

	// Collect valid responses
	var validResponses []*Response
	var firstError error
	for i, resp := range responses {
		if errors[i] != nil && firstError == nil {
			firstError = errors[i]
		}
		if resp != nil {
			validResponses = append(validResponses, resp)
		}
	}

	if len(validResponses) == 0 {
		if firstError != nil {
			return nil, firstError
		}
		return nil, fmt.Errorf("all agents failed")
	}

	return validResponses, nil
}

// SimpleConflictResolver provides basic conflict resolution
type SimpleConflictResolver struct{}

// NewSimpleConflictResolver creates a new simple resolver
func NewSimpleConflictResolver() *SimpleConflictResolver {
	return &SimpleConflictResolver{}
}

// DetectConflict checks if responses contradict each other
func (r *SimpleConflictResolver) DetectConflict(responses []*Response) bool {
	// Simple heuristic: if confidence scores vary widely, there might be conflict
	if len(responses) < 2 {
		return false
	}

	minConf := responses[0].Confidence
	maxConf := responses[0].Confidence

	for _, resp := range responses[1:] {
		if resp.Confidence < minConf {
			minConf = resp.Confidence
		}
		if resp.Confidence > maxConf {
			maxConf = resp.Confidence
		}
	}

	// If variance > 0.3, consider it a conflict
	return (maxConf - minConf) > 0.3
}

// Resolve reconciles conflicting responses
func (r *SimpleConflictResolver) Resolve(ctx context.Context, responses []*Response) (*Response, error) {
	if len(responses) == 0 {
		return nil, fmt.Errorf("no responses to resolve")
	}

	// Simple strategy: pick the response with highest confidence
	bestResponse := responses[0]
	for _, resp := range responses[1:] {
		if resp.Confidence > bestResponse.Confidence {
			bestResponse = resp
		}
	}

	return bestResponse, nil
}

// QwenSummaryPropagator uses Qwen for summarization
type QwenSummaryPropagator struct {
	client *inference.Client
}

// NewQwenSummaryPropagator creates a new Qwen-based propagator
func NewQwenSummaryPropagator(client *inference.Client) *QwenSummaryPropagator {
	return &QwenSummaryPropagator{client: client}
}

// Summarize creates a concise summary
func (p *QwenSummaryPropagator) Summarize(ctx context.Context, response *Response, maxTokens int) (string, error) {
	prompt := fmt.Sprintf(`Summarize the following agent response in maximum %d tokens:

Agent: %s
Response: %s

Summary:`, maxTokens, response.AgentName, response.Answer)

	result, err := p.client.GenerateSync(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	return result.Response, nil
}

// Combine merges multiple summaries
func (p *QwenSummaryPropagator) Combine(ctx context.Context, summaries []string) (string, error) {
	if len(summaries) == 0 {
		return "", nil
	}
	if len(summaries) == 1 {
		return summaries[0], nil
	}

	prompt := fmt.Sprintf(`Combine the following summaries into a coherent response:

%s

Combined summary:`, joinSummaries(summaries))

	result, err := p.client.GenerateSync(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("combination failed: %w", err)
	}

	return result.Response, nil
}

// joinSummaries formats summaries for combination
func joinSummaries(summaries []string) string {
	result := ""
	for i, summary := range summaries {
		result += fmt.Sprintf("%d. %s\n\n", i+1, summary)
	}
	return result
}
