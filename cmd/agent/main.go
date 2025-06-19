package main

import (
	"flag"
	"fmt"
	"os"

	. "github.com/your-org/review-agent/pkg/cli"
)

const (
	Version = "1.0.0"
)

type Config struct {
	GitHubToken  string
	ClaudeAPIKey string
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
  --owner           Repository owner/organization (required)
  --repo            Repository name (required)
  --pr              Pull request number (required)

Examples:
  review-agent review --owner myorg --repo myrepo --pr 123
  GITHUB_TOKEN=xxx CLAUDE_API_KEY=yyy review-agent review --owner myorg --repo myrepo --pr 123
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
	if config.GitHubToken == "" {
		config.GitHubToken = os.Getenv("GITHUB_TOKEN")
	}
	if config.ClaudeAPIKey == "" {
		config.ClaudeAPIKey = os.Getenv("CLAUDE_API_KEY")
	}
	return nil
}

func validateReviewConfig(config *Config, owner, repo string, prNumber int) error {
	if config.GitHubToken == "" {
		return fmt.Errorf("GitHub token is required (set --github-token flag or GITHUB_TOKEN env var)")
	}
	if config.ClaudeAPIKey == "" {
		return fmt.Errorf("Claude API key is required (set --claude-key flag or CLAUDE_API_KEY env var)")
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
	reviewConfig := &ReviewConfig{
		GitHubToken:  config.GitHubToken,
		ClaudeAPIKey: config.ClaudeAPIKey,
	}

	reviewer := NewPRReviewer(reviewConfig)

	// Execute the review
	if err := reviewer.ReviewPR(owner, repo, prNumber); err != nil {
		return err
	}

	return nil
}
