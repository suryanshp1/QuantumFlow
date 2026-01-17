package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ApprovalWorkflow handles human-in-the-loop plan approval
type ApprovalWorkflow struct {
	planner *Planner
}

// NewApprovalWorkflow creates a new approval workflow handler
func NewApprovalWorkflow(planner *Planner) *ApprovalWorkflow {
	return &ApprovalWorkflow{
		planner: planner,
	}
}

// RequestApproval displays a plan and requests user approval
func (a *ApprovalWorkflow) RequestApproval(ctx context.Context, plan *ExecutionPlan) (bool, error) {
	// Display plan
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Printf("📋 PLAN REVIEW: %s\n", plan.Title)
	fmt.Println(strings.Repeat("═", 60))
	
	markdown := a.planner.FormatAsMarkdown(plan)
	fmt.Println(markdown)
	
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("\n⚠️  This plan will be executed automatically.")
	fmt.Println("Please review carefully before approving.\n")
	
	// Prompt for approval
	fmt.Print("Approve execution? [y/N/e(dit)]: ")
	
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	
	response = strings.TrimSpace(strings.ToLower(response))
	
	switch response {
	case "y", "yes":
		plan.State.Status = ExecutionStatusApproved
		return true, nil
	case "e", "edit":
		// Future: Open plan in $EDITOR
		fmt.Println("\n⚠️  Plan editing not yet implemented (coming soon!)")
		fmt.Println("For now, you can manually edit the plan file and re-run.\n")
		return false, nil
	default:
		plan.State.Status = ExecutionStatusCancelled
		return false, nil
	}
}

// SavePlanState saves plan state to disk for resumption
func (a *ApprovalWorkflow) SavePlanState(plan *ExecutionPlan) error {
	homeDir, _ := os.UserHomeDir()
	stateDir := fmt.Sprintf("%s/.quantumflow/state", homeDir)
	os.MkdirAll(stateDir, 0755)
	
	stateFile := fmt.Sprintf("%s/%s.json", stateDir, plan.ID)
	
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(stateFile, data, 0644)
}

// LoadPlanState loads a saved plan state
func (a *ApprovalWorkflow) LoadPlanState(planID string) (*ExecutionPlan, error) {
	homeDir, _ := os.UserHomeDir()
	stateFile := fmt.Sprintf("%s/.quantumflow/state/%s.json", homeDir, planID)
	
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}
	
	var plan ExecutionPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	
	return &plan, nil
}
