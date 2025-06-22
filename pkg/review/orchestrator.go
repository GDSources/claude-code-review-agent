package review

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
	"github.com/GDSources/claude-code-review-agent/pkg/llm"
)

// GitHubCommentClient interface for posting comments (to avoid circular imports)
type GitHubCommentClient interface {
	CreatePullRequestComment(ctx context.Context, owner, repo string, prNumber int, comment github.CreatePullRequestCommentRequest) (*github.PullRequestComment, error)
	CreatePullRequestComments(ctx context.Context, owner, repo string, prNumber int, comments []github.CreatePullRequestCommentRequest) (*github.CommentPostingResult, error)
	GetPullRequestComments(ctx context.Context, owner, repo string, prNumber int) ([]github.PullRequestComment, error)
	CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.IssueComment, error)
	UpdateIssueComment(ctx context.Context, owner, repo string, commentID int, body string) (*github.IssueComment, error)
	FindProgressComment(ctx context.Context, owner, repo string, issueNumber int) (*github.IssueComment, error)
}

type DefaultReviewOrchestrator struct {
	workspaceManager  WorkspaceManager
	diffFetcher       DiffFetcher
	codeAnalyzer      CodeAnalyzer
	codebaseFlattener CodebaseFlattener
	deletionAnalyzer  DeletionAnalyzer
	llmClient         llm.CodeReviewer
	githubClient      GitHubCommentClient
}

func NewDefaultReviewOrchestrator(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager:  workspaceManager,
		diffFetcher:       diffFetcher,
		codeAnalyzer:      codeAnalyzer,
		codebaseFlattener: nil,
		deletionAnalyzer:  nil,
		llmClient:         nil,
		githubClient:      nil,
	}
}

// NewReviewOrchestratorWithLLM creates orchestrator with LLM integration
func NewReviewOrchestratorWithLLM(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer, llmClient llm.CodeReviewer) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager:  workspaceManager,
		diffFetcher:       diffFetcher,
		codeAnalyzer:      codeAnalyzer,
		codebaseFlattener: nil,
		deletionAnalyzer:  nil,
		llmClient:         llmClient,
		githubClient:      nil,
	}
}

// NewReviewOrchestratorWithComments creates orchestrator with LLM and comment posting
func NewReviewOrchestratorWithComments(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer, llmClient llm.CodeReviewer, githubClient GitHubCommentClient) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager:  workspaceManager,
		diffFetcher:       diffFetcher,
		codeAnalyzer:      codeAnalyzer,
		codebaseFlattener: nil,
		deletionAnalyzer:  nil,
		llmClient:         llmClient,
		githubClient:      githubClient,
	}
}

// NewDefaultReviewOrchestratorLegacy creates orchestrator without diff analysis (for backward compatibility)
func NewDefaultReviewOrchestratorLegacy(workspaceManager WorkspaceManager) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager:  workspaceManager,
		diffFetcher:       nil,
		codeAnalyzer:      nil,
		codebaseFlattener: nil,
		deletionAnalyzer:  nil,
		llmClient:         nil,
		githubClient:      nil,
	}
}

// NewReviewOrchestratorWithDeletionAnalysis creates orchestrator with deletion analysis capabilities
func NewReviewOrchestratorWithDeletionAnalysis(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer, codebaseFlattener CodebaseFlattener, deletionAnalyzer DeletionAnalyzer, llmClient llm.CodeReviewer, githubClient GitHubCommentClient) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager:  workspaceManager,
		diffFetcher:       diffFetcher,
		codeAnalyzer:      codeAnalyzer,
		codebaseFlattener: codebaseFlattener,
		deletionAnalyzer:  deletionAnalyzer,
		llmClient:         llmClient,
		githubClient:      githubClient,
	}
}

