# QuantumFlow Testing Guide

This guide shows you how to test QuantumFlow at different levels, from quick unit tests to full integration testing.

## Quick Test (No Ollama Required)

The fastest way to verify the build works:

```bash
cd /Users/surajpandey/Documents/Python/terminal-ai-assistant

# Run unit tests only (skips integration tests)
go test -short ./...
```

**Expected Output:**
```
ok      github.com/quantumflow/quantumflow/internal/inference   (cached)
ok      github.com/quantumflow/quantumflow/internal/memory      (cached)
ok      github.com/quantumflow/quantumflow/internal/agent       (cached)
```

---

## Full Test with Ollama

If you have Ollama installed and running:

### 1. Start Ollama

```bash
# Start Ollama service
ollama serve &

# Pull a smaller model for testing (faster than 32B)
ollama pull qwen2:7b
```

### 2. Run All Tests

```bash
# Run all tests including integration tests
go test ./... -v

# With coverage report
go test -cover ./...

# Benchmarks
go test -bench=. ./internal/inference/
```

---

## Test Individual Components

### Inference Engine

```bash
# Unit tests
go test -short ./internal/inference/

# Integration tests (requires Ollama)
go test ./internal/inference/ -v

# Benchmarks
go test -bench=BenchmarkPool ./internal/inference/
```

**What it tests:**
- Ollama client initialization
- Synchronous generation
- Streaming generation
- Pool concurrency (100 workers)
- Rate limiting

### Memory Service

```bash
# Unit tests
go test -short ./internal/memory/

# Integration tests (requires Redis/Dgraph/BadgerDB)
go test ./internal/memory/ -v
```

**What it tests:**
- Memory extraction
- Embedding generation
- Vector similarity search (mocked)
- Knowledge graph operations (mocked)

### Agent Framework

```bash
# Unit tests
go test -short ./internal/agent/

# Full agent tests
go test ./internal/agent/ -v
```

**What it tests:**
- Query classification (rule-based)
- Agent routing
- Tool execution
- Conflict resolution

### Integration Connectors

```bash
# Unit tests (no API calls)
go test -short ./internal/integration/

# Integration tests (requires API credentials)
go test ./internal/integration/ -v
```

---

## Manual Testing with CLI

### 1. Build and Run

```bash
# Build binary
./scripts/build.sh

# Run QuantumFlow
./bin/quantumflow
```

### 2. Test Basic Chat

```
QuantumFlow> Hello, can you introduce yourself?
```

**Expected**: Streaming response from Qwen with performance metrics

### 3. Test Agent Routing

**CodeAgent:**
```
QuantumFlow> How do I parse Go AST for finding function declarations?
```
**Expected**: Routes to CodeAgent, discusses AST parsing

**DataAgent:**
```
QuantumFlow> Write a SQL query to find customers who purchased in last 30 days
```
**Expected**: Routes to DataAgent, generates SQL

**InfraAgent:**
```
QuantumFlow> How do I deploy a Docker container to Kubernetes?
```
**Expected**: Routes to InfraAgent, discusses deployment

**SecAgent:**
```
QuantumFlow> Check my authentication code for SQL injection vulnerabilities
```
**Expected**: Routes to SecAgent, discusses security

### 4. Test Commands

```
/models      # List available Ollama models
/history     # Show conversation history
/stats       # Display session statistics
/clear       # Start new conversation
/help        # Show help message
```

---

## Test Scenarios

### Scenario 1: Code Analysis

```
QuantumFlow> I have a Go function with a memory leak. How can I debug it?
```

**Expected Behavior:**
1. Classifier routes to CodeAgent (confidence ~0.8)
2. CodeAgent provides debugging strategies
3. Memory service stores the interaction
4. Response includes performance metrics

### Scenario 2: Multi-Turn Conversation

```
QuantumFlow> What is a goroutine?

QuantumFlow> How does it differ from a thread?

QuantumFlow> Show me an example
```

**Expected Behavior:**
1. Each response uses context from previous messages
2. Memory service retrieves relevant past interactions
3. Conversation history maintained in `/history`

### Scenario 3: Memory Retrieval

```
QuantumFlow> Earlier we discussed goroutines. Can you summarize?
```

**Expected Behavior:**
1. Memory service searches episodic memory
2. Retrieves top-5 relevant past interactions
3. Qwen synthesizes summary

---

## Performance Testing

### Latency Test

```bash
# Run inference benchmark
go test -bench=BenchmarkGenerateSync ./internal/inference/
```

**Expected Metrics:**
- First token: 150-300ms
- Throughput: 40-70 tokens/sec (depends on hardware)

### Concurrency Test

