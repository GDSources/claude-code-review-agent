package review

import (
	"context"

	"github.com/your-org/review-agent/pkg/github"
)

type GitHubClientInterface interface {
	CloneRepository(ctx context.Context, owner, repo, destination string) error
	CheckoutBranch(ctx context.Context, repoPath, branch string) error
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
