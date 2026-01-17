# Plan Mode 2.0: Autonomous Multi-Step Execution

QuantumFlow's **Plan Mode** transforms it from a simple terminal assistant into an autonomous software engineer capable of planning and executing complex multi-step tasks.

## 🚀 Key Features

- **Automated Planning**: Decomposes complex requests into phased execution plans using LLMs.
- **Autonomous Execution**: Agents execute plans step-by-step without manual intervention.
- **File System Access**: Can create, read, and modify files.
- **Command Execution**: Can run shell commands (install dependencies, run tests).
- **Human-in-the-Loop**: Review and approve plans before execution.
- **State Persistence**: Resumes interrupted plans automatically.
- **Checkpoint & Rollback**: Safe execution with automatic failure recovery.

## 📖 How to Use

### 1. Generate a Plan

Use the `/plan` command for complex tasks that require multiple steps.

```bash
/plan Add a REST API for blog posts with JWT auth
```

QuantumFlow will analyze the complexity and generate a phased plan:

```
📋 Generating execution plan...

## Phase 1: Database Schema (DataAgent)
## Phase 2: Auth Implementation (CodeAgent)
## Phase 3: API Endpoints (CodeAgent)

✓ Plan saved to: ~/.quantumflow/plans/plan_20260117.md
▶️  Execute with: /execute plan_20260117
```

### 2. Execute the Plan

Run the generated plan ID:

```bash
/execute plan_20260117
```

### 3. Review & Approve

You will see the full plan details. Type `y` to approve:

```
⚠️  This plan will be executed automatically.
Approve execution? [y/N/e(dit)]: y
```

### 4. Watch It Work

QuantumFlow will execute phases sequentially:

```
🚀 Starting execution...

📍 Phase 1/3: Database Schema
🤖 Agent: DataAgent
⚡ Commands Executed:
  • pip install sqlalchemy
💾 Files Created:
  • models.py

✅ Phase 1 complete!
```

## 🛠️ Advanced Features

### Editing Plans
You can manually edit the generated markdown plan file before execution to adjust tasks or phases:
```bash
nano ~/.quantumflow/plans/plan_20260117.md
```

### Restarting Plans
If a plan fails or is interrupted, simply run `/execute` again. 
- If interrupted: It resumes from the last checkpoint.
- If completed/failed: It asks if you want to restart from scratch.

### Safe Mode
Dangerous commands (e.g., `rm -rf /`) are blocked automatically.

## 🏗️ Architecture

Plan Mode uses a specific architecture:
1. **Planner**: LLM decomposes user intent into a JSON structure.
2. **Executor**: Sequential state machine that runs phases.
3. **Checkpoints**: JSON snapshots of execution state.
4. **Agents**: Specialized agents (Code, Data, Sec) perform the actual work.

---
**Next Steps**: Phase 3 will introduce Git integration for even safer rollbacks.
