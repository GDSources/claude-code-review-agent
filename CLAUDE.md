# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based code review agent that processes GitHub pull requests using Claude AI. The agent runs in Docker containers for CI/CD integration, receives webhook events from GitHub, analyzes code changes, and posts review comments back to pull requests.

## Development Commands

### Initial Setup
```bash
go mod init github.com/your-org/review-agent
go mod tidy
```

### Build and Run
```bash
# Build the application
go build -o bin/review-agent cmd/agent/main.go

# Run locally
./bin/review-agent

# Run with environment variables
GITHUB_TOKEN=your_token CLAUDE_API_KEY=your_key ./bin/review-agent
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/github/
go test ./pkg/llm/

# Run single test
go test -run TestWebhookHandler ./pkg/webhook/
```

### Docker Development
```bash
# Build Docker image
docker build -t review-agent:latest .

# Run in container
docker run --rm \
  -e GITHUB_TOKEN="${GITHUB_TOKEN}" \
  -e CLAUDE_API_KEY="${CLAUDE_API_KEY}" \
  -e WEBHOOK_SECRET="${WEBHOOK_SECRET}" \
  -p 8080:8080 \
  review-agent:latest

# Check container health
docker run --rm review-agent:latest /review-agent --health-check
```

### Code Quality
```bash
# Format code
go fmt ./...

# Lint (requires golangci-lint)
golangci-lint run

# Vet for issues
go vet ./...
```

## Architecture Overview

The application follows a modular, event-driven architecture with clear separation of concerns:

### Core Components Structure
```
pkg/
├── webhook/     # GitHub webhook handling and validation
├── github/      # GitHub API client and operations
├── llm/         # LLM provider interface (Claude + future providers)
├── analyzer/    # Code diff parsing and context extraction
├── review/      # Review orchestration and business logic
└── store/       # Database layer and state management
```

### Key Design Patterns

**Provider Interface Pattern**: The LLM client uses a provider interface to support multiple AI services (Claude, GPT, etc.) while maintaining consistent behavior.

**Event-Driven Flow**: GitHub webhooks trigger asynchronous review processes, allowing the system to handle multiple PRs concurrently without blocking.

**Context-Aware Analysis**: The analyzer extracts surrounding code context (±5 lines) around changes to give the LLM sufficient information for meaningful reviews.

### Data Flow
1. GitHub webhook received and validated
2. PR metadata and diffs fetched via GitHub API
3. Code changes parsed and contextualized
4. LLM analyzes changes and generates structured feedback
5. Comments filtered and posted back to GitHub PR

### Configuration Management
- Environment variables for secrets (tokens, API keys)
- YAML/JSON config files for review rules and thresholds
- Per-repository configuration support via `.review-agent.yml`

### Critical Integration Points

**GitHub API**: Uses both REST and GraphQL APIs with proper rate limiting (5000 req/hr). Authentication via Personal Access Token (MVP) or GitHub App (production).

**Claude API**: Structured prompts with JSON response format. Implements retry logic and token usage tracking for cost management.

**Webhook Security**: HMAC-SHA256 signature validation prevents unauthorized requests.

### Error Handling Strategy
- Exponential backoff for API rate limits
- Graceful degradation when LLM services are unavailable  
- Structured logging with correlation IDs for request tracing
- Health checks for container orchestration

### Future Extensibility
- Plugin system for specialized review types (security, performance)
- Multiple LLM provider support via interface abstraction
- Custom prompt templates per programming language
- Advanced filtering rules to prevent spam comments