func (r *DefaultReviewOrchestrator) HandlePullRequest(event *PullRequestEvent) (*ReviewResult, error) {
	ctx := context.Background()

	log.Printf("Starting review for PR #%d in %s", event.Number, event.Repository.FullName)

	result := &ReviewResult{
		CommentsPosted: 0,
		Status:         "success",
		Summary:        "",
	}

	// Initialize progress tracking if GitHub client is available
	var progressComment *github.IssueComment
	var reviewProgress *ReviewProgress
	if r.githubClient != nil {
		reviewData := &ReviewData{Event: event}
		reviewProgress = CreateInitialProgress(reviewData)

		// Check for existing progress comment first
		existingComment, err := r.githubClient.FindProgressComment(ctx,
			event.Repository.Owner.Login,
			event.Repository.Name,
			event.Number)
		if err != nil {
			log.Printf("Warning: failed to check for existing progress comment: %v", err)
		}

		if existingComment != nil {
			// Update existing progress comment
			progressComment = existingComment
			commentBody := GenerateProgressComment(reviewProgress)
			_, err = r.githubClient.UpdateIssueComment(ctx,
				event.Repository.Owner.Login,
				event.Repository.Name,
				int(existingComment.ID),
				commentBody)
			if err != nil {
				log.Printf("Warning: failed to update existing progress comment: %v", err)
			}
		} else {
			// Create new progress comment
			commentBody := GenerateProgressComment(reviewProgress)
			createdComment, err := r.githubClient.CreateIssueComment(ctx,
				event.Repository.Owner.Login,
				event.Repository.Name,
				event.Number,
				commentBody)
			if err != nil {
				log.Printf("Warning: failed to create progress comment: %v", err)
			} else {
				progressComment = createdComment
			}
		}
	}

	workspace, err := r.workspaceManager.CreateWorkspace(ctx, event)
	if err != nil {
		// Update progress comment with failure if available
		if r.githubClient != nil && progressComment != nil && reviewProgress != nil {
			UpdateProgressStage(reviewProgress, "failed", fmt.Sprintf("Failed to create workspace: %v", err))
			reviewProgress.Summary = "Review failed during workspace setup"
			commentBody := GenerateProgressComment(reviewProgress)
			_, updateErr := r.githubClient.UpdateIssueComment(ctx,
				event.Repository.Owner.Login,
				event.Repository.Name,
				int(progressComment.ID),
				commentBody)
			if updateErr != nil {
				log.Printf("Warning: failed to update progress comment with failure: %v", updateErr)
			}
		}
		result.Status = "failed"
		return result, fmt.Errorf("failed to create workspace for PR #%d: %w", event.Number, err)
	}

	defer func() {
		if cleanupErr := r.workspaceManager.CleanupWorkspace(workspace); cleanupErr != nil {
			log.Printf("Warning: failed to cleanup workspace: %v", cleanupErr)
		}
	}()

	log.Printf("Successfully cloned repository %s to %s", event.Repository.FullName, workspace.Path)
	log.Printf("Checked out branch %s for PR #%d", event.PullRequest.Head.Ref, event.Number)

	// Fetch and analyze PR diff if analyzers are available
	var reviewData *ReviewData
	if r.diffFetcher != nil && r.codeAnalyzer != nil {
		// Update progress to analyzing stage
		if r.githubClient != nil && progressComment != nil && reviewProgress != nil {
			UpdateProgressStage(reviewProgress, "analyzing", "Analyzing code changes...")
			commentBody := GenerateProgressComment(reviewProgress)
			_, updateErr := r.githubClient.UpdateIssueComment(ctx,
				event.Repository.Owner.Login,
				event.Repository.Name,
				int(progressComment.ID),
				commentBody)
			if updateErr != nil {
				log.Printf("Warning: failed to update progress comment to analyzing stage: %v", updateErr)
			}
		}

		diffResult, err := r.fetchPRDiff(ctx, event)
		if err != nil {
			log.Printf("Warning: failed to fetch PR diff: %v", err)
		} else {
			log.Printf("Fetched diff for PR #%d: %d files changed", event.Number, diffResult.TotalFiles)

			contextualDiff, err := r.analyzeDiff(diffResult)
			if err != nil {
				log.Printf("Warning: failed to analyze diff: %v", err)
			} else {
				log.Printf("Analyzed diff for PR #%d: %d added, %d removed lines",
					event.Number, contextualDiff.TotalAdded, contextualDiff.TotalRemoved)

				reviewData = &ReviewData{
					Event:          event,
					Workspace:      workspace,
					DiffResult:     diffResult,
					ContextualDiff: contextualDiff,
				}

				// Perform deletion analysis if available
				if r.codebaseFlattener != nil && r.deletionAnalyzer != nil {
					err := r.performDeletionAnalysis(ctx, reviewData)
					if err != nil {
						log.Printf("Warning: deletion analysis failed for PR #%d: %v", event.Number, err)
					}
				}
			}
		}
	} else {
		log.Printf("Diff analysis skipped (analyzers not configured)")
	}

	// Send reviewData to LLM for analysis if available
	if reviewData != nil && r.llmClient != nil {
		// Update progress to reviewing stage
		if r.githubClient != nil && progressComment != nil && reviewProgress != nil {
			UpdateProgressStage(reviewProgress, "reviewing", "Generating review comments...")
			commentBody := GenerateProgressComment(reviewProgress)
			_, updateErr := r.githubClient.UpdateIssueComment(ctx,
				event.Repository.Owner.Login,
				event.Repository.Name,
				int(progressComment.ID),
				commentBody)
			if updateErr != nil {
				log.Printf("Warning: failed to update progress comment to reviewing stage: %v", updateErr)
			}
		}

		log.Printf("Sending PR #%d to LLM for analysis", event.Number)

		reviewResponse, err := r.performLLMReview(ctx, reviewData)
		if err != nil {
			log.Printf("Warning: LLM review failed for PR #%d: %v", event.Number, err)
		} else {
			log.Printf("LLM review completed for PR #%d: %d comments generated",
				event.Number, len(reviewResponse.Comments))

			// Post generated comments back to GitHub PR
			if r.githubClient != nil {
				commentsPosted, err := r.postReviewComments(ctx, reviewData, reviewResponse)
				if err != nil {
					log.Printf("Warning: Failed to post comments to PR #%d: %v", event.Number, err)
				} else {
					result.CommentsPosted = commentsPosted
				}
			} else {
				log.Printf("GitHub client not configured, skipping comment posting for PR #%d", event.Number)
			}

			r.logReviewResults(reviewResponse)
		}
	} else if reviewData != nil {
		log.Printf("Review data prepared for PR #%d (LLM not configured)", event.Number)
	}

	// Update progress comment with completion status
	if r.githubClient != nil && progressComment != nil && reviewProgress != nil {
		UpdateProgressStage(reviewProgress, "completed", "Review completed successfully")

		// Generate summary based on results
		var summary string
		if result.CommentsPosted > 0 {
			if result.CommentsPosted == 1 {
				summary = "Posted 1 comment"
			} else {
				summary = fmt.Sprintf("Posted %d comments", result.CommentsPosted)
			}
		} else {
			summary = "No issues found"
		}
		reviewProgress.Summary = summary

		commentBody := GenerateProgressComment(reviewProgress)
		_, err := r.githubClient.UpdateIssueComment(ctx,
			event.Repository.Owner.Login,
			event.Repository.Name,
			int(progressComment.ID),
			commentBody)
		if err != nil {
			log.Printf("Warning: failed to update final progress comment: %v", err)
		}
	}

	log.Printf("Review completed for PR #%d", event.Number)
	return result, nil
}

