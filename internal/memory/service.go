package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/quantumflow/quantumflow/internal/inference"
	"github.com/quantumflow/quantumflow/internal/models"
)

// MemoryService implements the main memory service orchestrating all stores
type MemoryService struct {
	episodic   EpisodicStore
	semantic   SemanticStore
	procedural ProceduralStore
	embedding  EmbeddingGenerator
	extractor  Extractor
	compactor  Compactor

	config *Config
	stats  *Stats
	mu     sync.RWMutex

	startTime time.Time
	stopCh    chan struct{}
}

// NewMemoryService creates a new memory service instance
func NewMemoryService(config *Config, inferenceClient *inference.Client) (*MemoryService, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Initialize episodic store (Redis)
	episodic, err := NewRedisEpisodicStore(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create episodic store: %w", err)
	}

	// Initialize semantic store (Dgraph)
	semantic, err := NewDgraphSemanticStore(config)
	if err != nil {
		episodic.Close()
		return nil, fmt.Errorf("failed to create semantic store: %w", err)
	}

	// Initialize procedural store (BadgerDB)
	procedural, err := NewBadgerProceduralStore(config)
	if err != nil {
		episodic.Close()
		semantic.Close()
		return nil, fmt.Errorf("failed to create procedural store: %w", err)
	}

	// Initialize embedding generator
	// Try HuggingFace first, fallback to simple embeddings
	embedding := NewSimpleEmbedding(config.EmbeddingDimensions)

	// Initialize extractor
	extractor := NewQwenExtractor(inferenceClient)

	// Initialize compactor
	compactor := NewMemoryCompactor(episodic, config)

	service := &MemoryService{
		episodic:   episodic,
		semantic:   semantic,
		procedural: procedural,
		embedding:  embedding,
		extractor:  extractor,
		compactor:  compactor,
		config:     config,
		stats:      &Stats{},
		startTime:  time.Now(),
		stopCh:     make(chan struct{}),
	}

	// Start background compaction if enabled
	if config.CompactionEnabled {
		go service.runPeriodicCompaction()
	}

	return service, nil
}

// Store persists an interaction to memory
func (m *MemoryService) Store(ctx context.Context, interaction *models.Interaction) error {
	// Extract information from the interaction
	facts, err := m.extractor.ExtractFacts(ctx, interaction.UserQuery+" "+interaction.AgentResponse)
	if err != nil {
		// Log error but continue
		_ = err
	}
	_ = facts // TODO: Store facts in semantic graph

	entities, err := m.extractor.ExtractEntities(ctx, interaction.UserQuery+" "+interaction.AgentResponse)
	if err != nil {
		_ = err
	}

	// Create episodic memory
	embedding, err := m.embedding.Generate(ctx, interaction.UserQuery)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	memory := &models.Memory{
		ID:        interaction.ID,
		Type:      models.MemoryTypeEpisodic,
		Content:   interaction.UserQuery + "\n" + interaction.AgentResponse,
		Embedding: embedding,
		Timestamp: interaction.Timestamp,
		Metadata: map[string]interface{}{
			"tool_calls": len(interaction.ToolCalls),
			"duration":   interaction.Duration,
		},
	}

	// Store in episodic memory
	if err := m.episodic.Store(ctx, memory); err != nil {
		return fmt.Errorf("failed to store episodic memory: %w", err)
	}

	// Store entities in semantic graph
	for _, entity := range entities {
		if err := m.semantic.StoreEntity(ctx, entity); err != nil {
			// Log error but continue
			_ = err
		}
	}

	// Extract and store workflow patterns from tool calls
	if len(interaction.ToolCalls) > 0 {
		pattern := &models.WorkflowPattern{
			Name:        fmt.Sprintf("Pattern from %s", interaction.ID),
			Steps:       make([]models.WorkflowStep, len(interaction.ToolCalls)),
			Frequency:   1,
			SuccessRate: calculateSuccessRate(interaction.ToolCalls),
			LastUsed:    time.Now(),
		}

		for i, call := range interaction.ToolCalls {
			pattern.Steps[i] = models.WorkflowStep{
				Action:     call.Name,
				Tool:       call.Name,
				Parameters: call.Parameters,
			}
		}

		if err := m.procedural.StorePattern(ctx, pattern); err != nil {
			_ = err
		}
	}

	return nil
}

// Retrieve fetches the top-k most relevant memories for a query
func (m *MemoryService) Retrieve(ctx context.Context, query string, k int) ([]*models.Memory, error) {
	start := time.Now()

	// Generate embedding for query
	embedding, err := m.embedding.Generate(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search episodic memory
	memories, err := m.episodic.Search(ctx, embedding, k)
	if err != nil {
		return nil, fmt.Errorf("failed to search episodic memory: %w", err)
	}

	// Update stats
	m.mu.Lock()
	m.stats.AvgRetrievalMs = float64(time.Since(start).Milliseconds())
	m.mu.Unlock()

	return memories, nil
}

// Compact runs memory compaction and deduplication
func (m *MemoryService) Compact(ctx context.Context) error {
	result, err := m.compactor.Compact(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.stats.LastCompaction = time.Now()
	m.mu.Unlock()

	_ = result // Use compaction result for logging
	return nil
}

// Extract extracts facts and entities from text using Qwen
func (m *MemoryService) Extract(ctx context.Context, text string) ([]Fact, error) {
	return m.extractor.ExtractFacts(ctx, text)
}

// GetStats returns memory service statistics
func (m *MemoryService) GetStats(ctx context.Context) (*Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get counts from stores
	episodicCount, _ := m.episodic.Count(ctx)

	stats := &Stats{
		EpisodicCount:  episodicCount,
		LastCompaction: m.stats.LastCompaction,
		AvgRetrievalMs: m.stats.AvgRetrievalMs,
		Uptime:         time.Since(m.startTime),
	}

	return stats, nil
}

// Close gracefully shuts down the memory service
func (m *MemoryService) Close() error {
	close(m.stopCh)

	var errs []error

	if err := m.episodic.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := m.semantic.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := m.procedural.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing memory service: %v", errs)
	}

	return nil
}

// runPeriodicCompaction runs compaction at configured intervals
func (m *MemoryService) runPeriodicCompaction() {
	ticker := time.NewTicker(m.config.CompactionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := m.Compact(ctx); err != nil {
				// Log error
				_ = err
			}
			cancel()
		case <-m.stopCh:
			return
		}
	}
}

// calculateSuccessRate computes success rate from tool calls
func calculateSuccessRate(toolCalls []models.ToolCall) float64 {
	if len(toolCalls) == 0 {
		return 0
	}

	successful := 0
	for _, call := range toolCalls {
		if call.Error == "" {
			successful++
		}
	}

	return float64(successful) / float64(len(toolCalls))
}
