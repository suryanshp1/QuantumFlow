package agent

import (
"context"
"encoding/json"
"fmt"
"strings"

"github.com/quantumflow/quantumflow/internal/inference"
"github.com/quantumflow/quantumflow/internal/models"
)

// QuantumRouter uses LLM-based reasoning to route queries to appropriate agents
type QuantumRouter struct {
client *inference.Client
}

// NewQuantumRouter creates a new LLM-based router
func NewQuantumRouter(client *inference.Client) *QuantumRouter {
return &QuantumRouter{
client: client,
}
}

// RoutingDecision represents the LLM's routing decision
type RoutingDecision struct {
PrimaryAgent   string  `json:"primary_agent"`
Confidence     float64 `json:"confidence"`
Reasoning      string  `json:"reasoning"`
SecondaryAgent string  `json:"secondary_agent,omitempty"`
ToolsNeeded    []string `json:"tools_needed,omitempty"`
}

// Classify uses LLM to intelligently route queries
func (r *QuantumRouter) Classify(ctx context.Context, query string) (models.AgentType, float64, error) {
prompt := r.buildRoutingPrompt(query)

result, err := r.client.GenerateSync(ctx, prompt)
if err != nil {
return "", 0, fmt.Errorf("routing failed: %w", err)
}

var decision RoutingDecision
if err := r.parseRoutingResponse(result.Response, &decision); err != nil {
return "", 0, fmt.Errorf("failed to parse routing decision: %w", err)
}

// Normalize agent type from LLM response (includes fallback)
agentType := normalizeAgentType(decision.PrimaryAgent)

return agentType, decision.Confidence, nil
}

// ClassifyMulti returns top-k agent classifications
func (r *QuantumRouter) ClassifyMulti(ctx context.Context, query string, k int) ([]Classification, error) {
agentType, confidence, err := r.Classify(ctx, query)
if err != nil {
return nil, err
}

return []Classification{
{
AgentType:  agentType,
Confidence: confidence,
Reasoning:  fmt.Sprintf("Selected %s via LLM routing", agentType),
},
}, nil
}

// buildRoutingPrompt creates the prompt for LLM-based routing
func (r *QuantumRouter) buildRoutingPrompt(query string) string {
return fmt.Sprintf(`You are an intelligent routing system for a multi-agent AI coding assistant.

Available agents:
- code: Code generation, debugging, refactoring, AST parsing, linting, programming questions
- data: SQL queries, database schema, data analysis, migrations, analytics
- infra: Docker, Kubernetes, Terraform, deployment, DevOps, cloud infrastructure
- sec: Security audits, vulnerabilities, OWASP, authentication, encryption

User Query: %s

IMPORTANT RULES:
- If the query is a GENERAL question, greeting, or help request → use "code" agent
- If the query asks "what can you do?" or "introduce yourself" → use "code" agent
- For conversational/unclear queries → use "code" agent with low confidence
- NEVER return "none", "general", or any value other than: code, data, infra, sec
- You MUST choose one of the four agents above

Respond with ONLY a JSON object:
{
  "primary_agent": "code|data|infra|sec",
  "confidence": 0.0-1.0,
  "reasoning": "brief explanation"
}

JSON Response:`, query)
}

// parseRoutingResponse extracts the routing decision from LLM output
func (r *QuantumRouter) parseRoutingResponse(response string, decision *RoutingDecision) error {
response = strings.TrimSpace(response)

// Remove markdown code blocks
if strings.HasPrefix(response, "```json") {
response = strings.TrimPrefix(response, "```json")
response = strings.TrimSuffix(response, "```")
response = strings.TrimSpace(response)
} else if strings.HasPrefix(response, "```") {
response = strings.TrimPrefix(response, "```")
response = strings.TrimSuffix(response, "```")
response = strings.TrimSpace(response)
}

// Find JSON object
start := strings.Index(response, "{")
end := strings.LastIndex(response, "}")

if start == -1 || end == -1 {
return fmt.Errorf("no JSON object found in response")
}

jsonStr := response[start : end+1]

if err := json.Unmarshal([]byte(jsonStr), decision); err != nil {
return fmt.Errorf("JSON parse error: %w", err)
}

// Clamp confidence
if decision.Confidence < 0 {
decision.Confidence = 0
}
if decision.Confidence > 1 {
decision.Confidence = 1
}

return nil
}

// normalizeAgentType converts LLM output to proper agent type constant
// Includes fallback to CodeAgent for safety
func normalizeAgentType(agentStr string) models.AgentType {
normalized := strings.ToLower(strings.TrimSpace(agentStr))

switch normalized {
case "code", "codeagent":
return models.AgentTypeCode
case "data", "dataagent":
return models.AgentTypeData
case "infra", "infraagent":
return models.AgentTypeInfra
case "sec", "secagent", "security":
return models.AgentTypeSec
default:
// Fallback to CodeAgent for:
// - Invalid types ("none", "general", etc.)
// - Conversational queries
// - Unknown/malformed responses
return models.AgentTypeCode
}
}
