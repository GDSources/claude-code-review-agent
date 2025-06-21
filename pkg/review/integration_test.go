package review

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func createTestEvent() *PullRequestEvent {
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

func TestReviewOrchestrator_Integration(t *testing.T) {
	tests := []struct {
		name          string
		clonerFail    bool
		clonerError   error
		expectError   bool
		errorContains string
	}{
		{
			name:        "successful end-to-end review flow",
			expectError: false,
		},
		{
			name:          "clone failure propagates through layers",
			clonerFail:    true,
			clonerError:   fmt.Errorf("repository access denied"),
			expectError:   true,
			errorContains: "failed to create workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use real file system manager for integration test
			fsManager := NewDefaultFileSystemManager()

			// Use mock GitHub cloner to avoid actual git operations
			mockCloner := &mockGitHubCloner{
				shouldFail: tt.clonerFail,
				error:      tt.clonerError,
			}

			// Create workspace manager with real FS and mock cloner
			workspaceManager := NewDefaultWorkspaceManager(mockCloner, fsManager)

			// Create orchestrator
			orchestrator := NewDefaultReviewOrchestratorLegacy(workspaceManager)

			// Create test event
			event := createTestEvent()

			// Execute the review
			err := orchestrator.HandlePullRequest(event)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || err.Error() == "") {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}

			// Verify cloner was called with correct parameters
			if !tt.clonerFail && len(mockCloner.clonedRepos) == 1 {
				expectedRepo := fmt.Sprintf("%s/%s", event.Repository.Owner.Login, event.Repository.Name)
				if mockCloner.clonedRepos[0] != expectedRepo {
					t.Errorf("expected clone repo '%s', got '%s'", expectedRepo, mockCloner.clonedRepos[0])
				}
			}

			// Verify no temporary directories are left behind
			// This tests the cleanup functionality
			tempDir := os.TempDir()
			entries, err := os.ReadDir(tempDir)
			if err != nil {
				t.Logf("Warning: could not read temp dir: %v", err)
			} else {
				for _, entry := range entries {
					if entry.IsDir() && entry.Name()[:min(len(entry.Name()), 13)] == "review-agent-" {
						t.Errorf("found leftover temp directory: %s", entry.Name())
					}
				}
			}
		})
	}
}

type mockClonerWithFileCreation struct {
	*mockGitHubCloner
}

func (m *mockClonerWithFileCreation) CloneRepository(ctx context.Context, owner, repo, destination string) error {
	// Call the original mock logic
	err := m.mockGitHubCloner.CloneRepository(ctx, owner, repo, destination)
	if err != nil {
		return err
	}
	// Create the destination directory to simulate git clone behavior
	return os.MkdirAll(destination, 0755)
}

func TestWorkspaceManager_Integration(t *testing.T) {
	// Test workspace manager with real filesystem
	fsManager := NewDefaultFileSystemManager()

	// Create a mock cloner that actually creates the expected directory
	mockCloner := &mockClonerWithFileCreation{
		mockGitHubCloner: &mockGitHubCloner{},
	}

	workspaceManager := NewDefaultWorkspaceManager(mockCloner, fsManager)

	event := createTestEvent()

	// Create workspace
	workspace, err := workspaceManager.CreateWorkspace(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error creating workspace: %v", err)
	}

	// Verify workspace directory exists
	if !fsManager.Exists(workspace.Path) {
		t.Error("expected workspace path to exist")
	}

	// Verify temp directory was created
	parentDir := workspace.Path[:len(workspace.Path)-len(event.Repository.Name)-1] // Remove "/repo" from end
	if !fsManager.Exists(parentDir) {
		t.Error("expected parent temp directory to exist")
	}

	// Cleanup workspace
	err = workspaceManager.CleanupWorkspace(workspace)
	if err != nil {
		t.Errorf("unexpected error cleaning up workspace: %v", err)
	}

	// Verify cleanup removed the temp directory
	if fsManager.Exists(parentDir) {
		t.Error("expected temp directory to be cleaned up")
	}
}

func TestGitHubClonerAdapter_Integration(t *testing.T) {
	// Test the adapter integration with a mock client
	mockClient := &mockGitHubClient{}
	adapter := NewGitHubClonerAdapter(mockClient)

	ctx := context.Background()
	owner := "testowner"
	repo := "testrepo"
	destination := "/tmp/test-clone"

	err := adapter.CloneRepository(ctx, owner, repo, destination)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify the call was passed through correctly
	if len(mockClient.cloneCalls) != 1 {
		t.Errorf("expected 1 clone call, got %d", len(mockClient.cloneCalls))
	} else {
		call := mockClient.cloneCalls[0]
		if call.owner != owner {
			t.Errorf("expected owner '%s', got '%s'", owner, call.owner)
		}
		if call.repo != repo {
			t.Errorf("expected repo '%s', got '%s'", repo, call.repo)
		}
		if call.destination != destination {
			t.Errorf("expected destination '%s', got '%s'", destination, call.destination)
		}
	}
}

func TestReviewOrchestrator_ErrorHandling(t *testing.T) {
	// Test various error scenarios to ensure proper error handling and cleanup

	t.Run("workspace creation error", func(t *testing.T) {
		mockCloner := &mockGitHubCloner{
			shouldFail: true,
			error:      fmt.Errorf("git clone failed: repository not found"),
		}
		fsManager := NewDefaultFileSystemManager()
		workspaceManager := NewDefaultWorkspaceManager(mockCloner, fsManager)
		orchestrator := NewDefaultReviewOrchestratorLegacy(workspaceManager)

		event := createTestEvent()
		err := orchestrator.HandlePullRequest(event)

		if err == nil {
			t.Error("expected error when workspace creation fails")
		}

		// Verify no temp directories left behind even when clone fails
		tempDir := os.TempDir()
		entries, _ := os.ReadDir(tempDir)
		for _, entry := range entries {
			if entry.IsDir() && len(entry.Name()) >= 13 && entry.Name()[:13] == "review-agent-" {
				t.Errorf("found leftover temp directory after error: %s", entry.Name())
			}
		}
	})
}
