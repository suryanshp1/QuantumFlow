# QuantumFlow Quick Start Guide

This guide will help you get up and running with QuantumFlow in under 5 minutes.

## Prerequisites Check

Before starting, ensure you have:
- ✅ Go 1.21 or higher (`go version`)
- ✅ Ollama installed (`ollama --version`)
- ✅ Git

## Installation Steps

### 1. Install Ollama (if not already installed)

**macOS:**
```bash
brew install ollama
```

**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

### 2. Start Ollama Service

```bash
# Start Ollama in the background
ollama serve &
```

### 3. Pull Qwen Model

```bash
# For testing (smaller model)
ollama pull qwen2:7b

# For production (recommended - 32B parameters)
ollama pull qwen3-coder:30b
```

> **Note**: The 32B model requires approximately 20GB disk space and 16GB+ RAM

### 4. Build QuantumFlow

```bash
cd /Users/surajpandey/Documents/Python/terminal-ai-assistant

# Build the binary
./scripts/build.sh
```

### 5. Run QuantumFlow

```bash
./bin/quantumflow
```

## First Interaction

Once QuantumFlow starts, you'll see a welcome banner. Try these commands:

### Basic Chat
```
You: Hello! What can you help me with?
```

### Check Available Models
```
/models
```

### View Conversation History
```
/history
```

### Get Statistics
```
/stats
```

## Testing Without Ollama

If Ollama is not running, QuantumFlow will show a helpful error message:

```
⚠️  Warning: Could not connect to Ollama at http://localhost:11434
   Please ensure Ollama is running: ollama serve
   And that Qwen model is available: ollama pull qwen3-coder:30b
```

## Running Tests

```bash
# Unit tests only (no Ollama required)
go test -short ./...

# Integration tests (requires Ollama)
go test ./...

# Benchmarks
go test -bench=. ./internal/inference/
```

## Optional: Start Infrastructure Services

For advanced features (memory service, graph storage), start the Docker infrastructure:

```bash
docker-compose -f deployments/docker-compose.yml up -d

# Verify services are running
docker-compose ps
```

This starts:
- **Redis** (port 6379) - Vector memory storage
- **Dgraph** (port 8080) - Semantic knowledge graph
- **TimescaleDB** (port 5432) - Time-series patterns

## Next Steps

- Read [CONTRIBUTING.md](docs/CONTRIBUTING.md) for development guidelines
- Explore the codebase structure
- Try implementing a custom agent in `internal/agent/`
- Add business integrations (GitHub, Slack, Salesforce)

## Troubleshooting

### Issue: "Model not found"
**Solution**: Pull the model first
```bash
ollama pull qwen3-coder:30b
```

### Issue: "Connection refused"
**Solution**: Start Ollama service
```bash
ollama serve
```

### Issue: "Build failed"
**Solution**: Ensure Go 1.21+ is installed
```bash
go version
go mod tidy
```

## Configuration

Create a custom config file (optional):

```bash
cp config.example.yaml ~/.quantumflow/config.yaml
```

Edit the values as needed:
- Model name
- Context size
- Temperature
- Integration settings

## Performance Expectations

| Model | First Token | Throughput | RAM Required |
|-------|-------------|------------|--------------|
| qwen2:7b | <150ms | 80+ tok/s | 8GB |
| qwen2:14b | <200ms | 60+ tok/s | 12GB |
| qwen3-coder:30b | <250ms | 50+ tok/s | 20GB |
| qwen2:72b | <500ms | 30+ tok/s | 48GB |

*Performance assumes RTX 4090 or A100 GPU. CPU inference will be slower.*

## Getting Help

- **Issues**: https://github.com/quantumflow/quantumflow/issues
- **Discussions**: https://github.com/quantumflow/quantumflow/discussions
- **Documentation**: See `docs/` directory

---

**You're all set!** 🚀

Start chatting with QuantumFlow and experience the power of local AI assistance.