// fetchPRDiff fetches the diff for a pull request
func (r *DefaultReviewOrchestrator) fetchPRDiff(ctx context.Context, event *PullRequestEvent) (*github.DiffResult, error) {
	if r.diffFetcher == nil {
		return nil, fmt.Errorf("diff fetcher not configured")
	}

	return r.diffFetcher.GetPullRequestDiffWithFiles(ctx,
		event.Repository.Owner.Login,
		event.Repository.Name,
		event.Number)
}

// analyzeDiff analyzes the fetched diff and extracts context
func (r *DefaultReviewOrchestrator) analyzeDiff(diffResult *github.DiffResult) (*analyzer.ContextualDiff, error) {
	if r.codeAnalyzer == nil {
		return nil, fmt.Errorf("code analyzer not configured")
	}

	// Validate input parameters
	if diffResult == nil {
		return nil, fmt.Errorf("diff result cannot be nil")
	}
	if diffResult.RawDiff == "" {
		return nil, fmt.Errorf("raw diff cannot be empty")
	}

	// Parse the raw diff
	parsedDiff, err := r.codeAnalyzer.ParseDiff(diffResult.RawDiff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}

	// Extract context (5 lines as mentioned in CLAUDE.md)
	contextualDiff, err := r.codeAnalyzer.ExtractContext(parsedDiff, 5)
	if err != nil {
		return nil, fmt.Errorf("failed to extract context: %w", err)
	}

	return contextualDiff, nil
}

