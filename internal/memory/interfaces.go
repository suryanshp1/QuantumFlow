package memory

import (
	"context"
	"time"

	"github.com/quantumflow/quantumflow/internal/models"
)

// Service is the main memory management service coordinating all memory stores
type Service interface {
	// Store persists an interaction to memory
	Store(ctx context.Context, interaction *models.Interaction) error

	// Retrieve fetches the top-k most relevant memories for a query
	Retrieve(ctx context.Context, query string, k int) ([]*models.Memory, error)

	// Compact runs memory compaction and deduplication
	Compact(ctx context.Context) error

	// Extract extracts facts and entities from text using Qwen
	Extract(ctx context.Context, text string) ([]Fact, error)

	// GetStats returns memory service statistics
	GetStats(ctx context.Context) (*Stats, error)

	// Close gracefully shuts down the memory service
	Close() error
}

// EpisodicStore handles conversation history storage (Redis)
type EpisodicStore interface {
	// Store stores a memory entry with vector embedding
	Store(ctx context.Context, memory *models.Memory) error

	// Search performs vector similarity search
	Search(ctx context.Context, embedding []float32, k int) ([]*models.Memory, error)

	// Delete removes a memory entry
	Delete(ctx context.Context, id string) error

	// Count returns total number of episodic memories
	Count(ctx context.Context) (int64, error)

	// Close closes the store connection
	Close() error
}

// SemanticStore handles knowledge graph storage (Dgraph)
type SemanticStore interface {
	// StoreEntity stores an entity in the knowledge graph
	StoreEntity(ctx context.Context, entity *models.Entity) error

	// StoreRelationship adds a relationship between entities
	StoreRelationship(ctx context.Context, rel *models.Relationship) error

	// QueryEntities finds entities matching criteria
	QueryEntities(ctx context.Context, query string) ([]*models.Entity, error)

	// Traverse performs graph traversal from a starting entity
	Traverse(ctx context.Context, startID string, depth int) ([]*models.Entity, error)

	// ResolveEntity finds or merges duplicate entities
	ResolveEntity(ctx context.Context, name string, entityType string) (*models.Entity, error)

	// Close closes the store connection
	Close() error
}

// ProceduralStore handles workflow pattern storage (BadgerDB)
type ProceduralStore interface {
	// StorePattern saves a workflow pattern
	StorePattern(ctx context.Context, pattern *models.WorkflowPattern) error

	// GetPattern retrieves a pattern by ID
	GetPattern(ctx context.Context, id string) (*models.WorkflowPattern, error)

	// FindSimilarPatterns finds patterns similar to given steps
	FindSimilarPatterns(ctx context.Context, steps []models.WorkflowStep, k int) ([]*models.WorkflowPattern, error)

	// UpdateFrequency increments pattern usage frequency
	UpdateFrequency(ctx context.Context, id string) error

	// GetTopPatterns returns most frequently used patterns
	GetTopPatterns(ctx context.Context, limit int) ([]*models.WorkflowPattern, error)

	// Close closes the store connection
	Close() error
}

// Extractor extracts structured information from text
type Extractor interface {
	// ExtractFacts extracts factual statements from text
	ExtractFacts(ctx context.Context, text string) ([]Fact, error)

	// ExtractEntities extracts named entities (people, places, things)
	ExtractEntities(ctx context.Context, text string) ([]*models.Entity, error)

	// ExtractRelationships identifies relationships between entities
	ExtractRelationships(ctx context.Context, text string) ([]*models.Relationship, error)

	// Summarize creates a concise summary of text
	Summarize(ctx context.Context, text string, maxTokens int) (string, error)
}

// Compactor handles memory compaction and deduplication
type Compactor interface {
	// Compact performs full memory compaction
	Compact(ctx context.Context) (*CompactionResult, error)

	// Deduplicate removes duplicate memories
	Deduplicate(ctx context.Context) (int, error)

	// Archive moves old memories to long-term storage
	Archive(ctx context.Context, olderThan time.Duration) (int, error)
}

// EmbeddingGenerator creates vector embeddings for text
type EmbeddingGenerator interface {
	// Generate creates an embedding vector for text
	Generate(ctx context.Context, text string) ([]float32, error)

	// GenerateBatch creates embeddings for multiple texts
	GenerateBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the embedding vector dimensionality
	Dimensions() int
}

// Fact represents an extracted factual statement
type Fact struct {
	ID         string                 `json:"id"`
	Statement  string                 `json:"statement"`
	Subject    string                 `json:"subject"`
	Predicate  string                 `json:"predicate"`
	Object     string                 `json:"object"`
	Confidence float64                `json:"confidence"`
	Source     string                 `json:"source"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// Stats contains memory service statistics
type Stats struct {
	EpisodicCount   int64         `json:"episodic_count"`
	SemanticCount   int64         `json:"semantic_count"`
	ProceduralCount int64         `json:"procedural_count"`
	TotalSize       int64         `json:"total_size_bytes"`
	LastCompaction  time.Time     `json:"last_compaction"`
	AvgRetrievalMs  float64       `json:"avg_retrieval_ms"`
	CacheHitRate    float64       `json:"cache_hit_rate"`
	Uptime          time.Duration `json:"uptime"`
}

// CompactionResult contains results from a compaction operation
type CompactionResult struct {
	MemoriesRemoved    int           `json:"memories_removed"`
	MemoriesCompacted  int           `json:"memories_compacted"`
	SpaceSavedBytes    int64         `json:"space_saved_bytes"`
	Duration           time.Duration `json:"duration"`
	DeduplicationCount int           `json:"deduplication_count"`
}

// Config holds memory service configuration
type Config struct {
	// Redis configuration
	RedisURL      string
	RedisPassword string
	RedisDB       int

	// Dgraph configuration
	DgraphURL      string
	DgraphAlphaURL string

	// BadgerDB configuration
	BadgerPath string

	// Compaction settings
	CompactionEnabled  bool
	CompactionInterval time.Duration
	RetentionDays      int

	// Embedding configuration
	EmbeddingDimensions int
	EmbeddingModel      string // "sentence-transformers/all-MiniLM-L6-v2"

	// Performance tuning
	CacheSize      int
	BatchSize      int
	MaxConcurrency int
}

// DefaultConfig returns default memory service configuration
func DefaultConfig() *Config {
	return &Config{
		RedisURL:            "localhost:6379",
		RedisPassword:       "quantumflow123",
		RedisDB:             0,
		DgraphURL:           "localhost:8080",
		DgraphAlphaURL:      "localhost:9080",
		BadgerPath:          "~/.quantumflow/badger",
		CompactionEnabled:   true,
		CompactionInterval:  1 * time.Hour,
		RetentionDays:       90,
		EmbeddingDimensions: 384, // MiniLM-L6-v2 dimensions
		EmbeddingModel:      "sentence-transformers/all-MiniLM-L6-v2",
		CacheSize:           10000,
		BatchSize:           32,
		MaxConcurrency:      8,
	}
}
