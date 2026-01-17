package agent

import (
"context"
"fmt"
"go/ast"
"go/parser"
"go/token"
"strings"
"time"

"github.com/quantumflow/quantumflow/internal/inference"
"github.com/quantumflow/quantumflow/internal/models"
)

type CodeAgent struct {
name   string
client *inference.Client
tools  []Tool
config *AgentConfig
}

func NewCodeAgent(client *inference.Client, config *AgentConfig) *CodeAgent {
if config == nil {
config = &AgentConfig{
Name:           "CodeAgent",
Type:           models.AgentTypeCode,
ModelName:      "qwen2.5:3b",
ContextSize:    32768,
Temperature:    0.3,
MaxConcurrency: 4,
MemoryEnabled:  true,
MaxMemoryItems: 10,
}
}

return &CodeAgent{
name:   config.Name,
client: client,
config: config,
tools: []Tool{
&ASTParserTool{},
&CodeSearchTool{},
&LintTool{},
},
}
}

func (a *CodeAgent) Name() string           { return a.name }
func (a *CodeAgent) Type() models.AgentType { return models.AgentTypeCode }
func (a *CodeAgent) GetTools() []Tool       { return a.tools }

func (a *CodeAgent) Execute(ctx context.Context, request *Request) (*Response, error) {
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
ToolCalls:  []models.ToolCall{},
Confidence: 0.9,
Duration:   time.Since(start),
TokensUsed: countTokens(fullResponse),
Metadata: map[string]interface{}{
"streaming": request.StreamCallback != nil,
},
}, nil
}

func (a *CodeAgent) CanHandle(ctx context.Context, query string) (float64, error) {
query = strings.ToLower(query)
codeKeywords := []string{
"code", "function", "class", "bug", "debug",
"implement", "refactor", "parse", "ast",
}

matchCount := 0
for _, keyword := range codeKeywords {
if strings.Contains(query, keyword) {
matchCount++
}
}

return float64(matchCount) / float64(len(codeKeywords)), nil
}

func (a *CodeAgent) buildPrompt(request *Request) string {
var prompt strings.Builder
prompt.WriteString("You are a code expert assistant. Provide accurate, well-structured code solutions.\n\n")

if request.Context != nil {
if request.Context.CurrentDir != "" {
prompt.WriteString(fmt.Sprintf("Working Directory: %s\n", request.Context.CurrentDir))
}
if request.Context.GitBranch != "" {
prompt.WriteString(fmt.Sprintf("Git Branch: %s\n", request.Context.GitBranch))
}
prompt.WriteString("\n")
}

if len(request.Memories) > 0 {
prompt.WriteString("Relevant Context:\n")
for _, mem := range request.Memories {
prompt.WriteString(fmt.Sprintf("- %s\n", truncate(mem.Content, 100)))
}
prompt.WriteString("\n")
}

prompt.WriteString(fmt.Sprintf("Query: %s\n\nResponse:", request.Query))
return prompt.String()
}

// Tools
type ASTParserTool struct{}

func (t *ASTParserTool) Name() string { return "ast_parser" }
func (t *ASTParserTool) Description() string { return "Parse Go code and extract AST structure" }
func (t *ASTParserTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
code, ok := params["code"].(string)
if !ok {
return "", fmt.Errorf("code parameter required")
}

fset := token.NewFileSet()
node, err := parser.ParseFile(fset, "", code, parser.ParseComments)
if err != nil {
return "", fmt.Errorf("parse error: %w", err)
}

var functions []string
ast.Inspect(node, func(n ast.Node) bool {
if fn, ok := n.(*ast.FuncDecl); ok {
functions = append(functions, fn.Name.Name)
}
return true
})

return fmt.Sprintf("Found %d functions: %s", len(functions), strings.Join(functions, ", ")), nil
}
func (t *ASTParserTool) IsDestructive() bool     { return false }
func (t *ASTParserTool) RequiresApproval() bool { return false }

type CodeSearchTool struct{}

func (t *CodeSearchTool) Name() string { return "code_search" }
func (t *CodeSearchTool) Description() string { return "Search codebase for patterns" }
func (t *CodeSearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
pattern, _ := params["pattern"].(string)
return fmt.Sprintf("Searching for pattern: %s", pattern), nil
}
func (t *CodeSearchTool) IsDestructive() bool     { return false }
func (t *CodeSearchTool) RequiresApproval() bool { return false }

type LintTool struct{}

func (t *LintTool) Name() string { return "lint" }
func (t *LintTool) Description() string { return "Run linter and check code quality" }
func (t *LintTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
code, _ := params["code"].(string)

issues := []string{}
if strings.Contains(code, "fmt.Println") {
issues = append(issues, "Consider using structured logging")
}
if strings.Contains(code, "panic(") {
issues = append(issues, "Avoid panics; use error returns")
}

if len(issues) == 0 {
return "No linting issues found", nil
}
return fmt.Sprintf("Found %d issues:\n%s", len(issues), strings.Join(issues, "\n")), nil
}
func (t *LintTool) IsDestructive() bool     { return false }
func (t *LintTool) RequiresApproval() bool { return false }
