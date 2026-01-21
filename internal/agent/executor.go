package agent

import (
	"context"
	"fmt"
	"os"
"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Executor executes multi-phase plans with checkpoint support
type Executor struct {
	orchestrator *AgentOrchestrator
	checkpoints  map[string]*Checkpoint
}

// NewExecutor creates a new plan executor
func NewExecutor(orchestrator *AgentOrchestrator) *Executor {
	return &Executor{
		orchestrator: orchestrator,
		checkpoints:  make(map[string]*Checkpoint),
	}
}

// Execute runs an execution plan phase by phase
func (e *Executor) Execute(ctx context.Context, plan *ExecutionPlan) error {
	// Update plan state - Only set StartedAt if not already set (resuming)
	now := time.Now()
	plan.State.Status = ExecutionStatusRunning
	if plan.State.StartedAt == nil {
		plan.State.StartedAt = &now
	}
	
	// If starting fresh, reset current phase
	if plan.State.CurrentPhase < 0 {
		plan.State.CurrentPhase = 0
	}
	
	// Initialize project manifest for tracking created files
	if plan.Manifest == nil {
		plan.Manifest = NewProjectManifest(plan.Title, ".")
		if plan.FileStructure != nil {
			plan.Manifest.SetExpectedStructure(plan.FileStructure)
		}
	}
	
	// Create project directory structure upfront
	if err := e.initProjectStructure(plan); err != nil {
		fmt.Printf("⚠️ Warning: Could not create project directories: %v\n", err)
	}
	
	fmt.Printf("\n🚀 Starting execution of: %s\n", plan.Title)
	fmt.Printf("Total phases: %d\n", len(plan.Phases))
	if len(plan.FileStructure) > 0 {
		fmt.Printf("📁 Project structure: %d directories\n", len(plan.FileStructure))
	}
	fmt.Println()
	
	// Execute each phase sequentially
	for i := plan.State.CurrentPhase; i < len(plan.Phases); i++ {
		phase := &plan.Phases[i]
		
		// Check if phase has dependencies
		if !e.areDependenciesMet(plan, phase) {
			return fmt.Errorf("dependencies not met for phase %s", phase.Name)
		}
		
		// Create checkpoint before phase
		checkpoint, err := e.createCheckpoint(plan.ID, i)
		if err != nil {
			return fmt.Errorf("failed to create checkpoint: %w", err)
		}
		
		// Execute phase
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("📍 Phase %d/%d: %s\n", i+1, len(plan.Phases), phase.Name)
		fmt.Printf("🤖 Agent: %s | ⏱️  Estimated: %s\n", phase.Agent, phase.EstimatedTime)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
		
		if err := e.executePhase(ctx, plan, phase); err != nil {
			// Phase failed - rollback
			fmt.Printf("\n❌ Phase %d failed: %v\n", i+1, err)
			fmt.Println("🔄 Rolling back to checkpoint...")
			
			if rollbackErr := e.rollback(checkpoint); rollbackErr != nil {
				return fmt.Errorf("phase failed and rollback failed: %w", rollbackErr)
			}
			
			plan.State.Status = ExecutionStatusFailed
			plan.State.FailedPhases = append(plan.State.FailedPhases, i)
			return fmt.Errorf("phase %d failed: %w", i+1, err)
		}
		
		// Phase succeeded
		plan.State.CompletedPhases = append(plan.State.CompletedPhases, i)
		plan.State.CurrentPhase = i + 1
		phase.Status = PhaseStatusCompleted
		
		fmt.Printf("\n✅ Phase %d complete!\n\n", i+1)
	}
	
	// All phases completed
	now = time.Now()
	plan.State.Status = ExecutionStatusCompleted
	plan.State.CompletedAt = &now
	
	duration := time.Since(*plan.State.StartedAt).Round(time.Second)
	
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🎉 Execution complete! (%s)\n", plan.Title)
	fmt.Printf("⏱️  Duration: %s\n", duration)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	
	return nil
}

// executePhase executes a single phase using the appropriate agent
func (e *Executor) executePhase(ctx context.Context, plan *ExecutionPlan, phase *Phase) error {
	phase.Status = PhaseStatusInProgress
	
	// Get the agent for this phase
	agents := e.orchestrator.GetAgents()
	var targetAgent Agent
	for _, agent := range agents {
		if agent.Type() == phase.Agent {
			targetAgent = agent
			break
		}
	}
	
	if targetAgent == nil {
		return fmt.Errorf("agent %s not found", phase.Agent)
	}
	
	// Build query from tasks with project context
	query := e.buildPhaseQuery(plan, phase)
	
	// Execute with the agent
	fmt.Printf("Executing tasks:\n")
	for i, task := range phase.Tasks {
		fmt.Printf("  %d. %s\n", i+1, task.Description)
	}
	fmt.Println()
	
	request := &Request{
		ID:      fmt.Sprintf("%s-phase-%s", plan.ID, phase.ID),
		Query:   query,
		Context: &Context{},
		Timeout: 10 * time.Minute, // Generous timeout for phases
	}
	
	response, err := targetAgent.Execute(ctx, request)
	if err != nil {
		return err
	}
	
	// Process agent response - Scan for file blocks and write them
	filesCreated, err := e.processFileBlocks(response.Answer, plan, phase.Name)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to write some files: %v\n", err)
	}
	
	if len(filesCreated) > 0 {
		fmt.Println("\n💾 Files Created/Updated:")
		for _, file := range filesCreated {
			fmt.Printf("  • %s\n", file)
		}
	}
	
	// Process agent response - Scan for command blocks and execute them
	commandsExecuted, err := e.processCommandBlocks(response.Answer)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to execute some commands: %v\n", err)
	}
	
	if len(commandsExecuted) > 0 {
		fmt.Println("\n⚡ Commands Executed:")
		for _, cmd := range commandsExecuted {
			fmt.Printf("  • %s\n", cmd)
		}
	}
	
	// Mark all tasks as completed
	for i := range phase.Tasks {
		phase.Tasks[i].Completed = true
		phase.Tasks[i].Result = response.Answer
	}
	
	fmt.Printf("\n📝 Agent Response:\n%s\n", truncateResponse(response.Answer, 500))
	
	return nil
}

