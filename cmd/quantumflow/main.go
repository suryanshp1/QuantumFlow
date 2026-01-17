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

fmt.Println("🤖 Multi-Agent System Active:")
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
handleCommand(input, &history, availableModels)
continue
}

history = append(history, models.Message{
Role:      "user",
Content:   input,
Timestamp: time.Now(),
})

fmt.Println()

// STEP 1: Fast routing to determine agent
fmt.Print("🔍 Analyzing query... ")
startRoute := time.Now()

// Route only (fast)
agents, err := orchestrator.Route(ctx, input, buildContext())
if err != nil || len(agents) == 0 {
fmt.Printf("\n❌ Error: routing failed\n\n")
continue
}

selectedAgent := agents[0]
routeDuration := time.Since(startRoute)

// Show routing result
fmt.Printf("✓ %s (%.0fms)\n", selectedAgent.Name(), routeDuration.Seconds()*1000)

// Get confidence from classifier
classifier := agent.NewRuleBasedClassifier()
_, confidence, _ := classifier.Classify(ctx, input)
fmt.Printf("📊 Confidence: %.0f%% | Generating...\n", confidence*100)
fmt.Println()

// STEP 2: Execute with streaming
fmt.Print("QuantumFlow: ")

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

response, err := selectedAgent.Execute(ctx, request)
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

func handleCommand(cmd string, history *[]models.Message, modelsList []string) {
parts := strings.Fields(cmd)
if len(parts) == 0 {
return
}

switch parts[0] {
case "/help":
fmt.Println("\nCommands: /help /models /history /stats /clear /exit")
fmt.Println("Agent Routing: CodeAgent|DataAgent|InfraAgent|SecAgent\n")
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
╚═════════════════════════════════════════════════════════╝

`, version)
}

func truncate(s string, maxLen int) string {
if len(s) <= maxLen {
return s
}
return s[:maxLen-3] + "..."
}
