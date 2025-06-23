# Review Agent

A Go-based code review agent that processes GitHub pull requests using Claude AI. The agent runs in Docker containers for CI/CD integration, receives webhook events from GitHub, analyzes code changes, and posts review comments back to pull requests.

## Quick Start

### 1. Environment Setup

```bash
# Create .env file with your API keys
make init-env
cp .env.example .env
# Edit .env with your actual GitHub token and Claude API key
```

### 2. Development

```bash
# Run all tests
make test

# Build the application
make build

# Start development server with hot reload
make dev

# Start production server in Docker
make docker-run
```

### 3. Usage

#### CLI Mode (Direct PR Review)

**Local Binary:**
```bash
# Review a specific pull request
./bin/review-agent review --owner myorg --repo myrepo --pr 123
```

**Docker (Production):**
```bash
# Review using Docker with production image
make review-docker OWNER=myorg REPO=myrepo PR=123

# Or specify custom .env file
make review-docker OWNER=myorg REPO=myrepo PR=123 ENV_FILE=.env.production.local

# Direct docker command
docker run --rm --env-file .env.production.local review-agent:latest \
  ./review-agent review --owner myorg --repo myrepo --pr 123
```

**Docker (Development):**
```bash
# Review using development Docker with hot reload
make review-dev-docker OWNER=myorg REPO=myrepo PR=123

# Direct docker command with volume mount
docker run --rm --env-file .env.development.local \
  -v "$(pwd):/app" -w /app \
  review-agent:dev \
  go run cmd/agent/main.go review --owner myorg --repo myrepo --pr 123
```

#### Server Mode (Webhook Integration)
```bash
# Start webhook server
./bin/review-agent server

# Or with Docker
make docker-run
```

## Configuration

The application supports multiple configuration methods with the following precedence:

1. **Command line flags** (highest precedence)
2. **Environment variables**
3. **.env file** (lowest precedence)

### Required Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `GH_TOKEN` | GitHub Personal Access Token | `ghp_xxxxxxxxxxxx` |
| `CLAUDE_API_KEY` | Claude API Key from Anthropic | `sk-ant-xxxxxxxxxxxx` |
| `WEBHOOK_SECRET` | GitHub webhook secret (server mode only) | `your-secret-here` |

### Optional Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DEV_PORT` | `8081` | Development server port |

## Development Commands

```bash
# Setup and Building
make install-deps     # Install Go dependencies
make build            # Build binary
make docker-build     # Build Docker image

# Environment Setup
make init-env         # Show environment setup options
make init-dev         # Create .env.development.local
make init-prod        # Create .env.production.local

# CLI Commands
make review-docker OWNER=org REPO=repo PR=123    # Run review CLI in Docker
make review-dev-docker OWNER=org REPO=repo PR=123 # Run review CLI in dev Docker

# Testing and Quality
make test             # Run all tests
make test-coverage    # Run tests with coverage
make lint             # Run linter
make fmt              # Format code
make vet              # Run go vet
make check            # Run all checks (fmt, vet, lint, test)

# Development Environment
make dev              # Start development with hot reload
make dev-detached     # Start development (background)
make dev-stop         # Stop development environment

# Production Environment
make docker-run       # Run in Docker
make docker-run-detached  # Run in Docker (background)

# Testing
make test-webhook     # Test webhook endpoint
make test-health      # Test health endpoint

# Utilities
make logs             # Show application logs
make logs-dev         # Show development logs
make shell            # Get shell in container
make clean            # Clean up containers and artifacts
make help             # Show all commands
```

## Architecture

### Core Components

```
pkg/
├── webhook/     # GitHub webhook handling and validation
├── github/      # GitHub API client and operations
├── llm/         # LLM provider interface (Claude + future providers)
├── analyzer/    # Code diff parsing and context extraction
├── review/      # Review orchestration and business logic
└── store/       # Database layer and state management
```

### Key Features

- **Token-based GitHub API authentication**
- **HMAC-SHA256 webhook signature validation**
- **Layered architecture with clear boundaries**
- **Dependency injection and interface-based mocking**
- **Temporary workspace management with automatic cleanup**
- **Configuration precedence with .env file support**
- **Docker support with development and production images**

## API Endpoints

### Webhook Server

- `POST /webhook` - GitHub webhook endpoint
- `GET /health` - Health check endpoint

## Docker Development

### Production Container
```bash
# Build and run production container
make docker-build
make docker-run

# Test the running container
make test-health
make test-webhook
```

### Development Container with Hot Reload
```bash
# Start development environment
make dev

# View logs
make logs-dev

# Get shell access
make shell-dev
```

### Docker Compose Profiles

- **Default**: Production server
- **dev**: Development server with hot reload
- **test**: Testing utilities

```bash
# Start specific profile
docker-compose --profile dev up
docker-compose --profile test up
```

## Testing

### Webhook Testing

The application includes comprehensive webhook testing:

```bash
# Start the server
make docker-run-detached

# Test webhook endpoints
WEBHOOK_SECRET=your-secret make test-webhook

# Expected test results:
# ✓ Health check passed
# ✓ Webhook correctly processes valid requests
# ✓ Webhook rejects requests without valid signatures
# ✓ Ping events are handled
```

### Unit Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage
```

## Configuration Examples

### .env File
```bash
# GitHub Personal Access Token
GH_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx

# Claude API Key
CLAUDE_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxx

# Webhook Secret (for server mode)
WEBHOOK_SECRET=your-webhook-secret-here

