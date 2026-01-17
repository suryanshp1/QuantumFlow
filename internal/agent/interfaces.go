package agent

import (
"context"
"time"

"github.com/quantumflow/quantumflow/internal/models"
)

// Agent represents a specialized AI agent for specific tasks
type Agent interface {
Name() string
Type() models.AgentType
Execute(ctx context.Context, request *Request) (*Response, error)
CanHandle(ctx context.Context, query string) (float64, error)
GetTools() []Tool
}

// Request represents a request sent to an agent
type Request struct {
ID          string
Query       string
Context     *Context
Memories    []*models.Memory
MaxTokens   int
Temperature float64
Timeout     time.Duration

// StreamCallback is called for each token during streaming generation
StreamCallback func(token string)
}

// Response represents an agent's response
type Response struct {
AgentName   string
AgentType   models.AgentType
Answer      string
ToolCalls   []models.ToolCall
Confidence  float64
Duration    time.Duration
TokensUsed  int
Metadata    map[string]interface{}
}

// Context contains contextual information for agent execution
type Context struct {
UserID        string
SessionID     string
CurrentDir    string
GitBranch     string
RecentCommits []string
OpenFiles     []string
Environment   map[string]string
Constraints   *Constraints
}

// Constraints defines execution boundaries for agents
type Constraints struct {
MaxToolCalls     int
AllowedTools     []string
DeniedTools      []string
MaxExecutionTime time.Duration
DryRun           bool
}

// Tool represents a capability available to agents
type Tool interface {
Name() string
Description() string
Execute(ctx context.Context, params map[string]interface{}) (string, error)
IsDestructive() bool
RequiresApproval() bool
}

// Orchestrator manages multiple agents and routes queries
type Orchestrator interface {
Route(ctx context.Context, query string, context *Context) ([]Agent, error)
Execute(ctx context.Context, request *Request) (*Response, error)
RegisterAgent(agent Agent) error
GetAgents() []Agent
}

// Classifier determines which agent type is best suited for a query
type Classifier interface {
Classify(ctx context.Context, query string) (models.AgentType, float64, error)
ClassifyMulti(ctx context.Context, query string, k int) ([]Classification, error)
}

// Classification represents a classification result
type Classification struct {
AgentType  models.AgentType
Confidence float64
Reasoning  string
}

// ConflictResolver handles contradictory outputs from multiple agents
type ConflictResolver interface {
Resolve(ctx context.Context, responses []*Response) (*Response, error)
DetectConflict(responses []*Response) bool
}

// SummaryPropagator creates concise summaries of agent outputs
type SummaryPropagator interface {
Summarize(ctx context.Context, response *Response, maxTokens int) (string, error)
Combine(ctx context.Context, summaries []string) (string, error)
}

// AgentConfig holds agent configuration
type AgentConfig struct {
Name            string
Type            models.AgentType
ModelName       string
ContextSize     int
Temperature     float64
MaxConcurrency  int
Tools           []Tool
MemoryEnabled   bool
MaxMemoryItems  int
}

// OrchestratorConfig holds orchestrator configuration
type OrchestratorConfig struct {
ClassifierType      string
ParallelExecution   bool
ConflictResolution  bool
SummaryPropagation  bool
MaxAgentsPerQuery   int
DefaultTimeout      time.Duration
}

// DefaultOrchestratorConfig returns default configuration
func DefaultOrchestratorConfig() *OrchestratorConfig {
return &OrchestratorConfig{
ClassifierType:     "rule-based",
ParallelExecution:  false,
ConflictResolution: true,
SummaryPropagation: true,
MaxAgentsPerQuery:  1,
DefaultTimeout:     5 * time.Minute,
}
}
