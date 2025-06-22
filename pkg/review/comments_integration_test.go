package review

import (
	"testing"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
	"github.com/GDSources/claude-code-review-agent/pkg/llm"
)

func TestCommentPostingWorkflow_EndToEnd(t *testing.T) {
	// Create full end-to-end test with all components

	// Mock workspace manager
	mockWM := &mockWorkspaceManager{}

	// Mock diff fetcher
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{
			RawDiff:    "diff --git a/main.go b/main.go\n+func test() {}",
			TotalFiles: 1,
		},
	}

	// Mock code analyzer
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
			FilesWithContext: []analyzer.FileWithContext{
				{
					FileDiff: analyzer.FileDiff{
						Filename: "main.go",
						Language: "go",
					},
				},
			},
		},
	}

	// Mock LLM client that generates comments
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{
					Filename:   "main.go",
					LineNumber: 10,
					Comment:    "Consider adding documentation",
					Severity:   llm.SeverityMinor,
					Type:       llm.CommentTypeSuggestion,
				},
				{
					Filename:   "main.go",
					LineNumber: 0, // Should be skipped (general comment)
					Comment:    "Overall file looks good",
					Severity:   llm.SeverityInfo,
					Type:       llm.CommentTypePraise,
				},
				{
					Filename:   "main.go",
					LineNumber: 20,
					Comment:    "Potential memory leak here",
					Severity:   llm.SeverityMajor,
					Type:       llm.CommentTypeIssue,
				},
			},
			Summary:   "Code review completed",
			ModelUsed: "test-model",
			TokensUsed: llm.TokenUsage{
				TotalTokens: 150,
			},
		},
	}

	// Mock GitHub client that tracks comment creation
	mockGitHub := &mockGitHubCommentClient{}

	// Create orchestrator with comment posting
	orchestrator := NewReviewOrchestratorWithComments(
		mockWM, mockDF, mockCA, mockLLM, mockGitHub)

	// Create test event
	event := &PullRequestEvent{
		Action: "opened",
		Number: 123,
		PullRequest: PullRequest{
			ID:     456789,
			Number: 123,
			Title:  "Test PR with comments",
			Head: Branch{
				SHA: "abc123def456",
				Ref: "feature/test",
			},
			Base: Branch{
				Ref: "main",
			},
			User: User{
				Login: "testuser",
			},
		},
		Repository: Repository{
			Name:     "test-repo",
			FullName: "testorg/test-repo",
			Owner: User{
				Login: "testorg",
			},
		},
	}

	// Execute the review
	result, err := orchestrator.HandlePullRequest(event)
	if err != nil {
		t.Fatalf("HandlePullRequest failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	// Verify that only valid line comments were posted (skipping line 0)
	if len(mockGitHub.createCommentCalls) != 2 {
		t.Errorf("expected 2 comment creation calls, got %d", len(mockGitHub.createCommentCalls))
	}

	// Verify first comment
	firstCall := mockGitHub.createCommentCalls[0]
	if firstCall.owner != "testorg" {
		t.Errorf("expected owner 'testorg', got '%s'", firstCall.owner)
	}
	if firstCall.repo != "test-repo" {
		t.Errorf("expected repo 'test-repo', got '%s'", firstCall.repo)
	}
	if firstCall.prNumber != 123 {
		t.Errorf("expected PR number 123, got %d", firstCall.prNumber)
	}
	if firstCall.comment.Body != "Consider adding documentation" {
		t.Errorf("expected first comment body 'Consider adding documentation', got '%s'", firstCall.comment.Body)
	}
	if firstCall.comment.Path != "main.go" {
		t.Errorf("expected first comment path 'main.go', got '%s'", firstCall.comment.Path)
	}
	if firstCall.comment.Line != 10 {
		t.Errorf("expected first comment line 10, got %d", firstCall.comment.Line)
	}
	if firstCall.comment.CommitID != "abc123def456" {
		t.Errorf("expected commit ID 'abc123def456', got '%s'", firstCall.comment.CommitID)
	}

	// Verify second comment
	secondCall := mockGitHub.createCommentCalls[1]
	if secondCall.comment.Body != "Potential memory leak here" {
		t.Errorf("expected second comment body 'Potential memory leak here', got '%s'", secondCall.comment.Body)
	}
	if secondCall.comment.Line != 20 {
		t.Errorf("expected second comment line 20, got %d", secondCall.comment.Line)
	}
}

func TestCommentPostingWorkflow_PartialFailure(t *testing.T) {
	// Test that workflow continues even if some comments fail to post

	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// LLM generates one comment
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{
					Filename:   "test.go",
					LineNumber: 15,
					Comment:    "Test comment",
					Severity:   llm.SeverityMinor,
					Type:       llm.CommentTypeSuggestion,
				},
			},
		},
	}

	// GitHub client that fails to post comments
	mockGitHub := &mockGitHubCommentClient{
		shouldFailComment: true,
		commentError:      newError("GitHub API rate limit exceeded"),
	}

	orchestrator := NewReviewOrchestratorWithComments(
		mockWM, mockDF, mockCA, mockLLM, mockGitHub)

	event := createTestPullRequestEvent()

	// Should not fail even if comment posting fails
	result, err := orchestrator.HandlePullRequest(event)
	if err != nil {
		t.Errorf("HandlePullRequest should not fail when comment posting fails, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	// Verify comment posting was attempted
	if len(mockGitHub.createCommentCalls) != 1 {
		t.Errorf("expected 1 comment creation attempt, got %d", len(mockGitHub.createCommentCalls))
	}
}

func TestCommentPostingWorkflow_NoLLMComments(t *testing.T) {
	// Test workflow when LLM doesn't generate any comments

	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// LLM generates no comments
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{}, // Empty
			Summary:  "No issues found",
		},
	}

	mockGitHub := &mockGitHubCommentClient{}

	orchestrator := NewReviewOrchestratorWithComments(
		mockWM, mockDF, mockCA, mockLLM, mockGitHub)

	event := createTestPullRequestEvent()

	// Should succeed without posting any comments
	result, err := orchestrator.HandlePullRequest(event)
	if err != nil {
		t.Errorf("HandlePullRequest should succeed with no comments, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}
	if result.CommentsPosted != 0 {
		t.Errorf("expected 0 comments posted, got %d", result.CommentsPosted)
	}

	// Verify no comment posting was attempted
	if len(mockGitHub.createCommentCalls) != 0 {
		t.Errorf("expected 0 comment creation attempts, got %d", len(mockGitHub.createCommentCalls))
	}
}

// Helper function to create errors for tests
func newError(message string) error {
	return &customError{message: message}
}

type customError struct {
	message string
}

func (e *customError) Error() string {
	return e.message
}
