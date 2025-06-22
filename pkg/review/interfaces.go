package review

import (
	"context"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
	"github.com/GDSources/claude-code-review-agent/pkg/webhook"
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
	HandlePullRequest(event *PullRequestEvent) (*ReviewResult, error)
}

// ReviewResult contains the outcome of a review operation
type ReviewResult struct {
	CommentsPosted int    `json:"comments_posted"`
	Status         string `json:"status"`
	Summary        string `json:"summary,omitempty"`
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

// DiffFetcher fetches PR diffs from GitHub API
type DiffFetcher interface {
	GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error)
}

// CodeAnalyzer processes diffs and extracts structured information for LLM analysis
type CodeAnalyzer interface {
	ParseDiff(rawDiff string) (*analyzer.ParsedDiff, error)
	ExtractContext(parsedDiff *analyzer.ParsedDiff, contextLines int) (*analyzer.ContextualDiff, error)
}

// DeletionAnalyzer analyzes code deletions for orphaned references
type DeletionAnalyzer interface {
	AnalyzeDeletions(request *analyzer.DeletionAnalysisRequest) (*analyzer.DeletionAnalysisResult, error)
}

// CodebaseFlattener flattens codebase for AI analysis
type CodebaseFlattener interface {
	FlattenWorkspace(workspacePath string) (*analyzer.FlattenedCodebase, error)
	FlattenDiff(workspacePath string, diff *analyzer.ParsedDiff) (*analyzer.FlattenedCodebase, error)
}

// ReviewData contains all information needed for LLM analysis
type ReviewData struct {
	Event             *PullRequestEvent                `json:"event"`
	Workspace         *Workspace                       `json:"workspace"`
	DiffResult        *github.DiffResult               `json:"diff_result"`
	ContextualDiff    *analyzer.ContextualDiff         `json:"contextual_diff"`
	FlattenedCodebase *analyzer.FlattenedCodebase      `json:"flattened_codebase,omitempty"`
	DeletionAnalysis  *analyzer.DeletionAnalysisResult `json:"deletion_analysis,omitempty"`
}
