package agent

import (
	"context"
	"fmt"
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
	// Update plan state
	now := time.Now()
	plan.State.Status = ExecutionStatusRunning
	plan.State.StartedAt = &now
	plan.State.CurrentPhase = 0
	
	fmt.Printf("\n🚀 Starting execution of: %s\n", plan.Title)
	fmt.Printf("Total phases: %d\n\n", len(plan.Phases))
	
	// Execute each phase sequentially
	for i := range plan.Phases {
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
	
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🎉 Execution complete! (%s)\n", plan.Title)
	fmt.Printf("⏱️  Duration: %s\n", now.Sub(*plan.State.StartedAt).Round(time.Second))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	
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
	
	// Build query from tasks
	query := e.buildPhaseQuery(phase)
	
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
	
	// Mark all tasks as completed
	for i := range phase.Tasks {
		phase.Tasks[i].Completed = true
		phase.Tasks[i].Result = response.Answer
	}
	
	fmt.Printf("\n📝 Agent Response:\n%s\n", truncateResponse(response.Answer, 500))
	
	return nil
}

// buildPhaseQuery creates a comprehensive query for the phase
func (e *Executor) buildPhaseQuery(phase *Phase) string {
	query := fmt.Sprintf("Phase: %s\n\n", phase.Name)
	query += "Please complete the following tasks:\n\n"
	
	for i, task := range phase.Tasks {
		query += fmt.Sprintf("%d. %s\n", i+1, task.Description)
	}
	
	query += fmt.Sprintf("\n\nSuccess Criteria: %s\n", phase.SuccessCriteria)
	
	return query
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
