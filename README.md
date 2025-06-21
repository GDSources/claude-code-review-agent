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
| `GITHUB_TOKEN` | GitHub Personal Access Token | `ghp_xxxxxxxxxxxx` |
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
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx

# Claude API Key
CLAUDE_API_KEY=sk-ant-xxxxxxxxxxxxxxxxxxxx

# Webhook Secret (for server mode)
WEBHOOK_SECRET=your-webhook-secret-here

# Optional: Custom port
PORT=3000
```

### Environment Variables
```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
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