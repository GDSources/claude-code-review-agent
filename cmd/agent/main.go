package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/your-org/review-agent/pkg/cli"
	"github.com/your-org/review-agent/pkg/github"
	"github.com/your-org/review-agent/pkg/llm"
	"github.com/your-org/review-agent/pkg/review"
	"github.com/your-org/review-agent/pkg/webhook"
)

const (
	Version = "1.0.0"
)

type Config struct {
	GitHubToken   string
	ClaudeAPIKey  string
	ClaudeModel   string
	WebhookSecret string
}

type ServerConfig struct {
	GitHubToken   string
	ClaudeAPIKey  string
	ClaudeModel   string
	WebhookSecret string
	Port          int
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "review":
		runReview(os.Args[2:])
	case "server":
		runServer(os.Args[2:])
	case "init":
		runInit(os.Args[2:])
	case "version":
		runVersion(os.Args[2:])
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`review-agent - GitHub PR review automation tool

Usage:
  review-agent <command> [flags]

Commands:
  review      Review a specific pull request
  server      Start webhook server for automated reviews
  init        Create a sample .env file for configuration
  version     Show version information
  help        Show this help message

Use "review-agent <command> --help" for more information about a command.
`)
}

func runReview(args []string) {
	fs := flag.NewFlagSet("review", flag.ExitOnError)

	config := &Config{}
	var owner, repo string
	var prNumber int

	fs.StringVar(&config.GitHubToken, "github-token", "", "GitHub API token")
	fs.StringVar(&config.ClaudeAPIKey, "claude-key", "", "Claude API key")
	fs.StringVar(&config.ClaudeModel, "claude-model", "", "Claude model to use")
	fs.StringVar(&owner, "owner", "", "Repository owner/organization")
	fs.StringVar(&repo, "repo", "", "Repository name")
	fs.IntVar(&prNumber, "pr", 0, "Pull request number")

	fs.Usage = func() {
		fmt.Print(`Review a specific pull request

Usage:
  review-agent review [flags]

Flags:
  --github-token    GitHub API token (or set GITHUB_TOKEN env var)
  --claude-key      Claude API key (or set CLAUDE_API_KEY env var)
  --claude-model    Claude model to use (or set CLAUDE_MODEL env var, default: claude-sonnet-4-20250514)
  --owner           Repository owner/organization (required)
  --repo            Repository name (required)
  --pr              Pull request number (required)

Available Claude Models:
  claude-3-5-haiku-20241022     Fast and cost-effective, good for simple reviews
  claude-3-5-sonnet-20241022    Balanced performance and cost
  claude-3-5-sonnet-20250106    Enhanced capabilities with recent improvements
  claude-3-7-haiku-20250109     Fast and efficient with improved reasoning
  claude-3-7-sonnet-20250109    Enhanced capabilities and better performance
  claude-sonnet-4-20250514      Most capable model, best for complex analysis (recommended)

Configuration:
  The tool loads configuration in this order (highest precedence first):
  1. Command line flags
  2. Environment variables
  3. .env file (searched in current directory, home directory, ~/.config/review-agent/)

Examples:
  # Use .env file for configuration
  review-agent init                                    # Create sample .env file
  review-agent review --owner myorg --repo myrepo --pr 123

  # Use environment variables
  GITHUB_TOKEN=xxx CLAUDE_API_KEY=yyy review-agent review --owner myorg --repo myrepo --pr 123

  # Use command line flags
  review-agent review --github-token xxx --claude-key yyy --owner myorg --repo myrepo --pr 123
`)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if err := loadEnvConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if err := validateReviewConfig(config, owner, repo, prNumber); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if err := executeReview(config, owner, repo, prNumber); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Review failed: %v\n", err)
		os.Exit(1)
	}
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Print(`Create a sample .env file for configuration

Usage:
  review-agent init

This command creates a .env.example file in the current directory with
sample configuration values. Copy it to .env and update with your actual
API keys.
`)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	envLoader := cli.NewEnvLoader()
	if err := envLoader.CreateSampleEnvFile(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create .env.example file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Created .env.example file")
	fmt.Println("📝 Copy it to .env and update with your actual API keys:")
	fmt.Println("   cp .env.example .env")
	fmt.Println("   # Edit .env with your GitHub token and Claude API key")
}

func runVersion(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Print(`Show version information

Usage:
  review-agent version
`)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("review-agent version %s\n", Version)
}

func loadEnvConfig(config *Config) error {
	// First, load from .env file (lowest precedence)
	envLoader := cli.NewEnvLoader()
	if err := envLoader.LoadEnvFile(); err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}

	// Then apply environment variables (higher precedence than .env file)
	if config.GitHubToken == "" {
		config.GitHubToken = os.Getenv("GITHUB_TOKEN")
	}
	if config.ClaudeAPIKey == "" {
		config.ClaudeAPIKey = os.Getenv("CLAUDE_API_KEY")
	}
	if config.ClaudeModel == "" {
		config.ClaudeModel = os.Getenv("CLAUDE_MODEL")
	}
	if config.WebhookSecret == "" {
		config.WebhookSecret = os.Getenv("WEBHOOK_SECRET")
	}

	return nil
}

