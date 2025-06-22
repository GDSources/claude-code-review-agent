package review

import (
	"context"
	"fmt"
	"testing"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
	"github.com/GDSources/claude-code-review-agent/pkg/llm"
)

// Mock implementations for deletion analysis testing

type mockCodebaseFlattener struct{}

func (m *mockCodebaseFlattener) FlattenWorkspace(workspacePath string) (*analyzer.FlattenedCodebase, error) {
	return &analyzer.FlattenedCodebase{
		Files: []analyzer.FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import "fmt"

func main() {
	result := CalculateSum(5, 10) // This will be orphaned
	fmt.Printf("Result: %d\n", result)
	
	SafeFunction() // This is safe
}

func SafeFunction() {
	fmt.Println("Safe function")
}`,
				LineCount: 12,
			},
			{
				RelativePath: "service.go",
				Language:     "go",
				Content: `package main

func ProcessData() {
	// This function also calls deleted code
	total := CalculateSum(1, 2) // Orphaned reference
	fmt.Printf("Processing: %d\n", total)
}`,
				LineCount: 6,
			},
		},
		TotalFiles: 2,
		TotalLines: 18,
		Languages:  []string{"go"},
		ProjectInfo: analyzer.ProjectInfo{
			Type: "go",
			Name: "test-deletion-analysis",
		},
		Summary: "Test codebase for deletion analysis",
	}, nil
}

func (m *mockCodebaseFlattener) FlattenDiff(workspacePath string, diff *analyzer.ParsedDiff) (*analyzer.FlattenedCodebase, error) {
	return m.FlattenWorkspace(workspacePath)
}

type mockDeletionAnalyzer struct{}

func (m *mockDeletionAnalyzer) AnalyzeDeletions(request *analyzer.DeletionAnalysisRequest) (*analyzer.DeletionAnalysisResult, error) {
	// Simulate finding orphaned references
	return &analyzer.DeletionAnalysisResult{
		OrphanedReferences: []analyzer.OrphanedReference{
			{
				DeletedEntity:    "CalculateSum",
				ReferencingFile:  "main.go",
				ReferencingLines: []int{6},
				ReferenceType:    "function_call",
				Context:          "result := CalculateSum(5, 10)",
				Severity:         "error",
				Suggestion:       "Remove the call to CalculateSum or provide an alternative implementation",
			},
			{
				DeletedEntity:    "CalculateSum",
				ReferencingFile:  "service.go",
				ReferencingLines: []int{5},
				ReferenceType:    "function_call",
				Context:          "total := CalculateSum(1, 2)",
				Severity:         "error",
				Suggestion:       "Remove the call to CalculateSum or provide an alternative implementation",
			},
		},
		SafeDeletions: []string{"UnusedFunction"},
		Warnings: []analyzer.Warning{
			{
				Type:       "orphaned_references",
				Message:    "Found 2 potential orphaned references",
				Severity:   "warning",
				Suggestion: "Review the identified references and either remove them or provide alternative implementations",
			},
		},
		Summary:    "Deletion analysis found 2 orphaned references that need attention",
		Confidence: 0.85,
	}, nil
}

func TestReviewOrchestrator_DeletionAnalysisIntegration(t *testing.T) {
	// Create mock components
	workspaceManager := &integrationMockWorkspaceManager{}
	diffFetcher := &integrationMockDiffFetcher{}
	codeAnalyzer := &integrationMockCodeAnalyzer{}
	codebaseFlattener := &mockCodebaseFlattener{}
	deletionAnalyzer := &mockDeletionAnalyzer{}
	llmClient := &integrationMockLLMClient{}
	githubClient := &integrationMockGitHubCommentClient{}

	// Create orchestrator with deletion analysis capabilities
	orchestrator := NewReviewOrchestratorWithDeletionAnalysis(
		workspaceManager,
		diffFetcher,
		codeAnalyzer,
		codebaseFlattener,
		deletionAnalyzer,
		llmClient,
		githubClient,
	)

	// Create a PR event with deletions
	event := &PullRequestEvent{
		Number: 123,
		PullRequest: PullRequest{
			ID:    456789,
			Title: "Remove deprecated utility functions",
			User: User{
				Login: "developer",
			},
			Head: Branch{
				Ref: "feature/cleanup-utils",
				SHA: "abc123def456",
			},
			Base: Branch{
				Ref: "main",
			},
		},
		Repository: Repository{
			FullName: "company/test-repo",
			Owner: User{
				Login: "company",
			},
			Name: "test-repo",
		},
	}

	// Handle the PR (this should trigger deletion analysis)
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

	// Verify that deletion analysis was performed
	if workspaceManager.lastWorkspace == nil {
		t.Fatal("Expected workspace to be created")
	}

	if diffFetcher.lastOwner != "company" || diffFetcher.lastRepo != "test-repo" || diffFetcher.lastPRNumber != 123 {
		t.Error("Expected diff fetcher to be called with correct parameters")
	}

	if codeAnalyzer.parsedDiff == nil {
		t.Error("Expected code analyzer to parse diff")
	}

	// The orchestrator should have called the LLM with deletion analysis data
	if llmClient.lastRequest == nil {
		t.Error("Expected LLM client to be called")
	}

	// The orchestrator should have posted comments
	if len(githubClient.postedComments) == 0 {
		t.Error("Expected comments to be posted")
	}

	t.Logf("Deletion analysis integration test completed successfully")
	t.Logf("- Workspace created: %s", workspaceManager.lastWorkspace.Path)
	t.Logf("- Diff fetched for PR #%d", event.Number)
	t.Logf("- LLM analysis performed")
	t.Logf("- Comments posted: %d", len(githubClient.postedComments))
}

func TestReviewOrchestrator_DeletionAnalysisWithActualComponents(t *testing.T) {
	// This test demonstrates integration with actual deletion analysis components
	workspaceManager := &integrationMockWorkspaceManager{}
	diffFetcher := &integrationMockDiffFetcher{}
	codeAnalyzer := &integrationMockCodeAnalyzer{}

	// Use real deletion analysis components
	codebaseFlattener := analyzer.NewDefaultCodebaseFlattener()
	deletionAnalyzer := analyzer.NewDefaultDeletionAnalyzer() // Heuristic mode

	llmClient := &integrationMockLLMClient{}
	githubClient := &integrationMockGitHubCommentClient{}

	// Create orchestrator with real deletion analysis
	orchestrator := NewReviewOrchestratorWithDeletionAnalysis(
		workspaceManager,
		diffFetcher,
		codeAnalyzer,
		codebaseFlattener,
		deletionAnalyzer,
		llmClient,
		githubClient,
	)

	// Create a PR event with deletions
	event := &PullRequestEvent{
		Number: 456,
		PullRequest: PullRequest{
			Title: "Refactor authentication system",
			User: User{
				Login: "developer",
			},
			Head: Branch{
				Ref: "feature/auth-refactor",
				SHA: "def456ghi789",
			},
			Base: Branch{
				Ref: "main",
			},
		},
		Repository: Repository{
			FullName: "company/auth-service",
			Owner: User{
				Login: "company",
			},
			Name: "auth-service",
		},
	}

	// Handle the PR
	result, err := orchestrator.HandlePullRequest(event)
	if err != nil {
		t.Fatalf("HandlePullRequest with real components failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	t.Logf("Real deletion analysis integration test completed")
}

func TestReviewOrchestrator_DeletionAnalysisDisabled(t *testing.T) {
	// Test that orchestrator works without deletion analysis components
	workspaceManager := &integrationMockWorkspaceManager{}
	diffFetcher := &integrationMockDiffFetcher{}
	codeAnalyzer := &integrationMockCodeAnalyzer{}
	llmClient := &integrationMockLLMClient{}
	githubClient := &integrationMockGitHubCommentClient{}

	// Create orchestrator WITHOUT deletion analysis components
	orchestrator := NewReviewOrchestratorWithComments(
		workspaceManager,
		diffFetcher,
		codeAnalyzer,
		llmClient,
		githubClient,
	)

	event := &PullRequestEvent{
		Number: 789,
		PullRequest: PullRequest{
			Title: "Regular PR without deletion analysis",
			User: User{
				Login: "developer",
			},
			Head: Branch{
				Ref: "feature/regular-changes",
				SHA: "ghi789jkl012",
			},
			Base: Branch{
				Ref: "main",
			},
		},
		Repository: Repository{
			FullName: "company/regular-repo",
			Owner: User{
				Login: "company",
			},
			Name: "regular-repo",
		},
	}

	// This should work fine without deletion analysis
	result, err := orchestrator.HandlePullRequest(event)
	if err != nil {
		t.Fatalf("HandlePullRequest without deletion analysis failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}
	if result.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Status)
	}

	t.Logf("PR review without deletion analysis completed successfully")
}

// Additional mock implementations for comprehensive testing

type integrationMockWorkspaceManager struct {
	lastWorkspace *Workspace
}

func (m *integrationMockWorkspaceManager) CreateWorkspace(ctx context.Context, event *PullRequestEvent) (*Workspace, error) {
	workspace := &Workspace{
		Path:        fmt.Sprintf("/tmp/test-workspace/%s", event.Repository.Name),
		Repository:  &event.Repository,
		PullRequest: &event.PullRequest,
	}
	m.lastWorkspace = workspace
	return workspace, nil
}

func (m *integrationMockWorkspaceManager) CleanupWorkspace(workspace *Workspace) error {
	return nil
}

type integrationMockDiffFetcher struct {
	lastOwner    string
	lastRepo     string
	lastPRNumber int
}

func (m *integrationMockDiffFetcher) GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error) {
	m.lastOwner = owner
	m.lastRepo = repo
	m.lastPRNumber = prNumber

	// Return a diff with deletions
	return &github.DiffResult{
		RawDiff: `diff --git a/utils.go b/utils.go
deleted file mode 100644
index 1234567..0000000
--- a/utils.go
+++ /dev/null
@@ -1,5 +0,0 @@
-func CalculateSum(a, b int) int {
-    return a + b
-}
-
-func UnusedFunction() {}

diff --git a/main.go b/main.go
index 2345678..3456789 100644
--- a/main.go
+++ b/main.go
@@ -3,7 +3,6 @@ import "fmt"
 
 func main() {
 	result := CalculateSum(5, 10)
-	processOldData() // Removed this line
 	fmt.Printf("Result: %d\n", result)
 }`,
		TotalFiles: 2,
	}, nil
}

type integrationMockCodeAnalyzer struct {
	parsedDiff *analyzer.ParsedDiff
}

func (m *integrationMockCodeAnalyzer) ParseDiff(rawDiff string) (*analyzer.ParsedDiff, error) {
	// Create a mock parsed diff with deletions
	parsedDiff := &analyzer.ParsedDiff{
		Files: []analyzer.FileDiff{
			{
				Filename: "utils.go",
				Status:   "deleted",
				Language: "go",
				Hunks: []analyzer.DiffHunk{
					{
						OldStart: 1,
						OldCount: 5,
						NewStart: 0,
						NewCount: 0,
						Lines: []analyzer.DiffLine{
							{Type: "removed", Content: "func CalculateSum(a, b int) int {", OldLineNo: 1},
							{Type: "removed", Content: "    return a + b", OldLineNo: 2},
							{Type: "removed", Content: "}", OldLineNo: 3},
							{Type: "removed", Content: "", OldLineNo: 4},
							{Type: "removed", Content: "func UnusedFunction() {}", OldLineNo: 5},
						},
					},
				},
			},
		},
	}
	m.parsedDiff = parsedDiff
	return parsedDiff, nil
}

func (m *integrationMockCodeAnalyzer) ExtractContext(parsedDiff *analyzer.ParsedDiff, contextLines int) (*analyzer.ContextualDiff, error) {
	return &analyzer.ContextualDiff{
		ParsedDiff: &analyzer.ParsedDiff{
			Files:        parsedDiff.Files,
			TotalFiles:   parsedDiff.TotalFiles,
			TotalAdded:   0,
			TotalRemoved: 6,
		},
		FilesWithContext: []analyzer.FileWithContext{
			{
				FileDiff: analyzer.FileDiff{
					Filename: "utils.go",
					Status:   "deleted",
					Language: "go",
				},
			},
		},
	}, nil
}

type integrationMockLLMClient struct {
	lastRequest *llm.ReviewRequest
}

func (m *integrationMockLLMClient) ReviewCode(ctx context.Context, request *llm.ReviewRequest) (*llm.ReviewResponse, error) {
	m.lastRequest = request
	return &llm.ReviewResponse{
		Comments: []llm.ReviewComment{
			{
				Filename:   "main.go",
				LineNumber: 6,
				Comment:    "Orphaned reference to CalculateSum function that was deleted",
				Severity:   llm.SeverityMajor,
				Type:       llm.CommentTypeIssue,
				Suggestion: "Remove this call or provide an alternative implementation",
			},
		},
		Summary:     "Found deletion safety issues",
		ModelUsed:   "test-model",
		TokensUsed:  llm.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		ReviewID:    "test-review-123",
		GeneratedAt: "2023-01-01T00:00:00Z",
	}, nil
}

func (m *integrationMockLLMClient) ValidateConfiguration() error {
	return nil
}

func (m *integrationMockLLMClient) GetModelInfo() llm.ModelInfo {
	return llm.ModelInfo{
		Name:        "test-model",
		Version:     "1.0",
		MaxTokens:   4000,
		Provider:    "test",
		Description: "Test model for deletion analysis",
	}
}

type integrationMockGitHubCommentClient struct {
	postedComments []github.CreatePullRequestCommentRequest
}

func (m *integrationMockGitHubCommentClient) CreatePullRequestComment(ctx context.Context, owner, repo string, prNumber int, comment github.CreatePullRequestCommentRequest) (*github.PullRequestComment, error) {
	m.postedComments = append(m.postedComments, comment)
	return &github.PullRequestComment{
		ID:   123,
		Body: comment.Body,
		Path: comment.Path,
		Line: comment.Line,
	}, nil
}

func (m *integrationMockGitHubCommentClient) CreatePullRequestComments(ctx context.Context, owner, repo string, prNumber int, comments []github.CreatePullRequestCommentRequest) (*github.CommentPostingResult, error) {
	for _, comment := range comments {
		m.postedComments = append(m.postedComments, comment)
	}

	return &github.CommentPostingResult{
		SuccessfulComments: make([]github.PullRequestComment, len(comments)),
		FailedComments:     []github.FailedComment{},
	}, nil
}

func (m *integrationMockGitHubCommentClient) GetPullRequestComments(ctx context.Context, owner, repo string, prNumber int) ([]github.PullRequestComment, error) {
	return []github.PullRequestComment{}, nil
}
