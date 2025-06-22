package review

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
	"github.com/GDSources/claude-code-review-agent/pkg/llm"
)

type mockWorkspaceManager struct {
	shouldFailCreate  bool
	shouldFailCleanup bool
	createError       error
	cleanupError      error
	createdWorkspace  *Workspace
	cleanupCalled     bool
}

func (m *mockWorkspaceManager) CreateWorkspace(ctx context.Context, event *PullRequestEvent) (*Workspace, error) {
	if m.shouldFailCreate {
		return nil, m.createError
	}

	workspace := &Workspace{
		Path:        "/tmp/test-workspace/" + event.Repository.Name,
		Repository:  &event.Repository,
		PullRequest: &event.PullRequest,
	}
	m.createdWorkspace = workspace
	return workspace, nil
}

func (m *mockWorkspaceManager) CleanupWorkspace(workspace *Workspace) error {
	m.cleanupCalled = true
	if m.shouldFailCleanup {
		return m.cleanupError
	}
	return nil
}

func createTestPullRequestEvent() *PullRequestEvent {
	return &PullRequestEvent{
		Action: "opened",
		Number: 42,
		PullRequest: PullRequest{
			ID:     123456,
			Number: 42,
			Title:  "Add amazing feature",
			State:  "open",
			Head: Branch{
				Ref: "feature/amazing",
				SHA: "abc123",
			},
			Base: Branch{
				Ref: "main",
				SHA: "def456",
			},
			User: User{
				ID:    1001,
				Login: "developer",
			},
		},
		Repository: Repository{
			ID:       789,
			Name:     "test-repo",
			FullName: "company/test-repo",
			Private:  false,
			Owner: User{
				ID:    2002,
				Login: "company",
			},
		},
	}
}

