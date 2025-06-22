package review

import (
	"context"
	"fmt"
	"testing"

	"github.com/GDSources/claude-code-review-agent/pkg/github"
)

type mockGitHubClient struct {
	shouldFail    bool
	error         error
	cloneCalls    []cloneCall
	checkoutCalls []checkoutCall
}

type cloneCall struct {
	owner       string
	repo        string
	destination string
}

type checkoutCall struct {
	repoPath string
	branch   string
}

func (m *mockGitHubClient) CloneRepository(ctx context.Context, owner, repo, destination string) error {
	m.cloneCalls = append(m.cloneCalls, cloneCall{
		owner:       owner,
		repo:        repo,
		destination: destination,
	})

	if m.shouldFail {
		return m.error
	}
	return nil
}

func (m *mockGitHubClient) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	m.checkoutCalls = append(m.checkoutCalls, checkoutCall{
		repoPath: repoPath,
		branch:   branch,
	})

	if m.shouldFail {
		return m.error
	}
	return nil
}

func (m *mockGitHubClient) GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*github.DiffResult, error) {
	if m.shouldFail {
		return nil, m.error
	}

	// Return a mock diff result
	return &github.DiffResult{
		Files:      []github.PullRequestFile{},
		RawDiff:    "mock diff content",
		TotalFiles: 0,
	}, nil
}

func TestGitHubClonerAdapter_CloneRepository(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		repo        string
		destination string
		clientFail  bool
		clientError error
		expectError bool
	}{
		{
			name:        "successful clone",
			owner:       "testowner",
			repo:        "testrepo",
			destination: "/tmp/test",
			expectError: false,
		},
		{
			name:        "client error propagates",
			owner:       "testowner",
			repo:        "testrepo",
			destination: "/tmp/test",
			clientFail:  true,
			clientError: fmt.Errorf("authentication failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockGitHubClient{
				shouldFail: tt.clientFail,
				error:      tt.clientError,
			}

			// Create adapter with mock client
			adapter := &GitHubClonerAdapter{client: mockClient}

			err := adapter.CloneRepository(context.Background(), tt.owner, tt.repo, tt.destination)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.clientError != nil && err != tt.clientError {
				t.Errorf("expected error %v, got %v", tt.clientError, err)
			}

			// Verify call was made to underlying client
			if len(mockClient.cloneCalls) != 1 {
				t.Errorf("expected 1 clone call, got %d", len(mockClient.cloneCalls))
			} else {
				call := mockClient.cloneCalls[0]
				if call.owner != tt.owner {
					t.Errorf("expected owner '%s', got '%s'", tt.owner, call.owner)
				}
				if call.repo != tt.repo {
					t.Errorf("expected repo '%s', got '%s'", tt.repo, call.repo)
				}
				if call.destination != tt.destination {
					t.Errorf("expected destination '%s', got '%s'", tt.destination, call.destination)
				}
			}
		})
	}
}

func TestGitHubClonerAdapter_CheckoutBranch(t *testing.T) {
	tests := []struct {
		name        string
		repoPath    string
		branch      string
		clientFail  bool
		clientError error
		expectError bool
	}{
		{
			name:        "successful checkout",
			repoPath:    "/tmp/test-repo",
			branch:      "feature-branch",
			expectError: false,
		},
		{
			name:        "client error propagates",
			repoPath:    "/tmp/test-repo",
			branch:      "feature-branch",
			clientFail:  true,
			clientError: fmt.Errorf("checkout failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockGitHubClient{
				shouldFail: tt.clientFail,
				error:      tt.clientError,
			}

			// Create adapter with mock client
			adapter := &GitHubClonerAdapter{client: mockClient}

			err := adapter.CheckoutBranch(context.Background(), tt.repoPath, tt.branch)

			// Check error expectations
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.clientError != nil && err != tt.clientError {
				t.Errorf("expected error %v, got %v", tt.clientError, err)
			}

			// Verify call was made to underlying client
			if len(mockClient.checkoutCalls) != 1 {
				t.Errorf("expected 1 checkout call, got %d", len(mockClient.checkoutCalls))
			} else {
				call := mockClient.checkoutCalls[0]
				if call.repoPath != tt.repoPath {
					t.Errorf("expected repoPath '%s', got '%s'", tt.repoPath, call.repoPath)
				}
				if call.branch != tt.branch {
					t.Errorf("expected branch '%s', got '%s'", tt.branch, call.branch)
				}
			}
		})
	}
}

func TestNewGitHubClonerAdapter(t *testing.T) {
	mockClient := &mockGitHubClient{}
	adapter := NewGitHubClonerAdapter(mockClient)

	if adapter == nil {
		t.Error("expected adapter to be created")
		return // Exit early to avoid nil pointer dereference
	}

	// We can't directly compare interface values, but we can verify the adapter is configured
	if adapter.client == nil {
		t.Error("expected adapter to store the provided client")
	}
}
