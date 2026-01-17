# QuantumFlow Future Roadmap

## 🚧 Phase 3: Robustness & Integration (Next)

**Goal**: Make QuantumFlow production-hardened and connected to the outside world.

### 1. Git-Based Rollback (Safety Net)
- **Problem**: Current rollback is in-memory only.
- **Solution**: Use `git` to snapshot state before every execution phase.
- **Mechanism**:
  - `git stash push -u -m "quantumflow-checkpoint-ID"`
  - On failure: `git stash apply`
  - Allows reverting even file creations and modifications safe and instantly.

### 2. Model Context Protocol (MCP) Integration
- **Goal**: Connect to external tools using the standard MCP protocol.
- **Connectors**:
  - **GitHub**: Create PRs, review code, manage issues.
  - **Postgres**: Inspect schema, run migration checks.
  - **Docker**: Manage containers, inspection.
  - **Slack/Discord**: Send notifications on plan completion.

### 3. Production Polish
- **Resume Command**: `/resume <plan_id>` explicitly.
- **Plan Editing**: Open `$EDITOR` (vim/nano) to modify plan.md before approval.
- **Progress UI**: Better spinners, progress bars for long-running tasks.

---

## 🔮 Phase 4: Hyper-Memory & Context

**Goal**: "Infinite" context window via clever retrieval.

### 1. Cross-Repository Indexing
- Analyze dependencies across multiple local repos.
- Understand how `frontend-repo` calls `backend-repo`.

### 2. Semantic Search 2.0
- Move from simple vector search to **GraphRAG**.
- "Find all functions that update user balance" (traversing call graphs).

### 3. Ephemeral Context
- Temporarily load large docs (PDFs, logs) for a single session without polluting long-term memory.

---

## 🛡️ Phase 5: SafeExec & Enterprise

**Goal**: Run untested AI code safely.

### 1. Sandboxed Execution
- Run agents in a **gVisor** container or **WASM** sandbox.
- Trap network calls (allow only whitelisted domains).
- Prevent file system destruction (`rm -rf /`).

### 2. Cost & Audit
- Token usage tracking per project.
- Audit logs for compliance (ISO 27001).

### 3. Parallel Orchestration
- Run multiple plans in parallel.
- "Project Manager" agent that spawns sub-agents.