func validateReviewConfig(config *Config, owner, repo string, prNumber int) error {
	if config.GitHubToken == "" {
		return fmt.Errorf("GitHub token is required (set --github-token flag, GITHUB_TOKEN env var, or add to .env file)")
	}
	if config.ClaudeAPIKey == "" {
		return fmt.Errorf("Claude API key is required (set --claude-key flag, CLAUDE_API_KEY env var, or add to .env file)")
	}
	if owner == "" {
		return fmt.Errorf("repository owner is required (set --owner flag)")
	}
	if repo == "" {
		return fmt.Errorf("repository name is required (set --repo flag)")
	}
	if prNumber <= 0 {
		return fmt.Errorf("valid pull request number is required (set --pr flag)")
	}
	return nil
}

func executeReview(config *Config, owner, repo string, prNumber int) error {
	fmt.Printf("🔍 Starting review for PR #%d in %s/%s...\n", prNumber, owner, repo)

	// Create reviewer with configuration
	reviewConfig := &cli.ReviewConfig{
		GitHubToken:  config.GitHubToken,
		ClaudeAPIKey: config.ClaudeAPIKey,
		ClaudeModel:  config.ClaudeModel,
	}

	reviewer := cli.NewPRReviewer(reviewConfig)

	// Execute the review
	if err := reviewer.ReviewPR(owner, repo, prNumber); err != nil {
		return err
	}

	return nil
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)

	serverConfig := &ServerConfig{
		Port: 8080, // Default port
	}

	fs.StringVar(&serverConfig.GitHubToken, "github-token", "", "GitHub API token")
	fs.StringVar(&serverConfig.ClaudeAPIKey, "claude-key", "", "Claude API key")
	fs.StringVar(&serverConfig.ClaudeModel, "claude-model", "", "Claude model to use")
	fs.StringVar(&serverConfig.WebhookSecret, "webhook-secret", "", "GitHub webhook secret")
	fs.IntVar(&serverConfig.Port, "port", 8080, "Server port")

	fs.Usage = func() {
		fmt.Print(`Start webhook server for automated PR reviews

Usage:
  review-agent server [flags]

Flags:
  --github-token     GitHub API token (or set GITHUB_TOKEN env var)
  --claude-key       Claude API key (or set CLAUDE_API_KEY env var)
  --claude-model     Claude model to use (or set CLAUDE_MODEL env var, default: claude-sonnet-4-20250514)
  --webhook-secret   GitHub webhook secret (or set WEBHOOK_SECRET env var)
  --port             Server port (default: 8080)

Available Claude Models:
  claude-3-5-haiku-20241022     Fast and cost-effective, good for simple reviews
  claude-3-5-sonnet-20241022    Balanced performance and cost
  claude-3-5-sonnet-20250106    Enhanced capabilities with recent improvements
  claude-3-7-haiku-20250109     Fast and efficient with improved reasoning
  claude-3-7-sonnet-20250109    Enhanced capabilities and better performance
  claude-sonnet-4-20250514      Most capable model, best for complex analysis (recommended)

Configuration:
  The server loads configuration in this order (highest precedence first):
  1. Command line flags
  2. Environment variables
  3. .env file (searched in current directory, home directory, ~/.config/review-agent/)

Examples:
  # Use .env file for configuration
  review-agent init                    # Create sample .env file
  review-agent server                  # Start server with .env config

  # Use environment variables
  GITHUB_TOKEN=xxx CLAUDE_API_KEY=yyy WEBHOOK_SECRET=zzz review-agent server

  # Use command line flags
  review-agent server --github-token xxx --claude-key yyy --webhook-secret zzz --port 3000
`)
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if err := loadServerConfig(serverConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if err := validateServerConfig(serverConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if err := startWebhookServer(serverConfig); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server failed: %v\n", err)
		os.Exit(1)
	}
}

func loadServerConfig(config *ServerConfig) error {
	// First, load from .env file (lowest precedence)
	envLoader := cli.NewEnvLoader()
	if err := envLoader.LoadEnvFile(); err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}

	// Then apply environment variables (higher precedence than .env file)
	if config.GitHubToken == "" {
		config.GitHubToken = os.Getenv("GITHUB_TOKEN")
	}
	if config.ClaudeAPIKey == "" {
		config.ClaudeAPIKey = os.Getenv("CLAUDE_API_KEY")
	}
	if config.ClaudeModel == "" {
		config.ClaudeModel = os.Getenv("CLAUDE_MODEL")
	}
	if config.WebhookSecret == "" {
		config.WebhookSecret = os.Getenv("WEBHOOK_SECRET")
	}

	// Port can also come from env var
	if portStr := os.Getenv("PORT"); portStr != "" && config.Port == 8080 { // Only override default
		if port, err := strconv.Atoi(portStr); err == nil {
			config.Port = port
		}
	}

	return nil
}

