package review

import (
	"context"

	"github.com/your-org/review-agent/pkg/github"
)

type GitHubClientInterface interface {
	CloneRepository(ctx context.Context, owner, repo, destination string) error
	CheckoutBranch(ctx context.Context, repoPath, branch string) error
	GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error)
}

type GitHubClonerAdapter struct {
	client GitHubClientInterface
}

func NewGitHubClonerAdapter(client GitHubClientInterface) *GitHubClonerAdapter {
	return &GitHubClonerAdapter{
		client: client,
	}
}

func (g *GitHubClonerAdapter) CloneRepository(ctx context.Context, owner, repo, destination string) error {
	return g.client.CloneRepository(ctx, owner, repo, destination)
}

func (g *GitHubClonerAdapter) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	return g.client.CheckoutBranch(ctx, repoPath, branch)
}

func NewGitHubClonerAdapterFromClient(client *github.Client) *GitHubClonerAdapter {
	return &GitHubClonerAdapter{
		client: client,
	}
}

// GitHubDiffFetcher adapter for GitHub client to implement DiffFetcher interface
type GitHubDiffFetcher struct {
	client GitHubClientInterface
}

func NewGitHubDiffFetcher(client GitHubClientInterface) *GitHubDiffFetcher {
	return &GitHubDiffFetcher{
		client: client,
	}
}

func (g *GitHubDiffFetcher) GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error) {
	return g.client.GetPullRequestDiffWithFiles(ctx, owner, repo, prNumber)
}

func NewGitHubDiffFetcherFromClient(client *github.Client) *GitHubDiffFetcher {
	return &GitHubDiffFetcher{
		client: client,
	}
}
