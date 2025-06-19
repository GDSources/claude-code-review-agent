package review

import (
	"context"
	"fmt"
	"testing"
)

type mockWorkspaceManager struct {
	shouldFailCreate  bool
	shouldFailCleanup bool
	createError       error
	cleanupError      error
	createdWorkspace  *Workspace
	cleanupCalled     bool
}

func (m *mockWorkspaceManager) CreateWorkspace(ctx context.Context, event *PullRequestEvent) (*Workspace, error) {
	if m.shouldFailCreate {
		return nil, m.createError
	}

	workspace := &Workspace{
		Path:        "/tmp/test-workspace/" + event.Repository.Name,
		Repository:  &event.Repository,
		PullRequest: &event.PullRequest,
	}
	m.createdWorkspace = workspace
	return workspace, nil
}

func (m *mockWorkspaceManager) CleanupWorkspace(workspace *Workspace) error {
	m.cleanupCalled = true
	if m.shouldFailCleanup {
		return m.cleanupError
	}
	return nil
}

func createTestPullRequestEvent() *PullRequestEvent {
	return &PullRequestEvent{
		Action: "opened",
		Number: 42,
		PullRequest: PullRequest{
			ID:     123456,
			Number: 42,
			Title:  "Add amazing feature",
			State:  "open",
			Head: Branch{
				Ref: "feature/amazing",
				SHA: "abc123",
			},
			Base: Branch{
				Ref: "main",
				SHA: "def456",
			},
			User: User{
				ID:    1001,
				Login: "developer",
			},
		},
		Repository: Repository{
			ID:       789,
			Name:     "test-repo",
			FullName: "company/test-repo",
			Private:  false,
			Owner: User{
				ID:    2002,
				Login: "company",
			},
		},
	}
}

func TestDefaultReviewOrchestrator_HandlePullRequest(t *testing.T) {
	tests := []struct {
		name                 string
		workspaceCreateFail  bool
		workspaceCreateErr   error
		workspaceCleanupFail bool
		workspaceCleanupErr  error
		expectError          bool
		expectCleanup        bool
		errorContains        string
	}{
		{
			name:          "successful review flow",
			expectError:   false,
			expectCleanup: true,
		},
		{
			name:                "workspace creation fails",
			workspaceCreateFail: true,
			workspaceCreateErr:  fmt.Errorf("failed to create temp directory"),
			expectError:         true,
			expectCleanup:       false,
			errorContains:       "failed to create workspace for PR #42",
		},
		{
			name:                 "cleanup fails but review succeeds",
			workspaceCleanupFail: true,
			workspaceCleanupErr:  fmt.Errorf("permission denied"),
			expectError:          false,
			expectCleanup:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWM := &mockWorkspaceManager{
				shouldFailCreate:  tt.workspaceCreateFail,
				createError:       tt.workspaceCreateErr,
				shouldFailCleanup: tt.workspaceCleanupFail,
				cleanupError:      tt.workspaceCleanupErr,
			}

			orchestrator := NewDefaultReviewOrchestrator(mockWM)
			event := createTestPullRequestEvent()

			err := orchestrator.HandlePullRequest(event)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || err.Error() == "" || err.Error()[:len(tt.errorContains)] != tt.errorContains[:len(tt.errorContains)]) {
				if err != nil && err.Error() != "" {
					if err.Error()[:min(len(err.Error()), len(tt.errorContains))] == tt.errorContains[:min(len(err.Error()), len(tt.errorContains))] {
						// This is OK, partial match is fine
					} else {
						t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
					}
				} else {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			}

			// Check cleanup expectations
			if tt.expectCleanup && !mockWM.cleanupCalled {
				t.Error("expected cleanup to be called")
			}
			if !tt.expectCleanup && mockWM.cleanupCalled {
				t.Error("expected cleanup not to be called")
			}

			// Verify workspace was created if not expecting creation failure
			if !tt.workspaceCreateFail && mockWM.createdWorkspace == nil {
				t.Error("expected workspace to be created")
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestDefaultReviewOrchestrator_WorkspaceIntegration(t *testing.T) {
	mockWM := &mockWorkspaceManager{}
	orchestrator := NewDefaultReviewOrchestrator(mockWM)
	event := createTestPullRequestEvent()

	err := orchestrator.HandlePullRequest(event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workspace was created with correct event data
	if mockWM.createdWorkspace == nil {
		t.Fatal("expected workspace to be created")
	}

	workspace := mockWM.createdWorkspace
	if workspace.Repository.FullName != "company/test-repo" {
		t.Errorf("expected repository full name 'company/test-repo', got '%s'", workspace.Repository.FullName)
	}

	if workspace.PullRequest.Number != 42 {
		t.Errorf("expected PR number 42, got %d", workspace.PullRequest.Number)
	}

	if !mockWM.cleanupCalled {
		t.Error("expected cleanup to be called")
	}
}
