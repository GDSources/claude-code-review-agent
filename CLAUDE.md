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
GH_TOKEN=your_token CLAUDE_API_KEY=your_key ./bin/review-agent
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
  -e GH_TOKEN="${GH_TOKEN}" \
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

## Git Workflow

### Feature Development Process

**IMPORTANT**: Always use feature branches and pull requests for development work.

#### 1. Start New Feature
```bash
# Create and switch to feature branch
git checkout -b feature/your-feature-name

# Alternative: bug fixes
git checkout -b fix/issue-description

# Alternative: documentation updates  
git checkout -b docs/update-description
```

#### 2. Development Cycle
```bash
# Make your changes and commit regularly
git add .

# MANDATORY: Run Final Verification before committing (see Pre-Commit Quality Verification section)
# This ensures all formatting, tests, and quality checks pass

git commit -m "feat: implement new feature

- Add specific functionality
- Update tests and documentation
- Ensure all tests pass"
```

#### 3. Create Pull Request
```bash
# Push feature branch to remote
git push -u origin feature/your-feature-name

# Create pull request via GitHub CLI (if available)
gh pr create --title "Add your feature" --body "Description of changes"

# Or create PR manually via GitHub web interface
```

#### 4. Code Review Process
- **MANDATORY**: Run Final Verification script before creating/updating PR
- Ensure all tests pass and code quality checks succeed
- Request review from team members
- Address feedback and update PR (re-run verification after changes)
- Merge only after approval and final verification

#### 5. Cleanup
```bash
# After PR is merged, cleanup local branches
git checkout main
git pull origin main
git branch -d feature/your-feature-name
```

### Branch Naming Conventions
- `feature/feature-name` - New features
- `fix/bug-description` - Bug fixes  
- `docs/update-type` - Documentation updates
- `refactor/component-name` - Code refactoring
- `test/test-description` - Test improvements

### Commit Message Format
Follow conventional commits format:
```
type(scope): brief description

Detailed explanation if needed
- Bullet points for multiple changes
- Reference issues: Fixes #123
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`

## Pre-Commit Quality Verification

**MANDATORY**: Before committing any task or changes, you MUST run the Final Verification script to ensure code quality and functionality.

### Final Verification Script
Always run this comprehensive check before any commit:

```bash
# Easy to use - just run the verification script
./scripts/verify.sh
```

The script performs the following checks:
1. **📝 Code Formatting**: Ensures all Go code follows standard formatting
2. **🧪 Test Suite**: Verifies all tests pass
3. **🔍 Static Analysis**: Runs `go vet` to catch potential issues
4. **🏗️ Build Verification**: Confirms the project compiles successfully
5. **📊 Git Status**: Shows current repository state
6. **📦 Dependency Check**: Ensures dependencies are current and tidy

#### Manual Verification (if script unavailable)
If the script is not available, run these commands manually:

```bash
# Check formatting
gofmt -s -l .

# Run tests  
go test ./...

# Static analysis
go vet ./...

# Build check
go build cmd/agent/main.go

# Dependencies
go mod tidy
```

### Quality Requirements

Before any commit, ALL of the following MUST pass:

1. **✅ Code Formatting**: `gofmt -s -l . | wc -l` returns 0
2. **✅ Test Suite**: `go test ./...` exits with code 0 (all tests pass)
3. **✅ Static Analysis**: `go vet ./...` exits with code 0 (no issues)
4. **✅ Build Success**: Code compiles without errors

### Failure Handling

If ANY verification check fails:
1. **DO NOT COMMIT** until all issues are resolved
2. Fix formatting with: `gofmt -s -w .`
3. Fix failing tests by debugging and updating code/tests
4. Fix vet issues by addressing static analysis warnings
5. Re-run the verification script until all checks pass

### Integration with Development Workflow

The verification script should be run:
- ✅ Before every `git commit`
- ✅ Before pushing to remote (`git push`)
- ✅ Before creating or updating pull requests
- ✅ After fixing any CI/CD pipeline failures
- ✅ As part of local development best practices

### Automation Suggestion

Consider adding this as a git pre-commit hook:
```bash
# Save the verification script as .git/hooks/pre-commit
# Make it executable: chmod +x .git/hooks/pre-commit
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