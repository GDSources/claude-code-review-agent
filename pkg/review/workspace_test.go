package review

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type mockGitHubCloner struct {
	shouldFail  bool
	error       error
	clonedRepos []string
	clonedPaths []string
}

func (m *mockGitHubCloner) CloneRepository(ctx context.Context, owner, repo, destination string) error {
	m.clonedRepos = append(m.clonedRepos, fmt.Sprintf("%s/%s", owner, repo))
	m.clonedPaths = append(m.clonedPaths, destination)

	if m.shouldFail {
		return m.error
	}
	return nil
}

type mockFileSystemManager struct {
	shouldFailCreate bool
	shouldFailRemove bool
	createError      error
	removeError      error
	createdDirs      []string
	removedPaths     []string
	tempDirPrefix    string
}

func (m *mockFileSystemManager) CreateTempDir(prefix string) (string, error) {
	m.tempDirPrefix = prefix
	if m.shouldFailCreate {
		return "", m.createError
	}

	tempDir := "/tmp/" + prefix + "test123"
	m.createdDirs = append(m.createdDirs, tempDir)
	return tempDir, nil
}

func (m *mockFileSystemManager) RemoveAll(path string) error {
	m.removedPaths = append(m.removedPaths, path)
	if m.shouldFailRemove {
		return m.removeError
	}
	return nil
}

func (m *mockFileSystemManager) Exists(path string) bool {
	return true
}

func createTestEventForWorkspace() *PullRequestEvent {
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

func TestDefaultWorkspaceManager_CreateWorkspace(t *testing.T) {
	tests := []struct {
		name          string
		fsCreateFail  bool
		fsCreateError error
		clonerFail    bool
		clonerError   error
		expectError   bool
		expectCleanup bool
		errorContains string
	}{
		{
			name:        "successful workspace creation",
			expectError: false,
		},
		{
			name:          "filesystem create temp dir fails",
			fsCreateFail:  true,
			fsCreateError: fmt.Errorf("permission denied"),
			expectError:   true,
			errorContains: "failed to create temporary directory",
		},
		{
			name:          "git clone fails",
			clonerFail:    true,
			clonerError:   fmt.Errorf("repository not found"),
			expectError:   true,
			expectCleanup: true,
			errorContains: "failed to clone repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFS := &mockFileSystemManager{
				shouldFailCreate: tt.fsCreateFail,
				createError:      tt.fsCreateError,
			}

			mockCloner := &mockGitHubCloner{
				shouldFail: tt.clonerFail,
				error:      tt.clonerError,
			}

			workspaceManager := NewDefaultWorkspaceManager(mockCloner, mockFS)
			event := createTestEventForWorkspace()

			workspace, err := workspaceManager.CreateWorkspace(context.Background(), event)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errorContains)) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}

			// Check cleanup behavior when clone fails
			if tt.expectCleanup && len(mockFS.removedPaths) == 0 {
				t.Error("expected cleanup to be called when clone fails")
			}

			// Check successful workspace creation
			if !tt.expectError {
				if workspace == nil {
					t.Fatal("expected workspace to be created")
				}

				expectedRepoPath := filepath.Join(mockFS.createdDirs[0], event.Repository.Name)
				if workspace.Path != expectedRepoPath {
					t.Errorf("expected workspace path '%s', got '%s'", expectedRepoPath, workspace.Path)
				}

				if workspace.Repository.FullName != event.Repository.FullName {
					t.Errorf("expected repository '%s', got '%s'", event.Repository.FullName, workspace.Repository.FullName)
				}

				if workspace.PullRequest.Number != event.PullRequest.Number {
					t.Errorf("expected PR number %d, got %d", event.PullRequest.Number, workspace.PullRequest.Number)
				}

				// Check that clone was called with correct parameters
				if len(mockCloner.clonedRepos) != 1 {
					t.Errorf("expected 1 clone call, got %d", len(mockCloner.clonedRepos))
				} else {
					expectedRepo := fmt.Sprintf("%s/%s", event.Repository.Owner.Login, event.Repository.Name)
					if mockCloner.clonedRepos[0] != expectedRepo {
						t.Errorf("expected clone repo '%s', got '%s'", expectedRepo, mockCloner.clonedRepos[0])
					}
					if mockCloner.clonedPaths[0] != expectedRepoPath {
						t.Errorf("expected clone path '%s', got '%s'", expectedRepoPath, mockCloner.clonedPaths[0])
					}
				}
			}

			// Check temp dir creation
			if !tt.fsCreateFail && len(mockFS.createdDirs) != 1 {
				t.Errorf("expected 1 temp dir creation, got %d", len(mockFS.createdDirs))
			}
			if !tt.fsCreateFail && mockFS.tempDirPrefix != "review-agent-" {
				t.Errorf("expected temp dir prefix 'review-agent-', got '%s'", mockFS.tempDirPrefix)
			}
		})
	}
}

func TestDefaultWorkspaceManager_CleanupWorkspace(t *testing.T) {
	tests := []struct {
		name          string
		workspace     *Workspace
		fsRemoveFail  bool
		fsRemoveError error
		expectError   bool
		expectRemove  bool
		errorContains string
	}{
		{
			name: "successful cleanup",
			workspace: &Workspace{
				Path: "/tmp/test123/repo",
			},
			expectError:  false,
			expectRemove: true,
		},
		{
			name: "cleanup with filesystem error",
			workspace: &Workspace{
				Path: "/tmp/test123/repo",
			},
			fsRemoveFail:  true,
			fsRemoveError: fmt.Errorf("permission denied"),
			expectError:   true,
			expectRemove:  true,
			errorContains: "failed to cleanup workspace",
		},
		{
			name:         "cleanup nil workspace",
			workspace:    nil,
			expectError:  false,
			expectRemove: false,
		},
		{
			name: "cleanup workspace with empty path",
			workspace: &Workspace{
				Path: "",
			},
			expectError:  false,
			expectRemove: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFS := &mockFileSystemManager{
				shouldFailRemove: tt.fsRemoveFail,
				removeError:      tt.fsRemoveError,
			}

			mockCloner := &mockGitHubCloner{}
			workspaceManager := NewDefaultWorkspaceManager(mockCloner, mockFS)

			err := workspaceManager.CleanupWorkspace(tt.workspace)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errorContains)) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}

			// Check remove behavior
			if tt.expectRemove && len(mockFS.removedPaths) == 0 {
				t.Error("expected remove to be called")
			}
			if !tt.expectRemove && len(mockFS.removedPaths) > 0 {
				t.Error("expected remove not to be called")
			}

			// Check that correct path was removed (parent directory of workspace)
			if tt.expectRemove && tt.workspace != nil && tt.workspace.Path != "" {
				expectedRemovePath := filepath.Dir(tt.workspace.Path)
				if len(mockFS.removedPaths) > 0 && mockFS.removedPaths[0] != expectedRemovePath {
					t.Errorf("expected remove path '%s', got '%s'", expectedRemovePath, mockFS.removedPaths[0])
				}
			}
		})
	}
}
