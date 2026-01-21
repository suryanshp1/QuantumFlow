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

// Generate creates an execution plan using two-stage hierarchical planning
// Stage 1: Generate file structure (minimal tokens)
// Stage 2: Generate phases (compact prompt)
func (p *Planner) Generate(ctx context.Context, req *PlanGenerationRequest) (*ExecutionPlan, error) {
	// Stage 1: Generate file structure first (small, focused prompt ~2k tokens)
	fmt.Println("📐 Stage 1: Generating file structure...")
	fileStructure, err := p.generateFileStructure(ctx, req.Query)
	if err != nil {
		// Fall back to empty structure if stage 1 fails
		fmt.Printf("⚠️ File structure generation failed, continuing without: %v\n", err)
		fileStructure = make(map[string][]string)
	}
	
	// Stage 2: Generate phases with compact prompt (~3k tokens)
	fmt.Println("📋 Stage 2: Generating execution phases...")
	plan, err := p.generatePhasesCompact(ctx, req.Query, fileStructure)
	if err != nil {
		return nil, fmt.Errorf("phase generation failed: %w", err)
	}

	// Set metadata
	plan.ID = generatePlanID()
	plan.FileStructure = fileStructure
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()
	plan.State = ExecutionState{
		Status: ExecutionStatusPending,
	}

	return plan, nil
}

// generateFileStructure creates a minimal prompt to get just the file structure
// This is Stage 1 of hierarchical planning (~2k tokens)
func (p *Planner) generateFileStructure(ctx context.Context, query string) (map[string][]string, error) {
	prompt := fmt.Sprintf(`Generate ONLY a file structure for this project:
"%s"

Step 1: Choose a short snake_case project name (e.g. ecommerce_api, chat_bot).
Step 2: ALL files must be inside a single root directory with that name.

Output ONLY valid JSON in this exact format:
{"dirs":{"{{project_name}}/":["main.py","README.md"],"{{project_name}}/src/":["api.py"]}}

JSON:`, query)

	result, err := p.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Parse response
	response := strings.TrimSpace(result.Response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON in response")
	}

	var parsed struct {
		Dirs map[string][]string `json:"dirs"`
	}
	if err := json.Unmarshal([]byte(response[start:end+1]), &parsed); err != nil {
		return nil, err
	}

	return parsed.Dirs, nil
}

// generatePhasesCompact creates phases using a minimal prompt
// This is Stage 2 of hierarchical planning (~3k tokens)
func (p *Planner) generatePhasesCompact(ctx context.Context, query string, fileStructure map[string][]string) (*ExecutionPlan, error) {
	// Count files for context
	fileCount := 0
	for _, files := range fileStructure {
		fileCount += len(files)
	}

	// Get project root from structure to guide phase tasks
	var projectRoot string
	for dir := range fileStructure {
		parts := strings.Split(dir, "/")
		if len(parts) > 0 && parts[0] != "" {
			if projectRoot == "" {
				projectRoot = parts[0]
			} else if projectRoot != parts[0] {
				// Detect multiple roots, but stick to first one found for prompt hint
			}
		}
	}

	prompt := fmt.Sprintf(`Create build plan for: %s

Context:
- Project Root: %s/
- Total Files: %d
- Agents: code, data, infra, sec

Rules:
1. Phases: 3-5 max
2. Tasks MUST use full file paths starting with %s/
3. Output JSON only

JSON:`, query, projectRoot, fileCount, projectRoot)

	result, err := p.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return p.parsePlanResponse(result.Response, query)
}

// buildPlanningPrompt creates a combined prompt (fallback for larger models)
func (p *Planner) buildPlanningPrompt(req *PlanGenerationRequest) string {
	return fmt.Sprintf(`Plan for: %s
Output JSON: {"title":"...","description":"...","phases":[{"name":"...","agent":"code","tasks":[{"description":"..."}],"success_criteria":"...","estimated_time":"5 min"}]}
JSON:`, req.Query)
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
		Title         string              `json:"title"`
		Description   string              `json:"description"`
		FileStructure map[string][]string `json:"file_structure"`
		Phases        []struct {
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
		Title:         rawPlan.Title,
		Description:   rawPlan.Description,
		FileStructure: rawPlan.FileStructure,
		Phases:        make([]Phase, len(rawPlan.Phases)),
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