func TestDefaultReviewOrchestrator_HandlePullRequest(t *testing.T) {
	tests := []struct {
		name                 string
		workspaceCreateFail  bool
		workspaceCreateErr   error
		workspaceCleanupFail bool
		workspaceCleanupErr  error
		expectError          bool
		expectCleanup        bool
		errorContains        string
	}{
		{
			name:          "successful review flow",
			expectError:   false,
			expectCleanup: true,
		},
		{
			name:                "workspace creation fails",
			workspaceCreateFail: true,
			workspaceCreateErr:  fmt.Errorf("failed to create temp directory"),
			expectError:         true,
			expectCleanup:       false,
			errorContains:       "failed to create workspace for PR #42",
		},
		{
			name:                 "cleanup fails but review succeeds",
			workspaceCleanupFail: true,
			workspaceCleanupErr:  fmt.Errorf("permission denied"),
			expectError:          false,
			expectCleanup:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWM := &mockWorkspaceManager{
				shouldFailCreate:  tt.workspaceCreateFail,
				createError:       tt.workspaceCreateErr,
				shouldFailCleanup: tt.workspaceCleanupFail,
				cleanupError:      tt.workspaceCleanupErr,
			}

			orchestrator := NewDefaultReviewOrchestratorLegacy(mockWM)
			event := createTestPullRequestEvent()

			result, err := orchestrator.HandlePullRequest(event)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check result expectations
			if result == nil {
				t.Error("expected result to be returned")
			} else {
				if !tt.expectError && result.Status != "success" {
					t.Errorf("expected status 'success', got '%s'", result.Status)
				}
				if tt.expectError && result.Status != "failed" {
					t.Errorf("expected status 'failed', got '%s'", result.Status)
				}
			}
			if tt.errorContains != "" && (err == nil || err.Error() == "" || err.Error()[:len(tt.errorContains)] != tt.errorContains[:len(tt.errorContains)]) {
				if err != nil && err.Error() != "" {
					if err.Error()[:min(len(err.Error()), len(tt.errorContains))] == tt.errorContains[:min(len(err.Error()), len(tt.errorContains))] {
						// This is OK, partial match is fine
					} else {
						t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
					}
				} else {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			}

			// Check cleanup expectations
			if tt.expectCleanup && !mockWM.cleanupCalled {
				t.Error("expected cleanup to be called")
			}
			if !tt.expectCleanup && mockWM.cleanupCalled {
				t.Error("expected cleanup not to be called")
			}

			// Verify workspace was created if not expecting creation failure
			if !tt.workspaceCreateFail && mockWM.createdWorkspace == nil {
				t.Error("expected workspace to be created")
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestDefaultReviewOrchestrator_WorkspaceIntegration(t *testing.T) {
	mockWM := &mockWorkspaceManager{}
	orchestrator := NewDefaultReviewOrchestratorLegacy(mockWM)
	event := createTestPullRequestEvent()

	result, err := orchestrator.HandlePullRequest(event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	// Verify workspace was created with correct event data
	if mockWM.createdWorkspace == nil {
		t.Fatal("expected workspace to be created")
	}

	workspace := mockWM.createdWorkspace
	if workspace.Repository.FullName != "company/test-repo" {
		t.Errorf("expected repository full name 'company/test-repo', got '%s'", workspace.Repository.FullName)
	}

	if workspace.PullRequest.Number != 42 {
		t.Errorf("expected PR number 42, got %d", workspace.PullRequest.Number)
	}

	if !mockWM.cleanupCalled {
		t.Error("expected cleanup to be called")
	}
}

// Additional mock objects for testing

type mockDiffFetcher struct {
	diffResult *github.DiffResult
	shouldFail bool
	error      error
}

func (m *mockDiffFetcher) GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error) {
	if m.shouldFail {
		return nil, m.error
	}
	return m.diffResult, nil
}

type mockCodeAnalyzer struct {
	parsedDiff     *analyzer.ParsedDiff
	contextualDiff *analyzer.ContextualDiff
	shouldFail     bool
	error          error
}

func (m *mockCodeAnalyzer) ParseDiff(rawDiff string) (*analyzer.ParsedDiff, error) {
	if m.shouldFail {
		return nil, m.error
	}
	return m.parsedDiff, nil
}

func (m *mockCodeAnalyzer) ExtractContext(parsedDiff *analyzer.ParsedDiff, contextLines int) (*analyzer.ContextualDiff, error) {
	if m.shouldFail {
		return nil, m.error
	}
	return m.contextualDiff, nil
}

// Mock objects for comment posting tests

type mockGitHubCommentClient struct {
	createCommentCalls       []createCommentCall
	shouldFailComment        bool
	commentError             error
	getCommentsCalls         []getCommentsCall
	existingComments         []github.PullRequestComment
	createIssueCommentCalls  []createIssueCommentCall
	updateIssueCommentCalls  []updateIssueCommentCall
	findProgressCommentCalls []findProgressCommentCall
	progressComment          *github.IssueComment
	shouldFailIssueComment   bool
	issueCommentError        error
}

type createIssueCommentCall struct {
	owner       string
	repo        string
	issueNumber int
	body        string
}

type updateIssueCommentCall struct {
	owner     string
	repo      string
	commentID int
	body      string
}

type findProgressCommentCall struct {
	owner       string
	repo        string
	issueNumber int
}

type createCommentCall struct {
	owner    string
	repo     string
	prNumber int
	comment  github.CreatePullRequestCommentRequest
}

type getCommentsCall struct {
	owner    string
	repo     string
	prNumber int
}

func (m *mockGitHubCommentClient) CreatePullRequestComment(ctx context.Context, owner, repo string, prNumber int, comment github.CreatePullRequestCommentRequest) (*github.PullRequestComment, error) {
	m.createCommentCalls = append(m.createCommentCalls, createCommentCall{
		owner:    owner,
		repo:     repo,
		prNumber: prNumber,
		comment:  comment,
	})

	if m.shouldFailComment {
		return nil, m.commentError
	}

	return &github.PullRequestComment{
		ID:   int64(len(m.createCommentCalls)),
		Body: comment.Body,
		Path: comment.Path,
		Line: comment.Line,
	}, nil
}

func (m *mockGitHubCommentClient) CreatePullRequestComments(ctx context.Context, owner, repo string, prNumber int, comments []github.CreatePullRequestCommentRequest) (*github.CommentPostingResult, error) {
	result := &github.CommentPostingResult{
		SuccessfulComments: make([]github.PullRequestComment, 0),
		FailedComments:     make([]github.FailedComment, 0),
	}

	for _, comment := range comments {
		prComment, err := m.CreatePullRequestComment(ctx, owner, repo, prNumber, comment)
		if err != nil {
			result.FailedComments = append(result.FailedComments, github.FailedComment{
				Request: comment,
				Error:   err.Error(),
			})
		} else {
			result.SuccessfulComments = append(result.SuccessfulComments, *prComment)
		}
	}

	return result, nil
}

func (m *mockGitHubCommentClient) GetPullRequestComments(ctx context.Context, owner, repo string, prNumber int) ([]github.PullRequestComment, error) {
	m.getCommentsCalls = append(m.getCommentsCalls, getCommentsCall{
		owner:    owner,
		repo:     repo,
		prNumber: prNumber,
	})

	return m.existingComments, nil
}

func (m *mockGitHubCommentClient) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.IssueComment, error) {
	m.createIssueCommentCalls = append(m.createIssueCommentCalls, createIssueCommentCall{
		owner:       owner,
		repo:        repo,
		issueNumber: issueNumber,
		body:        body,
	})

	if m.shouldFailIssueComment {
		return nil, m.issueCommentError
	}

	comment := &github.IssueComment{
		ID:   int64(len(m.createIssueCommentCalls)),
		Body: body,
		User: github.User{Login: "review-agent"},
	}
	m.progressComment = comment
	return comment, nil
}

func (m *mockGitHubCommentClient) UpdateIssueComment(ctx context.Context, owner, repo string, commentID int, body string) (*github.IssueComment, error) {
	m.updateIssueCommentCalls = append(m.updateIssueCommentCalls, updateIssueCommentCall{
		owner:     owner,
		repo:      repo,
		commentID: commentID,
		body:      body,
	})

	if m.shouldFailIssueComment {
		return nil, m.issueCommentError
	}

	comment := &github.IssueComment{
		ID:   int64(commentID),
		Body: body,
		User: github.User{Login: "review-agent"},
	}
	m.progressComment = comment
	return comment, nil
}

func (m *mockGitHubCommentClient) FindProgressComment(ctx context.Context, owner, repo string, issueNumber int) (*github.IssueComment, error) {
	m.findProgressCommentCalls = append(m.findProgressCommentCalls, findProgressCommentCall{
		owner:       owner,
		repo:        repo,
		issueNumber: issueNumber,
	})

	if m.shouldFailIssueComment {
		return nil, m.issueCommentError
	}

	return m.progressComment, nil
}

type mockLLMClientWithComments struct {
	reviewResponse *llm.ReviewResponse
	shouldFail     bool
	error          error
}

func (m *mockLLMClientWithComments) ReviewCode(ctx context.Context, request *llm.ReviewRequest) (*llm.ReviewResponse, error) {
	if m.shouldFail {
		return nil, m.error
	}
	return m.reviewResponse, nil
}

func (m *mockLLMClientWithComments) ValidateConfiguration() error {
	return nil
}

func (m *mockLLMClientWithComments) GetModelInfo() llm.ModelInfo {
	return llm.ModelInfo{Name: "test-model"}
}

func TestDefaultReviewOrchestrator_PostComments_Success(t *testing.T) {
	// Create mock LLM client with review comments
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{
					Filename:   "main.go",
					LineNumber: 15,
					Comment:    "Consider adding error handling",
					Severity:   llm.SeverityMajor,
					Type:       llm.CommentTypeIssue,
				},
				{
					Filename:   "utils.go",
					LineNumber: 25,
					Comment:    "Good implementation",
					Severity:   llm.SeverityInfo,
					Type:       llm.CommentTypePraise,
				},
			},
			Summary:     "Overall good code quality",
			ModelUsed:   "test-model",
			TokensUsed:  llm.TokenUsage{TotalTokens: 100},
			ReviewID:    "test-review-123",
			GeneratedAt: "2023-01-01T12:00:00Z",
		},
	}

	// Create mock GitHub comment client
	mockGitHub := &mockGitHubCommentClient{}

	// Create mock workspace manager
	mockWM := &mockWorkspaceManager{}

	// Create mock diff fetcher
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{
			RawDiff:    "test diff",
			TotalFiles: 2,
		},
	}

	// Create mock code analyzer
	mockCA := &mockCodeAnalyzer{
		parsedDiff: &analyzer.ParsedDiff{
			TotalFiles: 2,
		},
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 2},
		},
	}

	// Create orchestrator with comment posting enabled
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Create test event
	event := createTestPullRequestEvent()

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
	if result.CommentsPosted != 2 {
		t.Errorf("expected 2 comments posted, got %d", result.CommentsPosted)
	}

	// Verify comments were posted
	if len(mockGitHub.createCommentCalls) != 2 {
		t.Errorf("expected 2 comment creation calls, got %d", len(mockGitHub.createCommentCalls))
	}

	// Verify comment content
	firstCall := mockGitHub.createCommentCalls[0]
	if firstCall.comment.Body != "Consider adding error handling" {
		t.Errorf("expected first comment body 'Consider adding error handling', got '%s'", firstCall.comment.Body)
	}
	if firstCall.comment.Path != "main.go" {
		t.Errorf("expected first comment path 'main.go', got '%s'", firstCall.comment.Path)
	}
	if firstCall.comment.Line != 15 {
		t.Errorf("expected first comment line 15, got %d", firstCall.comment.Line)
	}
}

