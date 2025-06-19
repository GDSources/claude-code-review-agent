package review

import (
	"context"
	"fmt"
	"path/filepath"
)

type DefaultWorkspaceManager struct {
	cloner GitHubCloner
	fs     FileSystemManager
}

func NewDefaultWorkspaceManager(cloner GitHubCloner, fs FileSystemManager) *DefaultWorkspaceManager {
	return &DefaultWorkspaceManager{
		cloner: cloner,
		fs:     fs,
	}
}

func (w *DefaultWorkspaceManager) CreateWorkspace(ctx context.Context, event *PullRequestEvent) (*Workspace, error) {
	tempDir, err := w.fs.CreateTempDir("review-agent-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	repoPath := filepath.Join(tempDir, event.Repository.Name)

	if err := w.cloner.CloneRepository(ctx, event.Repository.Owner.Login, event.Repository.Name, repoPath); err != nil {
		w.fs.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone repository %s/%s: %w",
			event.Repository.Owner.Login, event.Repository.Name, err)
	}

	workspace := &Workspace{
		Path:        repoPath,
		Repository:  &event.Repository,
		PullRequest: &event.PullRequest,
	}

	return workspace, nil
}

func (w *DefaultWorkspaceManager) CleanupWorkspace(workspace *Workspace) error {
	if workspace == nil || workspace.Path == "" {
		return nil
	}

	tempDir := filepath.Dir(workspace.Path)
	if err := w.fs.RemoveAll(tempDir); err != nil {
		return fmt.Errorf("failed to cleanup workspace at %s: %w", tempDir, err)
	}

	return nil
}
