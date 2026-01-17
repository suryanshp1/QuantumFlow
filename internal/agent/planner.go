package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quantumflow/quantumflow/internal/inference"
)

// Planner generates execution plans for complex queries
type Planner struct {
	client *inference.Client
}

// NewPlanner creates a new plan generator
func NewPlanner(client *inference.Client) *Planner {
	return &Planner{
		client: client,
	}
}

// Generate creates an execution plan for a complex query
func (p *Planner) Generate(ctx context.Context, req *PlanGenerationRequest) (*ExecutionPlan, error) {
	prompt := p.buildPlanningPrompt(req)
	
	// Generate plan using LLM
	result, err := p.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	// Parse LLM response into structured plan
	plan, err := p.parsePlanResponse(result.Response, req.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	// Set metadata
	plan.ID = generatePlanID()
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()
	plan.State = ExecutionState{
		Status: ExecutionStatusPending,
	}

	return plan, nil
}

// buildPlanningPrompt creates the LLM prompt for plan generation
func (p *Planner) buildPlanningPrompt(req *PlanGenerationRequest) string {
	contextInfo := ""
	if req.Context != nil {
		contextInfo = fmt.Sprintf(`
Project Context:
- Current Directory: %s
- Recent Activity: Working on a coding project
`, req.Context.CurrentDir)
	}

	return fmt.Sprintf(`You are a senior software architect planning a complex software development task.

User Request: %s
%s
Task: Generate a detailed, phased implementation plan.

IMPORTANT RULES:
1. Break down into 2-7 logical phases
2. Each phase should be completable in 5-30 minutes
3. Assign appropriate agent: code, data, infra, or sec
4. Include specific, actionable tasks
5. Define clear success criteria
6. Estimate realistic time

Respond with ONLY a JSON object in this EXACT format:
{
  "title": "Brief plan title",
  "description": "One-sentence summary of what will be built",
  "phases": [
    {
      "name": "Phase name",
      "agent": "code|data|infra|sec",
      "tasks": [
        {"description": "Specific task 1"},
        {"description": "Specific task 2"}
      ],
      "success_criteria": "How to verify this phase succeeded",
      "estimated_time": "5-10 minutes",
      "dependencies": []
    }
  ]
}

Guidelines for phases:
- Phase 1: Usually setup/design/schema
- Middle phases: Core implementation
- Final phase: Testing and verification
- Use "code" agent for general programming tasks
- Use "data" agent for database/SQL work
- Use "infra" agent for deployment/Docker/K8s
- Use "sec" agent for security audits

JSON Response:`, req.Query, contextInfo)
}

// parsePlanResponse extracts the execution plan from LLM output
func (p *Planner) parsePlanResponse(response string, query string) (*ExecutionPlan, error) {
	response = strings.TrimSpace(response)
	
	// Remove markdown code blocks if present
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
		return nil, fmt.Errorf("no JSON object found in response")
	}

	jsonStr := response[start : end+1]
	
	// Parse into temporary structure
	var rawPlan struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Phases      []struct {
			Name            string   `json:"name"`
			Agent           string   `json:"agent"`
			Tasks           []struct {
				Description string `json:"description"`
			} `json:"tasks"`
			SuccessCriteria string   `json:"success_criteria"`
			EstimatedTime   string   `json:"estimated_time"`
			Dependencies    []string `json:"dependencies"`
		} `json:"phases"`
	}
	
	if err := json.Unmarshal([]byte(jsonStr), &rawPlan); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	// Convert to ExecutionPlan
	plan := &ExecutionPlan{
		Title:       rawPlan.Title,
		Description: rawPlan.Description,
		Phases:      make([]Phase, len(rawPlan.Phases)),
	}

	// Convert phases
	for i, rawPhase := range rawPlan.Phases {
		// Normalize agent type
		agent := normalizeAgentType(rawPhase.Agent)
		
		// Convert tasks
		tasks := make([]Task, len(rawPhase.Tasks))
		for j, rawTask := range rawPhase.Tasks {
			tasks[j] = Task{
				ID:          fmt.Sprintf("task-%d-%d", i+1, j+1),
				Description: rawTask.Description,
				Completed:   false,
			}
		}

		plan.Phases[i] = Phase{
			ID:              fmt.Sprintf("phase-%d", i+1),
			Name:            rawPhase.Name,
			Agent:           agent,
			Tasks:           tasks,
			SuccessCriteria: rawPhase.SuccessCriteria,
			EstimatedTime:   rawPhase.EstimatedTime,
			Dependencies:    rawPhase.Dependencies,
			Status:          PhaseStatusPending,
		}
	}

	return plan, nil
}

// FormatAsMarkdown converts an execution plan to markdown format
func (p *Planner) FormatAsMarkdown(plan *ExecutionPlan) string {
	var md strings.Builder
	
	md.WriteString(fmt.Sprintf("# %s\n\n", plan.Title))
	md.WriteString(fmt.Sprintf("%s\n\n", plan.Description))
	md.WriteString(fmt.Sprintf("**Plan ID:** %s  \n", plan.ID))
	md.WriteString(fmt.Sprintf("**Created:** %s  \n\n", plan.CreatedAt.Format("2006-01-02 15:04:05")))
	
	md.WriteString("---\n\n")
	
	for i, phase := range plan.Phases {
		md.WriteString(fmt.Sprintf("## Phase %d: %s\n\n", i+1, phase.Name))
		md.WriteString(fmt.Sprintf("**Agent:** %s  \n", phase.Agent))
		md.WriteString(fmt.Sprintf("**Estimated Time:** %s  \n\n", phase.EstimatedTime))
		
		md.WriteString("**Tasks:**\n")
		for _, task := range phase.Tasks {
			md.WriteString(fmt.Sprintf("- [ ] %s\n", task.Description))
		}
		md.WriteString("\n")
		
		md.WriteString(fmt.Sprintf("**Success Criteria:**  \n%s\n\n", phase.SuccessCriteria))
		
		if len(phase.Dependencies) > 0 {
			md.WriteString(fmt.Sprintf("**Dependencies:** %s\n\n", strings.Join(phase.Dependencies, ", ")))
		}
		
		md.WriteString("---\n\n")
	}
	
	return md.String()
}

// generatePlanID creates a unique plan ID
func generatePlanID() string {
	return fmt.Sprintf("plan_%s", time.Now().Format("20060102_150405"))
}
