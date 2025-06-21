package review

import (
	"context"

	"github.com/your-org/review-agent/pkg/webhook"
)

type GitHubCloner interface {
	CloneRepository(ctx context.Context, owner, repo, destination string) error
	CheckoutBranch(ctx context.Context, repoPath, branch string) error
}

type FileSystemManager interface {
	CreateTempDir(prefix string) (string, error)
	RemoveAll(path string) error
	Exists(path string) bool
}

type ReviewOrchestrator interface {
	HandlePullRequest(event *PullRequestEvent) error
}

type PullRequestEvent = webhook.PullRequestEvent
type PullRequest = webhook.PullRequest
type Repository = webhook.Repository
type Branch = webhook.Branch
type User = webhook.User

type Workspace struct {
	Path        string
	Repository  *Repository
	PullRequest *PullRequest
}

type WorkspaceManager interface {
	CreateWorkspace(ctx context.Context, event *PullRequestEvent) (*Workspace, error)
	CleanupWorkspace(workspace *Workspace) error
}