// performDeletionAnalysis analyzes code deletions for orphaned references
func (r *DefaultReviewOrchestrator) performDeletionAnalysis(ctx context.Context, reviewData *ReviewData) error {
	if r.codebaseFlattener == nil || r.deletionAnalyzer == nil {
		return fmt.Errorf("deletion analysis components not configured")
	}

	// Validate input parameters
	if reviewData == nil {
		return fmt.Errorf("review data cannot be nil")
	}
	if reviewData.DiffResult == nil {
		return fmt.Errorf("diff result cannot be nil")
	}
	if reviewData.Workspace == nil {
		return fmt.Errorf("workspace cannot be nil")
	}
	if reviewData.Workspace.Path == "" {
		return fmt.Errorf("workspace path cannot be empty")
	}

	// Parse the diff to extract deleted content
	parsedDiff, err := r.codeAnalyzer.ParseDiff(reviewData.DiffResult.RawDiff)
	if err != nil {
		return fmt.Errorf("failed to parse diff for deletion analysis: %w", err)
	}

	// Extract deleted content from the diff
	deletedContent := extractDeletedContent(parsedDiff)

	// Only proceed if there are deletions
	if len(deletedContent) == 0 {
		log.Printf("No deletions found in PR #%d, skipping deletion analysis", reviewData.Event.Number)
		return nil
	}

	log.Printf("Found %d code deletions in PR #%d, performing safety analysis",
		len(deletedContent), reviewData.Event.Number)

	// Flatten the codebase for AI analysis
	flattenedCodebase, err := r.codebaseFlattener.FlattenWorkspace(reviewData.Workspace.Path)
	if err != nil {
		return fmt.Errorf("failed to flatten codebase: %w", err)
	}

	log.Printf("Flattened codebase: %d files, %d lines for PR #%d",
		flattenedCodebase.TotalFiles, flattenedCodebase.TotalLines, reviewData.Event.Number)

	// Create deletion analysis request
	deletionRequest := &analyzer.DeletionAnalysisRequest{
		Codebase:       flattenedCodebase,
		DeletedContent: deletedContent,
		Context:        fmt.Sprintf("PR #%d: %s", reviewData.Event.Number, reviewData.Event.PullRequest.Title),
	}

	// Perform deletion analysis
	deletionResult, err := r.deletionAnalyzer.AnalyzeDeletions(deletionRequest)
	if err != nil {
		return fmt.Errorf("deletion analysis failed: %w", err)
	}

	// Store results in review data
	reviewData.FlattenedCodebase = flattenedCodebase
	reviewData.DeletionAnalysis = deletionResult

	// Log results
	log.Printf("Deletion analysis completed for PR #%d:", reviewData.Event.Number)
	log.Printf("  - Orphaned references: %d", len(deletionResult.OrphanedReferences))
	log.Printf("  - Safe deletions: %d", len(deletionResult.SafeDeletions))
	log.Printf("  - Warnings: %d", len(deletionResult.Warnings))
	log.Printf("  - Confidence: %.2f", deletionResult.Confidence)

	return nil
}

// performLLMReview sends the review data to the LLM for analysis
func (r *DefaultReviewOrchestrator) performLLMReview(ctx context.Context, reviewData *ReviewData) (*llm.ReviewResponse, error) {
	if r.llmClient == nil {
		return nil, fmt.Errorf("LLM client not configured")
	}

	// Validate input parameters
	if reviewData == nil {
		return nil, fmt.Errorf("review data cannot be nil")
	}
	if reviewData.Event == nil {
		return nil, fmt.Errorf("event data cannot be nil")
	}
	if reviewData.Event.PullRequest.ID == 0 {
		return nil, fmt.Errorf("pull request data is invalid (missing ID)")
	}

	// Create LLM review request
	request := &llm.ReviewRequest{
		PullRequestInfo: llm.PullRequestInfo{
			Number:      reviewData.Event.Number,
			Title:       reviewData.Event.PullRequest.Title,
			Author:      reviewData.Event.PullRequest.User.Login,
			Description: "", // Could be added to event structure if needed
			BaseBranch:  reviewData.Event.PullRequest.Base.Ref,
			HeadBranch:  reviewData.Event.PullRequest.Head.Ref,
		},
		DiffResult:     reviewData.DiffResult,
		ContextualDiff: reviewData.ContextualDiff,
		ReviewType:     llm.ReviewTypeGeneral, // Default to general review
		Instructions:   "",                    // Could be customizable
	}

	// Perform the review
	response, err := r.llmClient.ReviewCode(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("LLM review failed: %w", err)
	}

	return response, nil
}

