package cli

import (
	"fmt"

	"github.com/your-org/review-agent/pkg/github"
	"github.com/your-org/review-agent/pkg/review"
	"github.com/your-org/review-agent/pkg/webhook"
)

type ReviewConfig struct {
	GitHubToken  string
	ClaudeAPIKey string
}

type PRReviewer struct {
	githubClient *github.Client
	orchestrator review.ReviewOrchestrator
}

func NewPRReviewer(config *ReviewConfig) *PRReviewer {
	// Create GitHub client
	githubClient := github.NewClient(config.GitHubToken)

	// Create GitHub cloner adapter
	cloner := review.NewGitHubClonerAdapterFromClient(githubClient)

	// Create file system manager
	fsManager := review.NewDefaultFileSystemManager()

	// Create workspace manager
	workspaceManager := review.NewDefaultWorkspaceManager(cloner, fsManager)

	// Create review orchestrator
	orchestrator := review.NewDefaultReviewOrchestrator(workspaceManager)

	return &PRReviewer{
		githubClient: githubClient,
		orchestrator: orchestrator,
	}
}

func (r *PRReviewer) ReviewPR(owner, repo string, prNumber int) error {
	// Fetch PR data from GitHub API
	prEvent, err := r.fetchPREvent(owner, repo, prNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch PR data: %w", err)
	}

	// Execute review using orchestrator
	if err := r.orchestrator.HandlePullRequest(prEvent); err != nil {
		return fmt.Errorf("review execution failed: %w", err)
	}

	fmt.Printf("✓ Review completed successfully for PR #%d\n", prNumber)

	return nil
}

func (r *PRReviewer) fetchPREvent(owner, repo string, prNumber int) (*webhook.PullRequestEvent, error) {
	// TODO: Implement GitHub API calls to fetch PR data
	// For now, create a mock event structure
	event := &webhook.PullRequestEvent{
		Action: "opened",
		Number: prNumber,
		PullRequest: webhook.PullRequest{
			ID:     prNumber * 1000, // Mock ID
			Number: prNumber,
			Title:  fmt.Sprintf("PR #%d", prNumber),
			State:  "open",
			Head: webhook.Branch{
				Ref: "feature-branch",
				SHA: "abc123def456",
			},
			Base: webhook.Branch{
				Ref: "main",
				SHA: "def456ghi789",
			},
			User: webhook.User{
				ID:    1001,
				Login: "contributor",
			},
		},
		Repository: webhook.Repository{
			ID:       12345,
			Name:     repo,
			FullName: fmt.Sprintf("%s/%s", owner, repo),
			Private:  false,
			Owner: webhook.User{
				ID:    2002,
				Login: owner,
			},
		},
	}

	return event, nil
}
