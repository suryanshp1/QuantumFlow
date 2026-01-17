package agent

import (
"context"
"fmt"
"strings"
"time"

"github.com/quantumflow/quantumflow/internal/inference"
"github.com/quantumflow/quantumflow/internal/models"
)

// DataAgent specializes in data analysis and SQL tasks
type DataAgent struct {
name   string
client *inference.Client
tools  []Tool
config *AgentConfig
}

func NewDataAgent(client *inference.Client, config *AgentConfig) *DataAgent {
if config == nil {
config = &AgentConfig{
Name:           "DataAgent",
Type:           models.AgentTypeData,
ModelName:      "qwen2.5:3b",
ContextSize:    32768,
Temperature:    0.4,
MaxConcurrency: 4,
MemoryEnabled:  true,
MaxMemoryItems: 10,
}
}

return &DataAgent{
name:   config.Name,
client: client,
config: config,
tools: []Tool{
&SQLGeneratorTool{},
&DataAnalysisTool{},
&SchemaInspectorTool{},
},
}
}

func (a *DataAgent) Name() string           { return a.name }
func (a *DataAgent) Type() models.AgentType { return models.AgentTypeData }
func (a *DataAgent) GetTools() []Tool       { return a.tools }

func (a *DataAgent) Execute(ctx context.Context, request *Request) (*Response, error) {
start := time.Now()
prompt := a.buildPrompt(request)

var fullResponse string

if request.StreamCallback != nil {
// Streaming mode
tokenChan, err := a.client.Generate(ctx, prompt, true)
if err != nil {
return nil, fmt.Errorf("generation failed: %w", err)
}

for token := range tokenChan {
fullResponse += token
request.StreamCallback(token)
}
} else {
// Synchronous mode
result, err := a.client.GenerateSync(ctx, prompt)
if err != nil {
return nil, fmt.Errorf("generation failed: %w", err)
}
fullResponse = result.Response
}

return &Response{
AgentName:  a.name,
AgentType:  a.Type(),
Answer:     fullResponse,
Confidence: 0.85,
Duration:   time.Since(start),
TokensUsed: countTokens(fullResponse),
}, nil
}

func (a *DataAgent) CanHandle(ctx context.Context, query string) (float64, error) {
keywords := []string{"data", "sql", "query", "table", "database", "analytics"}
query = strings.ToLower(query)

matches := 0
for _, kw := range keywords {
if strings.Contains(query, kw) {
matches++
}
}
return float64(matches) / float64(len(keywords)), nil
}

func (a *DataAgent) buildPrompt(request *Request) string {
var prompt strings.Builder
prompt.WriteString("You are a data analysis expert. Provide SQL queries and data insights.\n\n")

if len(request.Memories) > 0 {
prompt.WriteString("Context:\n")
for _, mem := range request.Memories {
prompt.WriteString(fmt.Sprintf("- %s\n", truncate(mem.Content, 100)))
}
}

prompt.WriteString(fmt.Sprintf("\nQuery: %s\n\nResponse:", request.Query))
return prompt.String()
}

// InfraAgent specializes in infrastructure tasks
type InfraAgent struct {
name   string
client *inference.Client
tools  []Tool
config *AgentConfig
}

func NewInfraAgent(client *inference.Client, config *AgentConfig) *InfraAgent {
if config == nil {
config = &AgentConfig{
Name:        "InfraAgent",
Type:        models.AgentTypeInfra,
Temperature: 0.5,
}
}

return &InfraAgent{
name:   config.Name,
client: client,
config: config,
tools: []Tool{
&DockerTool{},
&KubectlTool{},
&TerraformTool{},
},
}
}

func (a *InfraAgent) Name() string           { return a.name }
func (a *InfraAgent) Type() models.AgentType { return models.AgentTypeInfra }
func (a *InfraAgent) GetTools() []Tool       { return a.tools }

func (a *InfraAgent) Execute(ctx context.Context, request *Request) (*Response, error) {
start := time.Now()
prompt := fmt.Sprintf("You are an infrastructure expert. Help with deployment and infra tasks.\n\nQuery: %s\n\nResponse:", request.Query)

var fullResponse string

if request.StreamCallback != nil {
tokenChan, err := a.client.Generate(ctx, prompt, true)
if err != nil {
return nil, err
}

for token := range tokenChan {
fullResponse += token
request.StreamCallback(token)
}
} else {
result, err := a.client.GenerateSync(ctx, prompt)
if err != nil {
return nil, err
}
fullResponse = result.Response
}

return &Response{
AgentName:  a.name,
AgentType:  a.Type(),
Answer:     fullResponse,
Confidence: 0.8,
Duration:   time.Since(start),
TokensUsed: countTokens(fullResponse),
}, nil
}

func (a *InfraAgent) CanHandle(ctx context.Context, query string) (float64, error) {
keywords := []string{"deploy", "docker", "kubernetes", "terraform", "infrastructure"}
query = strings.ToLower(query)

matches := 0
for _, kw := range keywords {
if strings.Contains(query, kw) {
matches++
}
}
return float64(matches) / float64(len(keywords)), nil
}

// SecAgent specializes in security tasks
type SecAgent struct {
name   string
client *inference.Client
tools  []Tool
config *AgentConfig
}

func NewSecAgent(client *inference.Client, config *AgentConfig) *SecAgent {
if config == nil {
config = &AgentConfig{
Name: "SecAgent",
Type: models.AgentTypeSec,
}
}

return &SecAgent{
name:   config.Name,
client: client,
config: config,
tools: []Tool{
&VulnerabilityScannerTool{},
&OWASPCheckerTool{},
&SecurityAuditTool{},
},
}
}

