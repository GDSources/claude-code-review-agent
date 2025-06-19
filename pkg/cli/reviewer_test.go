package cli

import (
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

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if event == nil {
		t.Fatal("expected PR event to be created")
	}

	// Verify event structure
	if event.Number != prNumber {
		t.Errorf("expected PR number %d, got %d", prNumber, event.Number)
	}

	if event.Repository.Name != repo {
		t.Errorf("expected repo name '%s', got '%s'", repo, event.Repository.Name)
	}

	if event.Repository.Owner.Login != owner {
		t.Errorf("expected owner '%s', got '%s'", owner, event.Repository.Owner.Login)
	}

	expectedFullName := owner + "/" + repo
	if event.Repository.FullName != expectedFullName {
		t.Errorf("expected full name '%s', got '%s'", expectedFullName, event.Repository.FullName)
	}

	if event.Action != "opened" {
		t.Errorf("expected action 'opened', got '%s'", event.Action)
	}

	if event.PullRequest.State != "open" {
		t.Errorf("expected PR state 'open', got '%s'", event.PullRequest.State)
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
	err := reviewer.ReviewPR(owner, repo, prNumber)

	// We expect this to fail with a git-related error since we're using mock data
	if err == nil {
		t.Error("expected error due to mock repository not existing")
	}

	// Verify the error is related to git operations, not configuration
	if err.Error()[:len("review execution failed")] != "review execution failed" {
		t.Errorf("expected git-related error, got: %v", err)
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