func TestDefaultReviewOrchestrator_PostComments_Failure(t *testing.T) {
	// Create mock LLM client with review comments
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{
					Filename:   "main.go",
					LineNumber: 15,
					Comment:    "Test comment",
					Severity:   llm.SeverityMajor,
					Type:       llm.CommentTypeIssue,
				},
			},
			Summary: "Test summary",
		},
	}

	// Create mock GitHub comment client that fails
	mockGitHub := &mockGitHubCommentClient{
		shouldFailComment: true,
		commentError:      fmt.Errorf("GitHub API error"),
	}

	// Create other mocks
	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test diff", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// Create orchestrator
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Execute the review
	result, err := orchestrator.HandlePullRequest(createTestPullRequestEvent())

	// Should not fail even if comment posting fails
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

func TestDefaultReviewOrchestrator_WithoutGitHubClient(t *testing.T) {
	// Test that orchestrator works without GitHub client (no comment posting)
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{Filename: "test.go", LineNumber: 1, Comment: "Test"},
			},
		},
	}

	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// Create orchestrator without GitHub client
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     nil, // No GitHub client
	}

	// Should succeed without trying to post comments
	result, err := orchestrator.HandlePullRequest(createTestPullRequestEvent())
	if err != nil {
		t.Errorf("HandlePullRequest should succeed without GitHub client, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}
	if result.CommentsPosted != 0 {
		t.Errorf("expected 0 comments posted without GitHub client, got %d", result.CommentsPosted)
	}
}

// NEW FAILING TESTS FOR PROGRESS COMMENTS (TDD APPROACH)

func TestDefaultReviewOrchestrator_ProgressComments_Success(t *testing.T) {
	// Create mock LLM client with review comments
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{
				{
					Filename:   "main.go",
					LineNumber: 15,
					Comment:    "Consider adding error handling",
					Severity:   llm.SeverityMajor,
					Type:       llm.CommentTypeIssue,
				},
			},
			Summary:     "Overall good code quality",
			ModelUsed:   "test-model",
			TokensUsed:  llm.TokenUsage{TotalTokens: 100},
			ReviewID:    "test-review-123",
			GeneratedAt: "2023-01-01T12:00:00Z",
		},
	}

	// Create mock GitHub comment client
	mockGitHub := &mockGitHubCommentClient{}

	// Create other mocks
	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{
			RawDiff:    "test diff",
			TotalFiles: 1,
		},
	}
	mockCA := &mockCodeAnalyzer{
		parsedDiff: &analyzer.ParsedDiff{
			TotalFiles: 1,
		},
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// Create orchestrator with comment posting enabled
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Create test event
	event := createTestPullRequestEvent()

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

	// Verify progress comment was created initially
	if len(mockGitHub.createIssueCommentCalls) < 1 {
		t.Error("expected at least 1 progress comment to be created")
	}

	// Verify initial progress comment content
	initialCall := mockGitHub.createIssueCommentCalls[0]
	if initialCall.owner != "company" {
		t.Errorf("expected owner 'company', got '%s'", initialCall.owner)
	}
	if initialCall.repo != "test-repo" {
		t.Errorf("expected repo 'test-repo', got '%s'", initialCall.repo)
	}
	if initialCall.issueNumber != 42 {
		t.Errorf("expected issue number 42, got %d", initialCall.issueNumber)
	}
	if !strings.Contains(initialCall.body, "🔍") {
		t.Error("expected initial progress comment to contain search emoji")
	}
	if !strings.Contains(initialCall.body, "review-agent:progress-comment") {
		t.Error("expected initial progress comment to contain progress marker")
	}

	// Verify progress comment was updated during analysis
	if len(mockGitHub.updateIssueCommentCalls) < 2 {
		t.Error("expected at least 2 progress comment updates (analyzing, reviewing)")
	}

	// Verify analyzing stage update
	analyzingUpdate := mockGitHub.updateIssueCommentCalls[0]
	if !strings.Contains(analyzingUpdate.body, "📊") {
		t.Error("expected analyzing update to contain chart emoji")
	}
	if !strings.Contains(analyzingUpdate.body, "analyzing") {
		t.Error("expected analyzing update to contain 'analyzing'")
	}

	// Verify reviewing stage update
	reviewingUpdate := mockGitHub.updateIssueCommentCalls[1]
	if !strings.Contains(reviewingUpdate.body, "💬") {
		t.Error("expected reviewing update to contain speech emoji")
	}
	if !strings.Contains(reviewingUpdate.body, "reviewing") {
		t.Error("expected reviewing update to contain 'reviewing'")
	}

	// Verify final completion update
	if len(mockGitHub.updateIssueCommentCalls) < 3 {
		t.Error("expected final completion update")
	}
	completionUpdate := mockGitHub.updateIssueCommentCalls[2]
	if !strings.Contains(completionUpdate.body, "✅") {
		t.Error("expected completion update to contain check mark emoji")
	}
	if !strings.Contains(completionUpdate.body, "completed") {
		t.Error("expected completion update to contain 'completed'")
	}
	if !strings.Contains(completionUpdate.body, "1 comment") {
		t.Error("expected completion update to contain comment count summary")
	}
}