// processFileBlocks identifies code blocks with potential filenames and writes them to disk
func (e *Executor) processFileBlocks(response string, plan *ExecutionPlan, phaseName string) ([]string, error) {
	var filesCreated []string
	
	// Multiple regex patterns to match different code block formats:
	// Pattern 1: ```language path/to/file.ext  (standard format)
	// Pattern 2: ```language filename="path/to/file.ext"  (quoted format)
	patterns := []string{
		"(?s)```(\\w+)\\s+([\\w./-]+)\\s*\n(.*?)```",           // ```python path/file.py\ncode\n```
		"(?s)```(\\w+)\\s*\n#\\s*([\\w./-]+)\\s*\n(.*?)```",    // ```python\n# path/file.py\ncode\n```
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(response, -1)
		
		for _, match := range matches {
			if len(match) < 4 {
				continue
			}
			
			// lang := match[1]  // language (python, go, etc)
			filename := strings.TrimSpace(match[2])
			content := strings.TrimSpace(match[3])
			
			// Skip if no content
			if content == "" {
				fmt.Printf("⚠️  Skipping empty file: %s\n", filename)
				continue
			}
			
			// Ensure file is in current directory or relative subdirectory
			cleanPath := filepath.Clean(filename)
			if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
				fmt.Printf("⚠️  Skipping unsafe file path: %s\n", filename)
				continue
			}
			
			// Check if file was already created in a previous phase
			if plan.Manifest != nil && plan.Manifest.FileExists(cleanPath) {
				fmt.Printf("⚠️  Skipping already created file: %s\n", cleanPath)
				continue
			}
			
			// Create directory if needed
			dir := filepath.Dir(cleanPath)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return filesCreated, fmt.Errorf("failed to create directory for %s: %w", filename, err)
				}
			}
			
			// Write file
			if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
				return filesCreated, fmt.Errorf("failed to write %s: %w", filename, err)
			}
			
			// Track file in manifest
			if plan.Manifest != nil {
				plan.Manifest.AddFile(cleanPath, phaseName, "")
			}
			
			filesCreated = append(filesCreated, cleanPath)
		}
	}
	
	return filesCreated, nil
}

// processCommandBlocks identifies shell command blocks and executes them
func (e *Executor) processCommandBlocks(response string) ([]string, error) {
	var commandsExecuted []string
	
	// Regex matches: ```bash or ```sh
	// followed by content
	// followed by ```
	re := regexp.MustCompile("(?m)^```(bash|sh|shell)\\s*\\n([\\s\\S]*?)^```")
	matches := re.FindAllStringSubmatch(response, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		script := strings.TrimSpace(match[2])
		if script == "" {
			continue
		}
		
		// Split into individual lines to execute sequentially 
		// (simplification, ideally we'd run the whole block as a script)
		lines := strings.Split(script, "\n")
		for _, cmdStr := range lines {
			cmdStr = strings.TrimSpace(cmdStr)
			if cmdStr == "" || strings.HasPrefix(cmdStr, "#") {
				continue
			}
			
			// Safety check: Prevent highly dangerous commands
			if isDangerousCommand(cmdStr) {
				fmt.Printf("⚠️  Skipping potentially dangerous command: %s\n", cmdStr)
				continue
			}
			
			fmt.Printf("running: %s\n", cmdStr)
			
			// Execute command
			cmd := exec.Command("bash", "-c", cmdStr)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			
			if err := cmd.Run(); err != nil {
				return commandsExecuted, fmt.Errorf("failed to execute '%s': %w", cmdStr, err)
			}
			
			commandsExecuted = append(commandsExecuted, cmdStr)
		}
	}
	
	return commandsExecuted, nil
}

