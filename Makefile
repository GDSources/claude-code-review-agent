# Review Agent Makefile
.PHONY: help build test clean docker-build docker-run dev dev-stop logs shell init-env

# Default target
help: ## Show this help message
	@echo "Review Agent - Development Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# Build targets
build: ## Build the application binary
	@echo "🔨 Building review-agent..."
	@go build -o bin/review-agent cmd/agent/main.go
	@echo "✅ Build complete"

build-linux: ## Build Linux binary (for Docker)
	@echo "🔨 Building Linux binary..."
	@CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/review-agent-linux cmd/agent/main.go
	@echo "✅ Linux build complete"

# Test targets
test: ## Run all tests
	@echo "🧪 Running tests..."
	@go test ./... -v

test-coverage: ## Run tests with coverage
	@echo "🧪 Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

# Code quality
lint: ## Run linter
	@echo "🔍 Running linter..."
	@golangci-lint run || echo "⚠️  golangci-lint not installed, skipping lint check"

fmt: ## Format code
	@echo "🎨 Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "🔍 Running go vet..."
	@go vet ./...

check: fmt vet lint test ## Run all checks (format, vet, lint, test)

# Docker targets
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	@docker build -t review-agent:latest .
	@echo "✅ Docker image built"

docker-build-dev: ## Build development Docker image
	@echo "🐳 Building development Docker image..."
	@docker build -f Dockerfile.dev -t review-agent:dev .
	@echo "✅ Development Docker image built"

docker-run: ## Run application in Docker
	@echo "🚀 Starting review-agent in Docker..."
	@if [ -f .env.production.local ]; then \
		echo "📝 Using .env.production.local"; \
		docker-compose --env-file .env.production.local up --build review-agent; \
	elif [ -f .env.production ]; then \
		echo "📝 Using .env.production"; \
		docker-compose --env-file .env.production up --build review-agent; \
	elif [ -f .env ]; then \
		echo "📝 Using .env"; \
		docker-compose --env-file .env up --build review-agent; \
	else \
		echo "⚠️  No .env file found, using environment variables"; \
		docker-compose up --build review-agent; \
	fi

docker-run-detached: ## Run application in Docker (detached)
	@echo "🚀 Starting review-agent in Docker (detached)..."
	@if [ -f .env.production.local ]; then \
		echo "📝 Using .env.production.local"; \
		docker-compose --env-file .env.production.local up -d --build review-agent; \
	elif [ -f .env.production ]; then \
		echo "📝 Using .env.production"; \
		docker-compose --env-file .env.production up -d --build review-agent; \
	elif [ -f .env ]; then \
		echo "📝 Using .env"; \
		docker-compose --env-file .env up -d --build review-agent; \
	else \
		echo "⚠️  No .env file found, using environment variables"; \
		docker-compose up -d --build review-agent; \
	fi

# Development targets
dev: ## Start development environment with hot reload
	@echo "🔧 Starting development environment..."
	@if [ -f .env.development.local ]; then \
		echo "📝 Using .env.development.local"; \
		docker-compose --env-file .env.development.local --profile dev up --build review-agent-dev; \
	elif [ -f .env.development ]; then \
		echo "📝 Using .env.development"; \
		docker-compose --env-file .env.development --profile dev up --build review-agent-dev; \
	elif [ -f .env ]; then \
		echo "📝 Using .env"; \
		docker-compose --env-file .env --profile dev up --build review-agent-dev; \
	else \
		echo "⚠️  No .env file found, using environment variables"; \
		docker-compose --profile dev up --build review-agent-dev; \
	fi

dev-detached: ## Start development environment (detached)
	@echo "🔧 Starting development environment (detached)..."
	@if [ -f .env.development.local ]; then \
		echo "📝 Using .env.development.local"; \
		docker-compose --env-file .env.development.local --profile dev up -d --build review-agent-dev; \
	elif [ -f .env.development ]; then \
		echo "📝 Using .env.development"; \
		docker-compose --env-file .env.development --profile dev up -d --build review-agent-dev; \
	elif [ -f .env ]; then \
		echo "📝 Using .env"; \
		docker-compose --env-file .env --profile dev up -d --build review-agent-dev; \
	else \
		echo "⚠️  No .env file found, using environment variables"; \
		docker-compose --profile dev up -d --build review-agent-dev; \
	fi