func validateServerConfig(config *ServerConfig) error {
	if config.GitHubToken == "" {
		return fmt.Errorf("GitHub token is required (set --github-token flag, GITHUB_TOKEN env var, or add to .env file)")
	}
	if config.ClaudeAPIKey == "" {
		return fmt.Errorf("Claude API key is required (set --claude-key flag, CLAUDE_API_KEY env var, or add to .env file)")
	}
	if config.WebhookSecret == "" {
		return fmt.Errorf("webhook secret is required (set --webhook-secret flag, WEBHOOK_SECRET env var, or add to .env file)")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("invalid port number: %d (must be between 1 and 65535)", config.Port)
	}
	return nil
}

func startWebhookServer(config *ServerConfig) error {
	fmt.Printf("🚀 Starting webhook server on port %d...\n", config.Port)

	// Create GitHub client
	githubClient := github.NewClient(config.GitHubToken)

	// Create GitHub cloner adapter
	cloner := review.NewGitHubClonerAdapterFromClient(githubClient)

	// Create file system manager
	fsManager := review.NewDefaultFileSystemManager()

	// Create workspace manager
	workspaceManager := review.NewDefaultWorkspaceManager(cloner, fsManager)

	// Create diff fetcher
	diffFetcher := review.NewGitHubDiffFetcherFromClient(githubClient)

	// Create code analyzer
	codeAnalyzer := review.NewDefaultAnalyzerAdapter()

	// Create Claude client for LLM reviews
	var claudeClient llm.CodeReviewer
	if config.ClaudeAPIKey != "" {
		model := config.ClaudeModel
		if model == "" {
			model = llm.DefaultClaudeModel
		}
		
		claudeConfig := llm.ClaudeConfig{
			APIKey:      config.ClaudeAPIKey,
			Model:       model,
			MaxTokens:   llm.DefaultClaudeMaxTokens,
			Temperature: llm.DefaultClaudeTemperature,
			BaseURL:     llm.DefaultClaudeBaseURL,
			Timeout:     llm.DefaultTimeoutSeconds,
		}
		
		var err error
		claudeClient, err = llm.NewClaudeClient(claudeConfig)
		if err != nil {
			fmt.Printf("Warning: Failed to create Claude client: %v\n", err)
			claudeClient = nil
		}
	} else {
		fmt.Printf("Warning: CLAUDE_API_KEY not provided, LLM reviews will be skipped\n")
	}

	// Create review orchestrator with LLM and comment posting integration
	orchestrator := review.NewReviewOrchestratorWithComments(workspaceManager, diffFetcher, codeAnalyzer, claudeClient, githubClient)

	// Create event processor
	eventProcessor := webhook.NewGitHubEventProcessor(orchestrator)

	// Create HMAC validator
	validator := webhook.NewHMACValidator(config.WebhookSecret)

	// Create webhook handler
	handler := webhook.NewHandler(validator, eventProcessor)

	// Set up HTTP routes
	http.Handle("/webhook", handler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf(":%d", config.Port)
	fmt.Printf("✓ Server listening on %s\n", addr)
	fmt.Printf("📥 Webhook endpoint: http://localhost%s/webhook\n", addr)
	fmt.Printf("🔍 Health check: http://localhost%s/health\n", addr)

	return http.ListenAndServe(addr, nil)
}
