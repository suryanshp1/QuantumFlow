package main

import (
"bufio"
"context"
"fmt"
"os"
"os/signal"
"strings"
"syscall"
"time"

"github.com/quantumflow/quantumflow/internal/agent"
"github.com/quantumflow/quantumflow/internal/inference"
"github.com/quantumflow/quantumflow/internal/models"
)

const version = "0.1.0-alpha"

func main() {
printBanner()

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
go func() {
<-sigChan
fmt.Println("\n\nShutting down...")
cancel()
os.Exit(0)
}()

config := inference.DefaultConfig()
client := inference.NewClient(config)

availableModels, err := client.ListModels(ctx)
if err != nil {
fmt.Printf("⚠️ Warning: %v\n", err)
}

modelFound := false
for _, m := range availableModels {
if m == config.Model {
modelFound = true
break
}
}

if !modelFound && len(availableModels) > 0 {
fmt.Printf("⚠️ Model '%s' not found\n", config.Model)
}

fmt.Printf("✓ Connected to Ollama | Model: %s\n\n", config.Model)

orchestratorConfig := agent.DefaultOrchestratorConfig()
orchestrator := agent.NewAgentOrchestrator(orchestratorConfig, nil, client)

orchestrator.RegisterAgent(agent.NewCodeAgent(client, nil))
orchestrator.RegisterAgent(agent.NewDataAgent(client, nil))
orchestrator.RegisterAgent(agent.NewInfraAgent(client, nil))
orchestrator.RegisterAgent(agent.NewSecAgent(client, nil))
// Initialize planner for Plan Mode
planner := agent.NewPlanner(client)
executor := agent.NewExecutor(orchestrator)
approval := agent.NewApprovalWorkflow(planner)

fmt.Println("🤖 Multi-Agent System Active (Quantum Router):")
fmt.Println("   • CodeAgent  - Code analysis")
fmt.Println("   • DataAgent  - SQL & analytics")
fmt.Println("   • InfraAgent - DevOps")
fmt.Println("   • SecAgent   - Security\n")

scanner := bufio.NewScanner(os.Stdin)
history := []models.Message{}

for {
fmt.Print("You: ")
if !scanner.Scan() {
break
}

input := strings.TrimSpace(scanner.Text())
if input == "" {
continue
}

if strings.HasPrefix(input, "/") {
handleCommand(input, &history, availableModels, client, planner, executor, approval)
continue
}

history = append(history, models.Message{
Role:      "user",
Content:   input,
Timestamp: time.Now(),
})

fmt.Println()

// Execute through orchestrator - it handles routing internally
// This eliminates the double LLM call (was: router.Classify + agent.Execute)
fmt.Print("🧠 Processing... ")
startGen := time.Now()

request := &agent.Request{
ID:      fmt.Sprintf("req-%d", time.Now().Unix()),
Query:   input,
Context: buildContext(),
Timeout: 5 * time.Minute,
StreamCallback: func(token string) {
fmt.Print(token)
},
}

// Single call through orchestrator (includes routing + execution)
response, err := orchestrator.Execute(ctx, request)
if err != nil {
fmt.Printf("\n❌ Error: %v\n\n", err)
continue
}

genDuration := time.Since(startGen)

// Show metrics
fmt.Printf("\n\n⏱ %.2fs | 🚀 %.1f tok/s | 📝 %d tokens\n\n",
genDuration.Seconds(),
float64(response.TokensUsed)/genDuration.Seconds(),
response.TokensUsed)

history = append(history, models.Message{
Role:      "assistant",
Content:   response.Answer,
Timestamp: time.Now(),
})
}
}

func buildContext() *agent.Context {
cwd, _ := os.Getwd()
return &agent.Context{
CurrentDir: cwd,
Constraints: &agent.Constraints{
MaxToolCalls:     10,
MaxExecutionTime: 5 * time.Minute,
},
}
}

func handleCommand(cmd string, history *[]models.Message, modelsList []string, client *inference.Client, planner *agent.Planner, executor *agent.Executor, approval *agent.ApprovalWorkflow) {
parts := strings.Fields(cmd)
if len(parts) == 0 {
return
}

switch parts[0] {
case "/help":
fmt.Println("\nCommands: /help /models /history /stats /clear /exit")
fmt.Println("Agent Routing: Quantum Router (LLM-based)\n")
case "/plan":
handlePlanCommand(cmd, client, planner)
case "/execute":
handleExecuteCommand(cmd, client, planner, executor, approval)
case "/clear", "/new":
*history = []models.Message{}
fmt.Println("✓ Conversation cleared\n")
case "/models":
fmt.Println("\nAvailable models:")
for _, m := range modelsList {
fmt.Printf("  • %s\n", m)
}
fmt.Println()
case "/history":
if len(*history) == 0 {
fmt.Println("\nNo history\n")
return
}
fmt.Println("\n=== History ===")
for i, msg := range *history {
fmt.Printf("%d. %s: %s\n", i+1, msg.Role, truncate(msg.Content, 60))
}
fmt.Println()
case "/stats":
fmt.Printf("\nMessages: %d\n\n", len(*history))
case "/exit", "/quit":
fmt.Println("Goodbye! 👋")
os.Exit(0)
}
}

func printBanner() {
fmt.Printf(`
╔═════════════════════════════════════════════════════════╗
║        QuantumFlow Terminal AI Assistant %s             ║
║            Powered by Quantum Router (LLM)              ║
╚═════════════════════════════════════════════════════════╝

`, version)
}