func TestDefaultReviewOrchestrator_ProgressComments_UpdateExisting(t *testing.T) {
	// Create mock LLM client
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{},
			Summary:  "No issues found",
		},
	}

	// Create mock GitHub client with existing progress comment
	existingComment := &github.IssueComment{
		ID:   999,
		Body: "🔍 **Review Progress**\n\n**Stage:** initializing\n<!-- review-agent:progress-comment -->",
		User: github.User{Login: "review-agent"},
	}
	mockGitHub := &mockGitHubCommentClient{
		progressComment: existingComment,
	}

	// Create other mocks
	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// Create orchestrator
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Execute the review
	result, err := orchestrator.HandlePullRequest(createTestPullRequestEvent())
	if err != nil {
		t.Fatalf("HandlePullRequest failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	// Verify existing progress comment was found
	if len(mockGitHub.findProgressCommentCalls) < 1 {
		t.Error("expected progress comment to be searched for")
	}

	// Should update existing comment instead of creating new one
	if len(mockGitHub.createIssueCommentCalls) > 0 {
		t.Error("expected no new progress comment to be created when existing one found")
	}

	// Verify updates were made to existing comment
	if len(mockGitHub.updateIssueCommentCalls) < 3 {
		t.Error("expected at least 3 updates to existing progress comment")
	}

	// Verify final update shows completion
	finalUpdate := mockGitHub.updateIssueCommentCalls[len(mockGitHub.updateIssueCommentCalls)-1]
	if finalUpdate.commentID != 999 {
		t.Errorf("expected update to comment ID 999, got %d", finalUpdate.commentID)
	}
	if !strings.Contains(finalUpdate.body, "completed") {
		t.Error("expected final update to show completion")
	}
}

func TestDefaultReviewOrchestrator_ProgressComments_Failure(t *testing.T) {
	// Create mock that fails during diff fetching
	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		shouldFail: true,
		error:      fmt.Errorf("API rate limit exceeded"),
	}
	mockCA := &mockCodeAnalyzer{}
	mockLLM := &mockLLMClientWithComments{}
	mockGitHub := &mockGitHubCommentClient{}

	// Create orchestrator
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Execute the review
	result, err := orchestrator.HandlePullRequest(createTestPullRequestEvent())

	// Should not fail completely even if analysis fails
	if err != nil {
		t.Errorf("HandlePullRequest should not fail completely, got: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status 'success' even with analysis failure, got '%s'", result.Status)
	}

	// Verify progress comment was still created
	if len(mockGitHub.createIssueCommentCalls) < 1 {
		t.Error("expected progress comment to be created even when analysis fails")
	}

	// Should have at least one update showing the failure was handled gracefully
	if len(mockGitHub.updateIssueCommentCalls) < 1 {
		t.Error("expected progress comment to be updated even when analysis fails")
	}
}