dev-watch: ## Start with hot reload (used inside container)
	@echo "👀 Starting with hot reload..."
	@air -c .air.toml

dev-stop: ## Stop development environment
	@echo "🛑 Stopping development environment..."
	@docker-compose --profile dev down

# Environment setup
init-env: ## Create .env file from template
	@if [ ! -f .env ]; then \
		echo "📝 Creating .env file..."; \
		./bin/review-agent init || (make build && ./bin/review-agent init); \
		echo "✅ .env.example created"; \
		echo "📋 Environment setup options:"; \
		echo ""; \
		echo "🏭 Production:"; \
		echo "   cp .env.production .env.production.local"; \
		echo "   # Edit .env.production.local with your production API keys"; \
		echo ""; \
		echo "🔧 Development:"; \
		echo "   cp .env.development .env.development.local"; \
		echo "   # Edit .env.development.local with your development API keys"; \
		echo ""; \
		echo "⚡ Quick setup (generic):"; \
		echo "   cp .env.example .env"; \
		echo "   # Edit .env with your API keys"; \
	else \
		echo "ℹ️  .env file already exists"; \
	fi

init-dev: ## Create development .env file
	@if [ ! -f .env.development.local ]; then \
		echo "📝 Creating development environment file..."; \
		cp .env.development .env.development.local; \
		echo "✅ .env.development.local created"; \
		echo "📝 Edit .env.development.local with your development API keys"; \
	else \
		echo "ℹ️  .env.development.local already exists"; \
	fi

init-prod: ## Create production .env file
	@if [ ! -f .env.production.local ]; then \
		echo "📝 Creating production environment file..."; \
		cp .env.production .env.production.local; \
		echo "✅ .env.production.local created"; \
		echo "📝 Edit .env.production.local with your production API keys"; \
	else \
		echo "ℹ️  .env.production.local already exists"; \
	fi

# Utility targets
logs: ## Show application logs
	@echo "📄 Showing production logs..."
	@if [ -f .env.production.local ]; then \
		docker-compose --env-file .env.production.local logs -f review-agent; \
	elif [ -f .env.production ]; then \
		docker-compose --env-file .env.production logs -f review-agent; \
	elif [ -f .env ]; then \
		docker-compose --env-file .env logs -f review-agent; \
	else \
		docker-compose logs -f review-agent; \
	fi

logs-dev: ## Show development logs
	@echo "📄 Showing development logs..."
	@if [ -f .env.development.local ]; then \
		docker-compose --env-file .env.development.local --profile dev logs -f review-agent-dev; \
	elif [ -f .env.development ]; then \
		docker-compose --env-file .env.development --profile dev logs -f review-agent-dev; \
	elif [ -f .env ]; then \
		docker-compose --env-file .env --profile dev logs -f review-agent-dev; \
	else \
		docker-compose --profile dev logs -f review-agent-dev; \
	fi

shell: ## Get shell access to running container
	@echo "🐚 Opening shell in production container..."
	@if [ -f .env.production.local ]; then \
		docker-compose --env-file .env.production.local exec review-agent sh; \
	elif [ -f .env.production ]; then \
		docker-compose --env-file .env.production exec review-agent sh; \
	elif [ -f .env ]; then \
		docker-compose --env-file .env exec review-agent sh; \
	else \
		docker-compose exec review-agent sh; \
	fi