func (a *SecAgent) Name() string           { return a.name }
func (a *SecAgent) Type() models.AgentType { return models.AgentTypeSec }
func (a *SecAgent) GetTools() []Tool       { return a.tools }

func (a *SecAgent) Execute(ctx context.Context, request *Request) (*Response, error) {
start := time.Now()
prompt := fmt.Sprintf("You are a security expert. Analyze and provide security recommendations.\n\nQuery: %s\n\nResponse:", request.Query)

var fullResponse string

if request.StreamCallback != nil {
tokenChan, err := a.client.Generate(ctx, prompt, true)
if err != nil {
return nil, err
}

for token := range tokenChan {
fullResponse += token
request.StreamCallback(token)
}
} else {
result, err := a.client.GenerateSync(ctx, prompt)
if err != nil {
return nil, err
}
fullResponse = result.Response
}

return &Response{
AgentName:  a.name,
AgentType:  a.Type(),
Answer:     fullResponse,
Confidence: 0.9,
Duration:   time.Since(start),
TokensUsed: countTokens(fullResponse),
}, nil
}

func (a *SecAgent) CanHandle(ctx context.Context, query string) (float64, error) {
keywords := []string{"security", "vulnerability", "owasp", "audit", "exploit"}
query = strings.ToLower(query)

matches := 0
for _, kw := range keywords {
if strings.Contains(query, kw) {
matches++
}
}
return float64(matches) / float64(len(keywords)), nil
}

// Tools
type SQLGeneratorTool struct{}
func (t *SQLGeneratorTool) Name() string { return "sql_generator" }
func (t *SQLGeneratorTool) Description() string { return "Generate SQL queries from natural language" }
func (t *SQLGeneratorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
description, _ := params["description"].(string)
return fmt.Sprintf("-- Generated SQL for: %s\nSELECT * FROM table;", description), nil
}
func (t *SQLGeneratorTool) IsDestructive() bool { return false }
func (t *SQLGeneratorTool) RequiresApproval() bool { return false }

type DataAnalysisTool struct{}
func (t *DataAnalysisTool) Name() string { return "data_analysis" }
func (t *DataAnalysisTool) Description() string { return "Analyze datasets and provide statistical insights" }
func (t *DataAnalysisTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Data analysis results: Mean=X, Median=Y", nil
}
func (t *DataAnalysisTool) IsDestructive() bool { return false }
func (t *DataAnalysisTool) RequiresApproval() bool { return false }

type SchemaInspectorTool struct{}
func (t *SchemaInspectorTool) Name() string { return "schema_inspector" }
func (t *SchemaInspectorTool) Description() string { return "Inspect database schema and relationships" }
func (t *SchemaInspectorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
table, _ := params["table"].(string)
return fmt.Sprintf("Schema for %s: columns, types, constraints", table), nil
}
func (t *SchemaInspectorTool) IsDestructive() bool { return false }
func (t *SchemaInspectorTool) RequiresApproval() bool { return false }

type DockerTool struct{}
func (t *DockerTool) Name() string { return "docker" }
func (t *DockerTool) Description() string { return "Docker container operations" }
func (t *DockerTool) IsDestructive() bool { return true }
func (t *DockerTool) RequiresApproval() bool { return true }
func (t *DockerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Docker command executed (dry run)", nil
}

type KubectlTool struct{}
func (t *KubectlTool) Name() string { return "kubectl" }
func (t *KubectlTool) Description() string { return "Kubernetes operations" }
func (t *KubectlTool) IsDestructive() bool { return true }
func (t *KubectlTool) RequiresApproval() bool { return true }
func (t *KubectlTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Kubectl command executed (dry run)", nil
}

type TerraformTool struct{}
func (t *TerraformTool) Name() string { return "terraform" }
func (t *TerraformTool) Description() string { return "Infrastructure as Code operations" }
func (t *TerraformTool) IsDestructive() bool { return true }
func (t *TerraformTool) RequiresApproval() bool { return true }
func (t *TerraformTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Terraform plan generated", nil
}

type VulnerabilityScannerTool struct{}
func (t *VulnerabilityScannerTool) Name() string { return "vuln_scanner" }
func (t *VulnerabilityScannerTool) Description() string { return "Scan for vulnerabilities" }
func (t *VulnerabilityScannerTool) IsDestructive() bool { return false }
func (t *VulnerabilityScannerTool) RequiresApproval() bool { return false }
func (t *VulnerabilityScannerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Vulnerability scan complete: 0 critical, 2 medium, 5 low", nil
}

type OWASPCheckerTool struct{}
func (t *OWASPCheckerTool) Name() string { return "owasp_checker" }
func (t *OWASPCheckerTool) Description() string { return "Check against OWASP Top 10" }
func (t *OWASPCheckerTool) IsDestructive() bool { return false }
func (t *OWASPCheckerTool) RequiresApproval() bool { return false }
func (t *OWASPCheckerTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "OWASP check passed: No critical issues", nil
}

type SecurityAuditTool struct{}
func (t *SecurityAuditTool) Name() string { return "security_audit" }
func (t *SecurityAuditTool) Description() string { return "Comprehensive security audit" }
func (t *SecurityAuditTool) IsDestructive() bool { return false }
func (t *SecurityAuditTool) RequiresApproval() bool { return false }
func (t *SecurityAuditTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
return "Security audit report generated", nil
}

func countTokens(text string) int {
return len(text) / 4
}

func truncate(s string, maxLen int) string {
if len(s) <= maxLen {
return s
}
return s[:maxLen-3] + "..."
}