func TestDefaultReviewOrchestrator_ProgressComments_GitHubFailure(t *testing.T) {
	// Create working mocks
	mockLLM := &mockLLMClientWithComments{
		reviewResponse: &llm.ReviewResponse{
			Comments: []llm.ReviewComment{},
			Summary:  "All good",
		},
	}
	mockWM := &mockWorkspaceManager{}
	mockDF := &mockDiffFetcher{
		diffResult: &github.DiffResult{RawDiff: "test", TotalFiles: 1},
	}
	mockCA := &mockCodeAnalyzer{
		contextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{TotalFiles: 1},
		},
	}

	// Create GitHub client that fails issue comment operations
	mockGitHub := &mockGitHubCommentClient{
		shouldFailIssueComment: true,
		issueCommentError:      fmt.Errorf("GitHub API error"),
	}

	// Create orchestrator
	orchestrator := &DefaultReviewOrchestrator{
		workspaceManager: mockWM,
		diffFetcher:      mockDF,
		codeAnalyzer:     mockCA,
		llmClient:        mockLLM,
		githubClient:     mockGitHub,
	}

	// Execute the review
	result, err := orchestrator.HandlePullRequest(createTestPullRequestEvent())

	// Should not fail even if progress comments fail
	if err != nil {
		t.Errorf("HandlePullRequest should not fail when progress comments fail, got: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status 'success' even with progress comment failure, got '%s'", result.Status)
	}

	// Verify attempts were made to create/update progress comments
	if len(mockGitHub.createIssueCommentCalls) < 1 {
		t.Error("expected attempt to create progress comment")
	}
}