// logReviewResults logs the LLM review results
func (r *DefaultReviewOrchestrator) logReviewResults(response *llm.ReviewResponse) {
	if response.Summary != "" {
		log.Printf("LLM Review Summary: %s", response.Summary)
	}

	log.Printf("Model: %s, Tokens Used: %d (input: %d, output: %d)",
		response.ModelUsed,
		response.TokensUsed.TotalTokens,
		response.TokensUsed.InputTokens,
		response.TokensUsed.OutputTokens)

	for i, comment := range response.Comments {
		log.Printf("Comment %d: %s:%d - %s (%s)",
			i+1,
			comment.Filename,
			comment.LineNumber,
			comment.Comment,
			comment.Severity)
	}
}

// postReviewComments posts LLM-generated comments to the GitHub PR and returns the count of successfully posted comments
func (r *DefaultReviewOrchestrator) postReviewComments(ctx context.Context, reviewData *ReviewData, reviewResponse *llm.ReviewResponse) (int, error) {
	if r.githubClient == nil {
		return 0, fmt.Errorf("GitHub client not configured")
	}

	// Get the commit SHA from the PR head
	commitID := reviewData.Event.PullRequest.Head.SHA

	// Convert LLM comments to GitHub format
	var githubComments []github.CreatePullRequestCommentRequest
	for _, llmComment := range reviewResponse.Comments {
		// Convert using the GitHub package conversion function
		commentInput := github.ReviewCommentInput{
			Filename:   llmComment.Filename,
			LineNumber: llmComment.LineNumber,
			Comment:    llmComment.Comment,
		}

		githubComment, shouldPost := github.ConvertReviewCommentToGitHub(commentInput, commitID)
		if shouldPost {
			githubComments = append(githubComments, githubComment)
		} else {
			log.Printf("Skipping comment for %s (line %d): not suitable for line-specific posting",
				llmComment.Filename, llmComment.LineNumber)
		}
	}

	if len(githubComments) == 0 {
		log.Printf("No valid line-specific comments to post for PR #%d", reviewData.Event.Number)
		return 0, nil
	}

	// Post comments in batch
	result, err := r.githubClient.CreatePullRequestComments(
		ctx,
		reviewData.Event.Repository.Owner.Login,
		reviewData.Event.Repository.Name,
		reviewData.Event.Number,
		githubComments,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to post comments: %w", err)
	}

	// Log results
	log.Printf("Posted %d comments successfully, %d failed for PR #%d",
		len(result.SuccessfulComments),
		len(result.FailedComments),
		reviewData.Event.Number)

	// Log any failed comments
	for _, failed := range result.FailedComments {
		log.Printf("Failed to post comment for %s:%d - %s",
			failed.Request.Path, failed.Request.Line, failed.Error)
	}

	return len(result.SuccessfulComments), nil
}

// extractDeletedContent extracts deleted code from a parsed diff
func extractDeletedContent(parsedDiff *analyzer.ParsedDiff) []analyzer.DeletedCode {
	var deletedContent []analyzer.DeletedCode

	for _, file := range parsedDiff.Files {
		if file.Status == "deleted" {
			// Entire file was deleted
			var content strings.Builder
			var startLine, endLine int

			for _, hunk := range file.Hunks {
				for _, line := range hunk.Lines {
					if line.Type == "removed" {
						if startLine == 0 {
							startLine = line.OldLineNo
						}
						endLine = line.OldLineNo
						content.WriteString(line.Content + "\n")
					}
				}
			}

			if content.Len() > 0 {
				deletedContent = append(deletedContent, analyzer.DeletedCode{
					File:       file.Filename,
					Content:    strings.TrimSuffix(content.String(), "\n"),
					StartLine:  startLine,
					EndLine:    endLine,
					Language:   file.Language,
					ChangeType: "deleted",
				})
			}
		} else {
			// File was modified, extract deleted sections
			for _, hunk := range file.Hunks {
				var content strings.Builder
				var startLine, endLine int
				var hasRemovedContent bool

				for _, line := range hunk.Lines {
					if line.Type == "removed" {
						if startLine == 0 {
							startLine = line.OldLineNo
						}
						endLine = line.OldLineNo
						content.WriteString(line.Content + "\n")
						hasRemovedContent = true
					}
				}

				if hasRemovedContent {
					deletedContent = append(deletedContent, analyzer.DeletedCode{
						File:       file.Filename,
						Content:    strings.TrimSuffix(content.String(), "\n"),
						StartLine:  startLine,
						EndLine:    endLine,
						Language:   file.Language,
						ChangeType: "deleted",
					})
				}
			}
		}
	}

	return deletedContent
}
