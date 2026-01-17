# Contributing to QuantumFlow

Thank you for your interest in contributing to QuantumFlow! This document provides guidelines for contributing to the project.

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (for infrastructure services)
- Ollama (for local Qwen models)
- Git

### Getting Started

1. **Fork and Clone**
   ```bash
   git clone https://github.com/your-username/quantumflow.git
   cd quantumflow
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Start Infrastructure Services**
   ```bash
   docker-compose -f deployments/docker-compose.yml up -d
   ```

4. **Pull Qwen Model**
   ```bash
   ollama pull qwen3-coder:30b
   ```

5. **Build and Run**
   ```bash
   ./scripts/build.sh
   ./bin/quantumflow
   ```

## Development Workflow

### Running Tests

```bash
# Unit tests only
go test -short ./...

# All tests (including integration tests)
go test ./...

# With coverage
go test -cover ./...

# Benchmarks
go test -bench=. ./internal/inference/
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` to format code
- Run `golangci-lint` before committing
- Write clear, descriptive commit messages

### Project Structure

```
quantumflow/
├── cmd/                   # Application entry points
├── internal/              # Private application code
│   ├── agent/            # Sub-agent orchestration
│   ├── inference/        # Qwen inference engine
│   ├── memory/           # Memory service
│   ├── integration/      # Business connectors
│   └── cli/              # CLI interface
├── pkg/                   # Public libraries
├── scripts/              # Build and deployment scripts
├── deployments/          # Docker and K8s configs
└── docs/                 # Documentation
```

## Pull Request Process

1. **Create a Feature Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Your Changes**
   - Write tests for new functionality
   - Update documentation as needed
   - Ensure all tests pass

3. **Commit Your Changes**
   ```bash
   git add .
   git commit -m "feat: add your feature description"
   ```

   Use conventional commits:
   - `feat:` for new features
   - `fix:` for bug fixes
   - `docs:` for documentation
   - `test:` for tests
   - `refactor:` for code refactoring
   - `perf:` for performance improvements

4. **Push and Create PR**
   ```bash
   git push origin feature/your-feature-name
   ```
   
   Then create a pull request on GitHub.

## Code Review Guidelines

- Be respectful and constructive
- Focus on code quality, not coding style preferences
- Explain your reasoning clearly
- Be open to feedback and discussion

## Adding New Features

### New Integration Connector

1. Create new package in `internal/integration/<service>/`
2. Implement the `Connector` interface
3. Add OAuth2 support if needed
4. Write comprehensive tests
5. Update documentation

### New Agent Type

1. Create agent implementation in `internal/agent/`
2. Update orchestrator routing logic
3. Add specialized tools
4. Write tests
5. Document agent capabilities

## Testing Guidelines

- Write unit tests for all new code
- Include integration tests for external dependencies
- Add benchmarks for performance-critical code
- Aim for 80%+ code coverage

## Documentation

- Update README.md for user-facing features
- Add code comments for complex logic
- Create ADRs (Architecture Decision Records) for significant decisions
- Update API documentation

## Getting Help

- Open an issue for bugs or feature requests
- Join our Discord community (link TBD)
- Check existing issues and discussions

## License

By contributing to QuantumFlow, you agree that your contributions will be licensed under the Apache License 2.0.