# Optional: Custom port
PORT=3000
```

### Environment Variables
```bash
export GH_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
export CLAUDE_API_KEY="sk-ant-xxxxxxxxxxxxxxxxxxxx"
export WEBHOOK_SECRET="your-webhook-secret-here"
```

### Command Line Flags
```bash
./bin/review-agent server \
  --github-token "ghp_xxxxxxxxxxxxxxxxxxxx" \
  --claude-key "sk-ant-xxxxxxxxxxxxxxxxxxxx" \
  --webhook-secret "your-webhook-secret-here" \
  --port 3000
```

## GitHub Action Usage

The review agent is available as a GitHub Action for easy integration into your workflows. The action uses a pre-built Docker container for fast execution and reliability.

### Quick Start

Add this to `.github/workflows/code-review.yml` in your repository:

```yaml
name: Automated Code Review
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
```

> **Note**: Replace `gdormoy/review-agent@v1` with the actual path to this action once published.

### Action Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `github-token` | ✅ | - | GitHub token for API access |
| `claude-api-key` | ✅ | - | Claude API key for AI reviews |
| `claude-model` | ❌ | `claude-sonnet-4-20250514` | Claude model to use |
| `pr-number` | ❌ | Auto-detected | Pull request number to review |
| `repository` | ❌ | Auto-detected | Repository in format owner/repo |
| `skip-draft` | ❌ | `true` | Skip review for draft PRs |
| `skip-labels` | ❌ | `skip-review,wip` | Labels that skip review |
| `review-paths` | ❌ | All files | Paths to review (e.g., `src/**/*.go`) |
| `exclude-paths` | ❌ | `vendor/**,node_modules/**` | Paths to exclude |
| `comment-threshold` | ❌ | `0.7` | Minimum confidence for comments |

### Action Outputs

| Output | Description |
|--------|-------------|
| `review-status` | Status of the review (success, skipped, failed) |
| `review-url` | URL to the pull request review |
| `comments-posted` | Number of review comments posted |

### Example Workflows

#### Advanced Configuration

```yaml
name: Code Review with Custom Settings
on:
  pull_request:
    paths:
      - 'src/**'
      - 'pkg/**'

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
          claude-model: 'claude-3-7-sonnet-20250109'
          skip-draft: 'false'  # Review draft PRs
          review-paths: 'src/**,pkg/**'
          exclude-paths: 'vendor/**,**/*_test.go'
          comment-threshold: '0.8'
```

#### Manual Review Trigger

```yaml
name: Manual Code Review
on:
  workflow_dispatch:
    inputs:
      pr-number:
        description: 'PR number to review'
        required: true
        type: number

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
          pr-number: ${{ inputs.pr-number }}
```

#### Comment-Triggered Review

```yaml
name: Review on Comment
on:
  issue_comment:
    types: [created]

jobs:
  review:
    if: |
      github.event.issue.pull_request &&
      contains(github.event.comment.body, '/review')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
```

### Comment Commands

When using the comment-triggered workflow, you can use these commands in PR comments:

- `/review` - Trigger a basic review
- `/review model:claude-3-7-haiku-20250109` - Use a specific model
- `/review paths:src/**,pkg/**` - Review specific paths only
- `/review --force` - Force review even for draft PRs

### Setting Up the Action

1. **Add Secrets**: In your repository settings, add the `CLAUDE_API_KEY` secret
2. **Create Workflow**: Add one of the example workflows above to `.github/workflows/`
3. **Customize**: Adjust the configuration to match your needs

### Using in Other Repositories

To use this review agent in any GitHub repository:

1. **No Installation Required**: The action uses a pre-built Docker container from GitHub Container Registry
2. **Fast Execution**: No build time - the container is already optimized and ready to use
3. **Simply Reference**: Use `gdormoy/review-agent@v1` in your workflow

**Example for any Go project:**
```yaml
name: AI Code Review
on:
  pull_request:
    types: [opened, synchronize]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
          review-paths: '**/*.go'
          exclude-paths: 'vendor/**,**/*_test.go'
```

**Example for any JavaScript/TypeScript project:**
```yaml
name: AI Code Review
on: [pull_request]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: gdormoy/review-agent@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          claude-api-key: ${{ secrets.CLAUDE_API_KEY }}
          review-paths: 'src/**/*.{js,ts,jsx,tsx}'
          exclude-paths: 'node_modules/**,dist/**,build/**'
```

### Security Considerations

- Never commit API keys directly to your repository
- Use GitHub Secrets for sensitive values
- The `GH_TOKEN` is automatically provided by GitHub Actions
- Review the action's permissions in your workflow

## GitHub Integration

### Setting Up Webhooks

1. Go to your repository settings
2. Navigate to Webhooks
3. Add webhook with:
   - **Payload URL**: `https://your-domain.com/webhook`
   - **Content type**: `application/json`
   - **Secret**: Use the same value as `WEBHOOK_SECRET`
   - **Events**: Select "Pull requests"

### Required GitHub Token Permissions

Your GitHub Personal Access Token needs these scopes:
- `repo` - Full repository access
- `pull_requests` - Pull request access

## Troubleshooting

### Common Issues

1. **Docker build fails**: Ensure Go version compatibility
2. **Webhook signature validation fails**: Check `WEBHOOK_SECRET` matches GitHub
3. **Repository clone fails**: Verify GitHub token permissions
4. **Tests fail**: Run `make clean` and retry

### Development Tips

- Use `make dev` for hot reloading during development
- Check `make logs-dev` for development server logs
- Use `make test-webhook` to verify webhook integration
- Run `make check` before committing changes

## License

[Add your license information here]