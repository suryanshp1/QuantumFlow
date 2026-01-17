package models

import "time"

// Message represents a single message in a conversation
type Message struct {
	Role      string                 `json:"role"`      // "user", "assistant", "system"
	Content   string                 `json:"content"`   // Message content
	Metadata  map[string]interface{} `json:"metadata"`  // Additional metadata
	Timestamp time.Time              `json:"timestamp"` // When the message was created
}

// Memory represents a stored memory entry
type Memory struct {
	ID        string                 `json:"id"`
	Type      MemoryType             `json:"type"`      // Episodic, Semantic, Procedural
	Content   string                 `json:"content"`   // Memory content
	Embedding []float32              `json:"embedding"` // 768-dim vector
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
	Score     float64                `json:"score"` // Relevance score
}

// MemoryType defines the type of memory
type MemoryType string

const (
	MemoryTypeEpisodic   MemoryType = "episodic"
	MemoryTypeSemantic   MemoryType = "semantic"
	MemoryTypeProcedural MemoryType = "procedural"
)

// Interaction represents a complete user-agent interaction
type Interaction struct {
	ID            string     `json:"id"`
	UserQuery     string     `json:"user_query"`
	AgentResponse string     `json:"agent_response"`
	ToolCalls     []ToolCall `json:"tool_calls"`
	Timestamp     time.Time  `json:"timestamp"`
	Duration      float64    `json:"duration"` // seconds
}

// ToolCall represents a tool invocation
type ToolCall struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
	Result     string                 `json:"result"`
	Error      string                 `json:"error,omitempty"`
	Duration   float64                `json:"duration"` // seconds
}

// AgentType defines the type of specialized agent
type AgentType string

const (
	AgentTypeCode  AgentType = "code"
	AgentTypeData  AgentType = "data"
	AgentTypeInfra AgentType = "infra"
	AgentTypeSec   AgentType = "sec"
)

// WorkflowPattern represents a stored workflow pattern
type WorkflowPattern struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Steps       []WorkflowStep `json:"steps"`
	Frequency   int            `json:"frequency"`
	SuccessRate float64        `json:"success_rate"`
	LastUsed    time.Time      `json:"last_used"`
}

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	Action     string                 `json:"action"`
	Tool       string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters"`
}

// Entity represents a semantic entity in the knowledge graph
type Entity struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Attributes    map[string]interface{} `json:"attributes"`
	Relationships []Relationship         `json:"relationships"`
}

// Relationship represents a relationship between entities
type Relationship struct {
	ID         string  `json:"id"`
	FromID     string  `json:"from_id"`
	ToID       string  `json:"to_id"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}
