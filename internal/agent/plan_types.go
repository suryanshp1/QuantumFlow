package agent

import (
	"time"

	"github.com/quantumflow/quantumflow/internal/models"
)

// ExecutionPlan represents a multi-phase execution plan
type ExecutionPlan struct {
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	Description   string              `json:"description"`
	FileStructure map[string][]string `json:"file_structure,omitempty"` // Expected: dir -> files
	Phases        []Phase             `json:"phases"`
	State         ExecutionState      `json:"state"`
	Manifest      *ProjectManifest    `json:"-"` // Runtime tracking (not serialized)
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// Phase represents a single phase in an execution plan
type Phase struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Agent           models.AgentType  `json:"agent"`
	Tasks           []Task            `json:"tasks"`
	SuccessCriteria string            `json:"success_criteria"`
	EstimatedTime   string            `json:"estimated_time"`
	Dependencies    []string          `json:"dependencies"` // IDs of phases that must complete first
	Status          PhaseStatus       `json:"status"`
}

// Task represents a specific task within a phase
type Task struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Completed   bool       `json:"completed"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ExecutionState tracks the current state of plan execution
type ExecutionState struct {
	Status          ExecutionStatus `json:"status"`
	CurrentPhase    int             `json:"current_phase"`
	CompletedPhases []int           `json:"completed_phases"`
	FailedPhases    []int           `json:"failed_phases"`
	LastCheckpoint  string          `json:"last_checkpoint,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}

// PhaseStatus represents the status of a phase
type PhaseStatus string

const (
	PhaseStatusPending    PhaseStatus = "pending"
	PhaseStatusInProgress PhaseStatus = "in_progress"
	PhaseStatusCompleted  PhaseStatus = "completed"
	PhaseStatusFailed     PhaseStatus = "failed"
	PhaseStatusSkipped    PhaseStatus = "skipped"
)

// ExecutionStatus represents the overall status of plan execution
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusApproved  ExecutionStatus = "approved"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// Checkpoint represents a saved state for rollback
type Checkpoint struct {
	ID          string            `json:"id"`
	PlanID      string            `json:"plan_id"`
	PhaseIndex  int               `json:"phase_index"`
	Timestamp   time.Time         `json:"timestamp"`
	GitStash    string            `json:"git_stash,omitempty"`
	Metadata    map[string]string `json:"metadata"`
}

// PlanGenerationRequest contains information for generating a plan
type PlanGenerationRequest struct {
	Query       string            `json:"query"`
	Context     *Context          `json:"context"`
	Preferences PlanPreferences   `json:"preferences"`
}

// PlanPreferences allows customization of plan generation
type PlanPreferences struct {
	MaxPhases       int  `json:"max_phases"`
	RequireApproval bool `json:"require_approval"`
	AutoExecute     bool `json:"auto_execute"`
	VerboseLogging  bool `json:"verbose_logging"`
}

// DefaultPlanPreferences returns default plan preferences
func DefaultPlanPreferences() PlanPreferences {
	return PlanPreferences{
		MaxPhases:       10,
		RequireApproval: true,
		AutoExecute:     false,
		VerboseLogging:  true,
	}
}