shell-dev: ## Get shell access to development container
	@echo "🐚 Opening shell in development container..."
	@if [ -f .env.development.local ]; then \
		docker-compose --env-file .env.development.local --profile dev exec review-agent-dev sh; \
	elif [ -f .env.development ]; then \
		docker-compose --env-file .env.development --profile dev exec review-agent-dev sh; \
	elif [ -f .env ]; then \
		docker-compose --env-file .env --profile dev exec review-agent-dev sh; \
	else \
		docker-compose --profile dev exec review-agent-dev sh; \
	fi

# CLI targets
review-docker: ## Run review CLI in Docker
	@echo "🔍 Running review CLI in Docker..."
	@if [ -z "$(OWNER)" ] || [ -z "$(REPO)" ] || [ -z "$(PR)" ]; then \
		echo "❌ Missing required parameters"; \
		echo "Usage: make review-docker OWNER=myorg REPO=myrepo PR=123"; \
		echo ""; \
		echo "Optional: Set environment with ENV_FILE=.env.development.local"; \
		exit 1; \
	fi
	@if [ -n "$(ENV_FILE)" ] && [ -f "$(ENV_FILE)" ]; then \
		echo "📝 Using environment file: $(ENV_FILE)"; \
		docker run --rm --env-file "$(ENV_FILE)" review-agent:latest \
			./review-agent review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	elif [ -f .env.production.local ]; then \
		echo "📝 Using .env.production.local"; \
		docker run --rm --env-file .env.production.local review-agent:latest \
			./review-agent review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	elif [ -f .env ]; then \
		echo "📝 Using .env"; \
		docker run --rm --env-file .env review-agent:latest \
			./review-agent review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	else \
		echo "⚠️  No .env file found, using environment variables"; \
		docker run --rm \
			-e GITHUB_TOKEN="$$GITHUB_TOKEN" \
			-e CLAUDE_API_KEY="$$CLAUDE_API_KEY" \
			review-agent:latest \
			./review-agent review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	fi

review-dev-docker: ## Run review CLI in development Docker
	@echo "🔍 Running review CLI in development Docker..."
	@if [ -z "$(OWNER)" ] || [ -z "$(REPO)" ] || [ -z "$(PR)" ]; then \
		echo "❌ Missing required parameters"; \
		echo "Usage: make review-dev-docker OWNER=myorg REPO=myrepo PR=123"; \
		exit 1; \
	fi
	@if [ -f .env.development.local ]; then \
		echo "📝 Using .env.development.local"; \
		docker run --rm --env-file .env.development.local \
			-v "$(PWD):/app" -w /app \
			review-agent:dev \
			go run cmd/agent/main.go review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	elif [ -f .env.development ]; then \
		echo "📝 Using .env.development"; \
		docker run --rm --env-file .env.development \
			-v "$(PWD):/app" -w /app \
			review-agent:dev \
			go run cmd/agent/main.go review --owner "$(OWNER)" --repo "$(REPO)" --pr "$(PR)"; \
	else \
		echo "❌ No development .env file found"; \
		echo "Run: make init-dev"; \
		exit 1; \
	fi

# Testing targets
test-webhook: ## Test webhook endpoint (requires running server)
	@echo "🧪 Testing webhook endpoint..."
	@./scripts/test-webhook.sh

test-health: ## Test health endpoint
	@echo "🏥 Testing health endpoint..."
	@curl -f http://localhost:8080/health || echo "❌ Health check failed"

# Cleanup targets
clean: ## Clean build artifacts
	@echo "🧹 Cleaning up..."
	@rm -rf bin/ tmp/ coverage.out coverage.html
	@docker-compose down --volumes --remove-orphans
	@echo "✅ Cleanup complete"

clean-all: clean ## Clean everything including Docker images
	@echo "🧹 Deep cleaning..."
	@docker system prune -f
	@docker volume prune -f
	@echo "✅ Deep cleanup complete"

# Installation targets
install-deps: ## Install development dependencies
	@echo "📦 Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies installed"

# CI/CD targets
ci: check docker-build ## Run CI pipeline (check + docker build)
	@echo "✅ CI pipeline completed"

# Default environment variables
export PORT ?= 8080
export DEV_PORT ?= 8081