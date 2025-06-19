package review

import (
	"context"
	"fmt"
	"log"
)

type DefaultReviewOrchestrator struct {
	workspaceManager WorkspaceManager
}

func NewDefaultReviewOrchestrator(workspaceManager WorkspaceManager) *DefaultReviewOrchestrator {
	return &DefaultReviewOrchestrator{
		workspaceManager: workspaceManager,
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

	// TODO: Add code analysis, LLM review, and comment posting here
	log.Printf("Review completed for PR #%d", event.Number)

	return nil
}
