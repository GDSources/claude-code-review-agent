package review

import (
	"context"
	"fmt"
	"log"

	"github.com/your-org/review-agent/pkg/analyzer"
	"github.com/your-org/review-agent/pkg/github"
	"github.com/your-org/review-agent/pkg/llm"
)

type DefaultReviewOrchestrator struct {
	workspaceManager WorkspaceManager
	diffFetcher      DiffFetcher
	codeAnalyzer     CodeAnalyzer
	llmClient        llm.CodeReviewer
}

func NewDefaultReviewOrchestrator(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager: workspaceManager,
		diffFetcher:      diffFetcher,
		codeAnalyzer:     codeAnalyzer,
		llmClient:        nil,
	}
}

// NewReviewOrchestratorWithLLM creates orchestrator with LLM integration
func NewReviewOrchestratorWithLLM(workspaceManager WorkspaceManager, diffFetcher DiffFetcher, codeAnalyzer CodeAnalyzer, llmClient llm.CodeReviewer) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager: workspaceManager,
		diffFetcher:      diffFetcher,
		codeAnalyzer:     codeAnalyzer,
		llmClient:        llmClient,
	}
}

// NewDefaultReviewOrchestratorLegacy creates orchestrator without diff analysis (for backward compatibility)
func NewDefaultReviewOrchestratorLegacy(workspaceManager WorkspaceManager) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager: workspaceManager,
		diffFetcher:      nil,
		codeAnalyzer:     nil,
		llmClient:        nil,
	}
}

func (r *DefaultReviewOrchestrator) HandlePullRequest(event *PullRequestEvent) error {
	ctx := context.Background()

	log.Printf("Starting review for PR #%d in %s", event.Number, event.Repository.FullName)

	workspace, err := r.workspaceManager.CreateWorkspace(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to create workspace for PR #%d: %w", event.Number, err)
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
			}
		}
	} else {
		log.Printf("Diff analysis skipped (analyzers not configured)")
	}

	// Send reviewData to LLM for analysis if available
	if reviewData != nil && r.llmClient != nil {
		log.Printf("Sending PR #%d to LLM for analysis", event.Number)
		
		reviewResponse, err := r.performLLMReview(ctx, reviewData)
		if err != nil {
			log.Printf("Warning: LLM review failed for PR #%d: %v", event.Number, err)
		} else {
			log.Printf("LLM review completed for PR #%d: %d comments generated", 
				event.Number, len(reviewResponse.Comments))
			
			// TODO: Post generated comments back to GitHub PR
			r.logReviewResults(reviewResponse)
		}
	} else if reviewData != nil {
		log.Printf("Review data prepared for PR #%d (LLM not configured)", event.Number)
	}

	log.Printf("Review completed for PR #%d", event.Number)
	return nil
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

// performLLMReview sends the review data to the LLM for analysis
func (r *DefaultReviewOrchestrator) performLLMReview(ctx context.Context, reviewData *ReviewData) (*llm.ReviewResponse, error) {
	if r.llmClient == nil {
		return nil, fmt.Errorf("LLM client not configured")
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
		Instructions:   "", // Could be customizable
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