```bash
# Test pool concurrency
go test -bench=BenchmarkPoolThroughput ./internal/inference/
```

**Expected**: 100 workers handling requests concurrently

### Memory Retrieval Test

```bash
# Test vector search performance
go test -bench=BenchmarkMemoryRetrieval ./internal/memory/
```

**Expected**: <50ms for top-10 retrieval

---

## Infrastructure Testing

### 1. Start Docker Services

```bash
cd /Users/surajpandey/Documents/Python/terminal-ai-assistant

# Start Redis, Dgraph, TimescaleDB
docker-compose -f deployments/docker-compose.yml up -d

# Check status
docker-compose ps
```

**Expected Output:**
```
NAME                STATUS
redis               Up
dgraph-zero         Up
dgraph-alpha        Up
timescaledb         Up
```

### 2. Test Redis Connection

```bash
# Connect to Redis
docker exec -it quantumflow-redis redis-cli

# Test command
127.0.0.1:6379> PING
PONG

# Check FT.SEARCH module (for vector search)
127.0.0.1:6379> FT._LIST
```

### 3. Test Dgraph

```bash
# Check Dgraph health
curl http://localhost:8080/health

# Expected: {"status":"healthy"}
```

---

## Troubleshooting

### Issue: Ollama Connection Failed

**Error:**
```
⚠️  Warning: Could not connect to Ollama at http://localhost:11434
```

**Fix:**
```bash
# Start Ollama
ollama serve &

# Verify it's running
curl http://localhost:11434/api/tags
```

### Issue: Model Not Found

**Error:**
```
⚠️  Model 'qwen3-coder:30b' not found
```

**Fix:**
```bash
# Pull the model
ollama pull qwen3-coder:30b

# Or use a smaller model
ollama pull qwen2:7b
```

### Issue: Build Failed

**Error:**
```
# github.com/quantumflow/quantumflow/...
```

**Fix:**
```bash
# Clean and rebuild
go clean
go mod tidy
./scripts/build.sh
```

### Issue: Docker Services Won't Start

**Fix:**
```bash
# Stop all services
docker-compose -f deployments/docker-compose.yml down

# Remove volumes (⚠️ deletes data)
docker-compose -f deployments/docker-compose.yml down -v

# Restart
docker-compose -f deployments/docker-compose.yml up -d
```

---

## Integration Testing (Advanced)

### GitHub Integration

```bash
# Set up OAuth credentials in config
vim ~/.quantumflow/config.yaml

# Enable GitHub
integrations:
  github:
    enabled: true
    oauth2:
      client_id: "your_github_client_id"
      client_secret: "your_github_secret"
```

**Test Query:**
```
QuantumFlow> List all open pull requests in my repository
```

### Salesforce Integration

**Test SOQL Query:**
```
QuantumFlow> Query Salesforce for all opportunities created this month
```

---

## Automated Test Suite

Create a test script:

```bash
#!/bin/bash
# test-all.sh

echo "🧪 Running QuantumFlow Test Suite..."

echo "1️⃣ Unit Tests..."
go test -short ./... || exit 1

echo "2️⃣ Building Binary..."
./scripts/build.sh || exit 1

echo "3️⃣ Checking Binary Size..."
size=$(ls -lh bin/quantumflow | awk '{print $5}')
echo "   Binary size: $size"

echo "4️⃣ Starting Docker Services..."
docker-compose -f deployments/docker-compose.yml up -d

echo "5️⃣ Waiting for Services..."
sleep 5

echo "6️⃣ Testing Redis..."
docker exec quantumflow-redis redis-cli PING || exit 1

echo "7️⃣ Testing Dgraph..."
curl -f http://localhost:8080/health || exit 1

echo "✅ All Tests Passed!"
```

Run it:
```bash
chmod +x test-all.sh
./test-all.sh
```

---

## Continuous Integration

For CI/CD pipelines (GitHub Actions example):

```yaml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run Tests
        run: go test -short ./...
      
      - name: Build
        run: ./scripts/build.sh
```

---

## Success Criteria

Your QuantumFlow installation is working correctly if:

✅ **Unit tests pass**: `go test -short ./...` returns no errors  
✅ **Binary builds**: `./scripts/build.sh` succeeds, size ~5.4MB  
✅ **Ollama connects**: CLI shows "✓ Connected to Ollama"  
✅ **Chat works**: Receives streaming responses with metrics  
✅ **Agents route**: Different queries go to appropriate agents  
✅ **Memory stores**: `/history` shows past conversations  
✅ **Docker services**: `docker-compose ps` shows all containers Up  

---

**Next Steps**: Once all tests pass, you're ready for production use! 🚀