// isDangerousCommand checks for obviously dangerous commands
func isDangerousCommand(cmd string) bool {
	dangerous := []string{"rm -rf /", "rm -rf ~", ":(){ :|:& };:"}
	for _, d := range dangerous {
		if strings.Contains(cmd, d) {
			return true
		}
	}
	return false
}

// buildPhaseQuery creates a comprehensive query for the phase with project context
func (e *Executor) buildPhaseQuery(plan *ExecutionPlan, phase *Phase) string {
	var query strings.Builder
	
	query.WriteString(fmt.Sprintf("Phase: %s\n\n", phase.Name))
	
	// Add file structure context from manifest
	if plan.Manifest != nil {
		structureCtx := plan.Manifest.GetFileStructureForContext()
		if structureCtx != "" {
			query.WriteString(structureCtx)
			query.WriteString("\n")
		}
		
		filesCtx := plan.Manifest.GetCreatedFilesForContext()
		if filesCtx != "" {
			query.WriteString(filesCtx)
			query.WriteString("\n")
		}
	}
	
	query.WriteString("Please complete the following tasks:\n\n")
	
	for i, task := range phase.Tasks {
		query.WriteString(fmt.Sprintf("%d. %s\n", i+1, task.Description))
	}
	
	query.WriteString(fmt.Sprintf("\n\nSuccess Criteria: %s\n", phase.SuccessCriteria))
	
	query.WriteString(`

FILE OUTPUT FORMAT - You MUST use this EXACT format to create files:
` + "```python ecommerce_api/routes/products.py" + `
from fastapi import APIRouter
router = APIRouter()

@router.get("/products")
def list_products():
    return {"products": []}
` + "```" + `

RULES:
1. Every code block MUST have the file path on the SAME LINE as the language
2. Include ACTUAL CODE in the block, not placeholders
3. Use FULL relative paths like: ecommerce_api/models/product.py
4. One file per code block
`)
	
	return query.String()
}

// initProjectStructure creates project directories upfront
func (e *Executor) initProjectStructure(plan *ExecutionPlan) error {
	if plan.FileStructure == nil || len(plan.FileStructure) == 0 {
		return nil
	}
	
	fmt.Println("📂 Creating project structure...")
	for dir := range plan.FileStructure {
		if dir == "." || dir == "" {
			continue
		}
		// Remove trailing slash if present
		cleanDir := strings.TrimSuffix(dir, "/")
		if err := os.MkdirAll(cleanDir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", cleanDir, err)
		}
		fmt.Printf("  ✓ %s/\n", cleanDir)
	}
	return nil
}

// areDependenciesMet checks if all dependencies for a phase are satisfied
// areDependenciesMet checks if all dependencies for a phase are satisfied
func (e *Executor) areDependenciesMet(plan *ExecutionPlan, phase *Phase) bool {
	if len(phase.Dependencies) == 0 {
		return true
	}
	
	for _, depID := range phase.Dependencies {
		depCompleted := false
		for _, completedIdx := range plan.State.CompletedPhases {
			completedPhase := plan.Phases[completedIdx]
			// Check if completed phase matches dependency by ID OR Name
			if completedPhase.ID == depID || completedPhase.Name == depID {
				depCompleted = true
				break
			}
		}
		if !depCompleted {
			return false
		}
	}
	
	return true
}

// createCheckpoint creates a checkpoint before executing a phase
func (e *Executor) createCheckpoint(planID string, phaseIndex int) (*Checkpoint, error) {
	checkpoint := &Checkpoint{
		ID:         fmt.Sprintf("checkpoint-%s-phase-%d", planID, phaseIndex),
		PlanID:     planID,
		PhaseIndex: phaseIndex,
		Timestamp:  time.Now(),
		Metadata:   make(map[string]string),
	}
	
	// For now, just store in memory
	// In Week 3, we'll add Git-based snapshots
	e.checkpoints[checkpoint.ID] = checkpoint
	
	return checkpoint, nil
}

// rollback restores state to a checkpoint
func (e *Executor) rollback(checkpoint *Checkpoint) error {
	// For now, just log
	// In Week 3, we'll implement Git restore
	fmt.Printf("Rollback to checkpoint: %s\n", checkpoint.ID)
	return nil
}

// truncateResponse truncates a response for display
func truncateResponse(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...\n[Response truncated for display]"
}
