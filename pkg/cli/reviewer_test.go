package cli

import (
	"strings"
	"testing"
)

func TestNewPRReviewer(t *testing.T) {
	config := &ReviewConfig{
		GitHubToken:  "test-token",
		ClaudeAPIKey: "test-key",
	}

	reviewer := NewPRReviewer(config)

	if reviewer == nil {
		t.Error("expected reviewer to be created")
		return // Exit early to avoid nil pointer dereference
	}

	if reviewer.githubClient == nil {
		t.Error("expected GitHub client to be initialized")
	}

	if reviewer.orchestrator == nil {
		t.Error("expected orchestrator to be initialized")
	}
}

func TestPRReviewer_FetchPREvent(t *testing.T) {
	config := &ReviewConfig{
		GitHubToken:  "test-token",
		ClaudeAPIKey: "test-key",
	}

	reviewer := NewPRReviewer(config)

	owner := "testowner"
	repo := "testrepo"
	prNumber := 123

	event, err := reviewer.fetchPREvent(owner, repo, prNumber)

	// This will fail with test credentials, which is expected
	if err == nil {
		t.Error("expected error with test credentials")
	}

	if event != nil {
		t.Error("expected no event to be created with invalid credentials")
	}

	// Verify it's a GitHub API error
	if !strings.Contains(err.Error(), "GitHub API") {
		t.Errorf("expected GitHub API error, got: %v", err)
	}
}

func TestPRReviewer_ReviewPR_Integration(t *testing.T) {
	// This test verifies the integration between CLI and orchestrator
	// Note: This is a unit test that verifies the integration flow without actual git operations
	config := &ReviewConfig{
		GitHubToken:  "test-token",
		ClaudeAPIKey: "test-key",
	}

	reviewer := NewPRReviewer(config)

	// Test with valid parameters
	owner := "testowner"
	repo := "testrepo"
	prNumber := 456

	// This test will fail because we don't have real git repositories
	// In a real scenario, this would complete the review flow
	result, err := reviewer.ReviewPR(owner, repo, prNumber)

	// We expect this to fail with a git-related error since we're using mock data
	if err == nil {
		t.Error("expected error due to mock repository not existing")
	}

	// Result should be returned even on error for partial results
	if result == nil {
		t.Error("expected result to be returned even on error")
	}

	// Verify the error is related to data fetching (GitHub API) or review execution
	if !strings.Contains(err.Error(), "failed to fetch PR data") && !strings.Contains(err.Error(), "review execution failed") {
		t.Errorf("expected fetch or execution error, got: %v", err)
	}
}

func TestReviewConfig(t *testing.T) {
	config := &ReviewConfig{
		GitHubToken:  "test-github-token",
		ClaudeAPIKey: "test-claude-key",
	}

	if config.GitHubToken != "test-github-token" {
		t.Errorf("expected GitHub token 'test-github-token', got '%s'", config.GitHubToken)
	}

	if config.ClaudeAPIKey != "test-claude-key" {
		t.Errorf("expected Claude key 'test-claude-key', got '%s'", config.ClaudeAPIKey)
	}
}
