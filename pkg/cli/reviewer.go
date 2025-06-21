package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/your-org/review-agent/pkg/github"
	"github.com/your-org/review-agent/pkg/review"
	"github.com/your-org/review-agent/pkg/webhook"
)

type ReviewConfig struct {
	GitHubToken  string
	ClaudeAPIKey string
}

type PRReviewer struct {
	config       *ReviewConfig
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

	// Create diff fetcher
	diffFetcher := review.NewGitHubDiffFetcherFromClient(githubClient)

	// Create code analyzer
	codeAnalyzer := review.NewDefaultAnalyzerAdapter()

	// Create review orchestrator
	orchestrator := review.NewDefaultReviewOrchestrator(workspaceManager, diffFetcher, codeAnalyzer)

	return &PRReviewer{
		config:       config,
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
	// Fetch actual PR data from GitHub API
	ctx := context.Background()
	prData, err := r.fetchGitHubPR(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR data from GitHub: %w", err)
	}

	// Convert GitHub API response to webhook event structure
	event := &webhook.PullRequestEvent{
		Action: "opened",
		Number: prNumber,
		PullRequest: webhook.PullRequest{
			ID:     prData.ID,
			Number: prData.Number,
			Title:  prData.Title,
			State:  prData.State,
			Head: webhook.Branch{
				Ref: prData.Head.Ref,
				SHA: prData.Head.SHA,
			},
			Base: webhook.Branch{
				Ref: prData.Base.Ref,
				SHA: prData.Base.SHA,
			},
			User: webhook.User{
				ID:    prData.User.ID,
				Login: prData.User.Login,
			},
		},
		Repository: webhook.Repository{
			ID:       prData.Repository.ID,
			Name:     repo,
			FullName: fmt.Sprintf("%s/%s", owner, repo),
			Private:  prData.Repository.Private,
			Owner: webhook.User{
				ID:    prData.Repository.Owner.ID,
				Login: owner,
			},
		},
	}

	return event, nil
}

// GitHub API response structures
type GitHubPRResponse struct {
	ID         int                 `json:"id"`
	Number     int                 `json:"number"`
	Title      string              `json:"title"`
	State      string              `json:"state"`
	Head       GitHubBranchRef     `json:"head"`
	Base       GitHubBranchRef     `json:"base"`
	User       GitHubUser          `json:"user"`
	Repository GitHubRepository    `json:"head_repository"`
}

type GitHubBranchRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type GitHubUser struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

type GitHubRepository struct {
	ID      int        `json:"id"`
	Private bool       `json:"private"`
	Owner   GitHubUser `json:"owner"`
}

func (r *PRReviewer) fetchGitHubPR(ctx context.Context, owner, repo string, prNumber int) (*GitHubPRResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("Authorization", "Bearer "+r.config.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "review-agent/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var prData GitHubPRResponse
	if err := json.NewDecoder(resp.Body).Decode(&prData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &prData, nil
}