func truncate(s string, maxLen int) string {
if len(s) <= maxLen {
return s
}
return s[:maxLen-3] + "..."
}

func handlePlanCommand(cmd string, client *inference.Client, planner *agent.Planner) {
parts := strings.SplitN(cmd, " ", 2)
if len(parts) < 2 {
fmt.Println("\nUsage: /plan <task description>")
fmt.Println("Example: /plan Add user authentication with JWT\n")
return
}

query := strings.TrimSpace(parts[1])

fmt.Println("\n🧠 Analyzing task complexity...")
fmt.Println("📋 Generating execution plan...\n")

ctx := context.Background()
req := &agent.PlanGenerationRequest{
Query:       query,
Context:     buildContext(),
Preferences: agent.DefaultPlanPreferences(),
}

plan, err := planner.Generate(ctx, req)
if err != nil {
fmt.Printf("❌ Failed to generate plan: %v\n\n", err)
return
}

// Create plans directory
homeDir, _ := os.UserHomeDir()
plansDir := fmt.Sprintf("%s/.quantumflow/plans", homeDir)
os.MkdirAll(plansDir, 0755)

// Save plan
planFile := fmt.Sprintf("%s/%s.md", plansDir, plan.ID)
markdown := planner.FormatAsMarkdown(plan)
os.WriteFile(planFile, []byte(markdown), 0644)

// Display summary
fmt.Println("═══════════════════════════════════════════════════════════")
fmt.Printf("📋 %s\n", plan.Title)
fmt.Println("═══════════════════════════════════════════════════════════")
fmt.Printf("%s\n\n", plan.Description)

fmt.Printf("**Phases:** %d\n", len(plan.Phases))
for i, phase := range plan.Phases {
fmt.Printf("\n## Phase %d: %s\n", i+1, phase.Name)
fmt.Printf("   Agent: %s | Time: %s\n", phase.Agent, phase.EstimatedTime)
fmt.Printf("   Tasks: %d\n", len(phase.Tasks))
}

fmt.Println("\n═══════════════════════════════════════════════════════════")
fmt.Printf("\n✓ Plan saved to: %s\n", planFile)

// Save plan state for execution
approval := agent.NewApprovalWorkflow(planner)
if err := approval.SavePlanState(plan); err != nil {
fmt.Printf("⚠️  Could not save plan state: %v\n", err)
}

fmt.Printf("\n📋 Plan ID: %s\n", plan.ID)
fmt.Printf("▶️  Execute with: /execute %s\n", plan.ID)
fmt.Println()
}

func handleExecuteCommand(cmd string, client *inference.Client, planner *agent.Planner, executor *agent.Executor, approval *agent.ApprovalWorkflow) {
parts := strings.Fields(cmd)
if len(parts) < 2 {
fmt.Println("\nUsage: /execute <plan-id>")
fmt.Println("Example: /execute plan_20260117_140530\n")
return
}

planID := parts[1]

// Load plan from saved state or file
fmt.Printf("\n📂 Loading plan: %s...\n", planID)

plan, err := approval.LoadPlanState(planID)
if err != nil {
fmt.Printf("❌ Could not load plan: %v\n", err)
fmt.Println("\nTip: Use /plan to generate a new plan first\n")
return
}

// Check if plan was already completed or failed
if plan.State.Status == agent.ExecutionStatusCompleted || plan.State.Status == agent.ExecutionStatusFailed {
fmt.Printf("\n⚠️  This plan has already finished with status: %s\n", plan.State.Status)
fmt.Print("Restart execution from beginning? [y/N]: ")
reader := bufio.NewReader(os.Stdin)
response, _ := reader.ReadString('\n')
response = strings.TrimSpace(strings.ToLower(response))

if response == "y" || response == "yes" {
// Reset state
plan.State.Status = agent.ExecutionStatusPending
plan.State.CurrentPhase = 0
plan.State.CompletedPhases = []int{}
plan.State.FailedPhases = []int{}
plan.State.StartedAt = nil
plan.State.CompletedAt = nil
// Reset tasks
for i := range plan.Phases {
plan.Phases[i].Status = agent.PhaseStatusPending
for j := range plan.Phases[i].Tasks {
plan.Phases[i].Tasks[j].Completed = false
plan.Phases[i].Tasks[j].Result = ""
}
}
fmt.Println("🔄 Plan state reset.")
} else {
fmt.Println("❌ Validation cancelled.")
return
}
}

// Request approval
approved, err := approval.RequestApproval(context.Background(), plan)
if err != nil {
fmt.Printf("❌ Approval error: %v\n\n", err)
return
}

if !approved {
fmt.Println("\n❌ Execution cancelled\n")
return
}

// Save approved state
if err := approval.SavePlanState(plan); err != nil {
fmt.Printf("⚠️  Could not save plan state: %v\n", err)
}

// Execute plan
ctx := context.Background()
if err := executor.Execute(ctx, plan); err != nil {
fmt.Printf("\n❌ Execution failed: %v\n\n", err)

// Save failed state
approval.SavePlanState(plan)
return
}

// Save completed state
if err := approval.SavePlanState(plan); err != nil {
fmt.Printf("⚠️  Could not save final state: %v\n", err)
}

fmt.Println("✅ Plan execution completed successfully!\n")
}
