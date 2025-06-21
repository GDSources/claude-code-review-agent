package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	token := "test-token"
	client := NewClient(token)

	if client.token != token {
		t.Errorf("expected token %s, got %s", token, client.token)
	}

	if client.baseURL != "https://api.github.com" {
		t.Errorf("expected baseURL https://api.github.com, got %s", client.baseURL)
	}

	if client.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
}

func TestGetAuthenticatedUser(t *testing.T) {
	expectedUser := &User{
		ID:        12345,
		Login:     "testuser",
		Name:      "Test User",
		Email:     "test@example.com",
		AvatarURL: "https://avatars.githubusercontent.com/u/12345",
		Company:   "Test Corp",
		Location:  "Test City",
		Bio:       "Test bio",
		CreatedAt: "2020-01-01T00:00:00Z",
		UpdatedAt: "2023-01-01T00:00:00Z",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("expected path /user, got %s", r.URL.Path)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("expected Authorization header 'Bearer test-token', got %s", authHeader)
		}

		acceptHeader := r.Header.Get("Accept")
		if acceptHeader != "application/vnd.github.v3+json" {
			t.Errorf("expected Accept header 'application/vnd.github.v3+json', got %s", acceptHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedUser)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	user, err := client.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != expectedUser.ID {
		t.Errorf("expected ID %d, got %d", expectedUser.ID, user.ID)
	}

	if user.Login != expectedUser.Login {
		t.Errorf("expected Login %s, got %s", expectedUser.Login, user.Login)
	}

	if user.Name != expectedUser.Name {
		t.Errorf("expected Name %s, got %s", expectedUser.Name, user.Name)
	}
}

func TestGetUser(t *testing.T) {
	expectedUser := &User{
		ID:        67890,
		Login:     "anotheruser",
		Name:      "Another User",
		AvatarURL: "https://avatars.githubusercontent.com/u/67890",
		Company:   "Another Corp",
		Location:  "Another City",
		Bio:       "Another bio",
		CreatedAt: "2021-01-01T00:00:00Z",
		UpdatedAt: "2023-06-01T00:00:00Z",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/anotheruser" {
			t.Errorf("expected path /users/anotheruser, got %s", r.URL.Path)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("expected Authorization header 'Bearer test-token', got %s", authHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedUser)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	user, err := client.GetUser(context.Background(), "anotheruser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != expectedUser.ID {
		t.Errorf("expected ID %d, got %d", expectedUser.ID, user.ID)
	}

	if user.Login != expectedUser.Login {
		t.Errorf("expected Login %s, got %s", expectedUser.Login, user.Login)
	}
}

func TestMakeRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient("invalid-token")
	client.baseURL = server.URL

	_, err := client.GetAuthenticatedUser(context.Background())
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

type mockCommandExecutor struct {
	commands []string
	args     [][]string
	dirs     []string
	error    error
	outputs  map[string]string // Map of command patterns to outputs
}

func (m *mockCommandExecutor) Execute(command string, args ...string) error {
	m.commands = append(m.commands, command)
	m.args = append(m.args, args)
	m.dirs = append(m.dirs, "") // No directory specified for Execute
	return m.error
}

func (m *mockCommandExecutor) ExecuteInDir(dir, command string, args ...string) error {
	m.commands = append(m.commands, command)
	m.args = append(m.args, args)
	m.dirs = append(m.dirs, dir)
	return m.error
}

func (m *mockCommandExecutor) ExecuteInDirWithOutput(dir, command string, args ...string) ([]byte, error) {
	m.commands = append(m.commands, command)
	m.args = append(m.args, args)
	m.dirs = append(m.dirs, dir)

	if m.error != nil {
		return nil, m.error
	}

	// Build command key for lookup
	cmdKey := command
	for _, arg := range args {
		cmdKey += " " + arg
	}

	if output, exists := m.outputs[cmdKey]; exists {
		return []byte(output), nil
	}

	// Default output for git remote get-url origin
	if command == "git" && len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" && args[2] == "origin" {
		return []byte("https://github.com/testowner/testrepo.git\n"), nil
	}

	return []byte(""), nil
}

func TestCloneRepository(t *testing.T) {
	tests := []struct {
		name          string
		owner         string
		repo          string
		destination   string
		mockError     error
		expectedError bool
		expectedCmds  []string
	}{
		{
			name:         "successful clone",
			owner:        "testowner",
			repo:         "testrepo",
			destination:  "/tmp/test-clone",
			mockError:    nil,
			expectedCmds: []string{"git"},
		},
		{
			name:          "git command fails",
			owner:         "testowner",
			repo:          "testrepo",
			destination:   "/tmp/test-clone",
			mockError:     fmt.Errorf("git clone failed"),
			expectedError: true,
			expectedCmds:  []string{"git"},
		},
		{
			name:         "clone with auth token",
			owner:        "privateowner",
			repo:         "privaterepo",
			destination:  "/tmp/private-clone",
			mockError:    nil,
			expectedCmds: []string{"git"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockCommandExecutor{error: tt.mockError}
			client := NewClient("test-token")
			client.cmdExecutor = mockExec

			err := client.CloneRepository(context.Background(), tt.owner, tt.repo, tt.destination)

			if tt.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(mockExec.commands) != len(tt.expectedCmds) {
				t.Errorf("expected %d commands, got %d", len(tt.expectedCmds), len(mockExec.commands))
			}

			for i, expectedCmd := range tt.expectedCmds {
				if i < len(mockExec.commands) && mockExec.commands[i] != expectedCmd {
					t.Errorf("expected command[%d] to be %s, got %s", i, expectedCmd, mockExec.commands[i])
				}
			}

			if len(mockExec.args) > 0 {
				args := mockExec.args[0]
				expectedURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", "test-token", tt.owner, tt.repo)
				if len(args) >= 3 && args[0] == "clone" {
					if args[1] != expectedURL {
						t.Errorf("expected clone URL %s, got %s", expectedURL, args[1])
					}
					if args[2] != tt.destination {
						t.Errorf("expected destination %s, got %s", tt.destination, args[2])
					}
				}
			}
		})
	}
}

func TestCloneRepositoryDirectoryCreation(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "github-client-test")
	defer os.RemoveAll(tmpDir)

	mockExec := &mockCommandExecutor{}
	client := NewClient("test-token")
	client.cmdExecutor = mockExec

	destination := filepath.Join(tmpDir, "nested", "path", "repo")
	err := client.CloneRepository(context.Background(), "owner", "repo", destination)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	parentDir := filepath.Dir(destination)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("expected parent directory to be created")
	}
}

func TestCheckoutBranch(t *testing.T) {
	tests := []struct {
		name          string
		repoPath      string
		branch        string
		mockError     error
		expectedError bool
		expectedCmds  []string
		expectedDirs  []string
		setupError    bool // Error during authentication setup
	}{
		{
			name:         "successful checkout existing branch",
			repoPath:     "/tmp/test-repo",
			branch:       "feature-branch",
			mockError:    nil,
			expectedCmds: []string{"git", "git", "git", "git"}, // get-url, set-url, fetch, checkout
			expectedDirs: []string{"/tmp/test-repo", "/tmp/test-repo", "/tmp/test-repo", "/tmp/test-repo"},
		},
		{
			name:          "authentication setup fails",
			repoPath:      "/tmp/test-repo",
			branch:        "feature-branch",
			mockError:     fmt.Errorf("auth failed"),
			expectedError: true,
			setupError:    true,
			expectedCmds:  []string{"git"}, // Only get-url command before failure
			expectedDirs:  []string{"/tmp/test-repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockCommandExecutor{error: tt.mockError}
			client := NewClient("test-token")
			client.cmdExecutor = mockExec

			err := client.CheckoutBranch(context.Background(), tt.repoPath, tt.branch)

			if tt.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(mockExec.commands) != len(tt.expectedCmds) {
				t.Errorf("expected %d commands, got %d", len(tt.expectedCmds), len(mockExec.commands))
			}

			for i, expectedCmd := range tt.expectedCmds {
				if i >= len(mockExec.commands) {
					t.Errorf("missing command %d: expected %s", i, expectedCmd)
					continue
				}
				if mockExec.commands[i] != expectedCmd {
					t.Errorf("command %d: expected %s, got %s", i, expectedCmd, mockExec.commands[i])
				}
			}

			for i, expectedDir := range tt.expectedDirs {
				if i >= len(mockExec.dirs) {
					t.Errorf("missing directory %d: expected %s", i, expectedDir)
					continue
				}
				if mockExec.dirs[i] != expectedDir {
					t.Errorf("directory %d: expected %s, got %s", i, expectedDir, mockExec.dirs[i])
				}
			}
		})
	}
}

func TestParseGitHubURL(t *testing.T) {
	client := NewClient("test-token")

	tests := []struct {
		name          string
		url           string
		expectedOwner string
		expectedRepo  string
		expectedError bool
	}{
		{
			name:          "HTTPS URL",
			url:           "https://github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedError: false,
		},
		{
			name:          "HTTPS URL with auth token",
			url:           "https://x-access-token:token@github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedError: false,
		},
		{
			name:          "SSH URL",
			url:           "git@github.com:owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedError: false,
		},
		{
			name:          "URL without .git suffix",
			url:           "https://github.com/owner/repo",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedError: false,
		},
		{
			name:          "URL with whitespace",
			url:           "  https://github.com/owner/repo.git\n",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedError: false,
		},
		{
			name:          "invalid URL format",
			url:           "invalid-url",
			expectedOwner: "",
			expectedRepo:  "",
			expectedError: true,
		},
		{
			name:          "incomplete GitHub URL",
			url:           "https://github.com/owner",
			expectedOwner: "",
			expectedRepo:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := client.parseGitHubURL(tt.url)

			if tt.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if owner != tt.expectedOwner {
				t.Errorf("expected owner %s, got %s", tt.expectedOwner, owner)
			}
			if repo != tt.expectedRepo {
				t.Errorf("expected repo %s, got %s", tt.expectedRepo, repo)
			}
		})
	}
}

func TestGetPullRequestDiff(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		prNumber     int
		mockResponse string
		mockError    error
		expectedDiff string
		expectError  bool
	}{
		{
			name:         "successful diff fetch",
			owner:        "testowner",
			repo:         "testrepo",
			prNumber:     123,
			mockResponse: "diff --git a/file.go b/file.go\n@@ -1,3 +1,4 @@\n func main() {\n+\tfmt.Println(\"Hello\")\n }\n",
			expectedDiff: "diff --git a/file.go b/file.go\n@@ -1,3 +1,4 @@\n func main() {\n+\tfmt.Println(\"Hello\")\n }\n",
			expectError:  false,
		},
		{
			name:        "API error",
			owner:       "testowner",
			repo:        "testrepo",
			prNumber:    123,
			mockError:   fmt.Errorf("API request failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", tt.owner, tt.repo, tt.prNumber)
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Check Accept header
				if r.Header.Get("Accept") != "application/vnd.github.v3.diff" {
					t.Errorf("expected Accept header 'application/vnd.github.v3.diff', got '%s'", r.Header.Get("Accept"))
				}

				if tt.mockError != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.baseURL = server.URL

			diff, err := client.GetPullRequestDiff(context.Background(), tt.owner, tt.repo, tt.prNumber)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && diff != tt.expectedDiff {
				t.Errorf("expected diff %q, got %q", tt.expectedDiff, diff)
			}
		})
	}
}

func TestGetPullRequestFiles(t *testing.T) {
	tests := []struct {
		name          string
		owner         string
		repo          string
		prNumber      int
		mockResponse  string
		mockError     error
		expectedFiles int
		expectError   bool
	}{
		{
			name:     "successful files fetch",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			mockResponse: `[
				{
					"filename": "file1.go",
					"status": "modified",
					"additions": 10,
					"deletions": 5,
					"changes": 15,
					"patch": "@@ -1,3 +1,4 @@\n func main() {\n+\tfmt.Println(\"Hello\")\n }\n"
				},
				{
					"filename": "file2.go",
					"status": "added",
					"additions": 20,
					"deletions": 0,
					"changes": 20,
					"patch": "@@ -0,0 +1,20 @@\n+package main\n+\n+func newFunc() {\n+}\n"
				}
			]`,
			expectedFiles: 2,
			expectError:   false,
		},
		{
			name:        "API error",
			owner:       "testowner",
			repo:        "testrepo",
			prNumber:    123,
			mockError:   fmt.Errorf("API request failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", tt.owner, tt.repo, tt.prNumber)
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}

				if tt.mockError != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.baseURL = server.URL

			files, err := client.GetPullRequestFiles(context.Background(), tt.owner, tt.repo, tt.prNumber)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && len(files) != tt.expectedFiles {
				t.Errorf("expected %d files, got %d", tt.expectedFiles, len(files))
			}
			if !tt.expectError && len(files) > 0 {
				// Verify first file structure
				file := files[0]
				if file.Filename != "file1.go" {
					t.Errorf("expected filename 'file1.go', got '%s'", file.Filename)
				}
				if file.Status != "modified" {
					t.Errorf("expected status 'modified', got '%s'", file.Status)
				}
				if file.Additions != 10 {
					t.Errorf("expected 10 additions, got %d", file.Additions)
				}
				if file.Deletions != 5 {
					t.Errorf("expected 5 deletions, got %d", file.Deletions)
				}
			}
		})
	}
}

func TestGetPullRequestDiffWithFiles(t *testing.T) {
	mockDiff := "diff --git a/file.go b/file.go\n@@ -1,3 +1,4 @@\n func main() {\n+\tfmt.Println(\"Hello\")\n }\n"
	mockFiles := `[{"filename": "file.go", "status": "modified", "additions": 1, "deletions": 0, "changes": 1}]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle both diff and files endpoints
		if strings.Contains(r.URL.Path, "/files") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(mockFiles))
		} else if r.Header.Get("Accept") == "application/vnd.github.v3.diff" {
			w.Write([]byte(mockDiff))
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	result, err := client.GetPullRequestDiffWithFiles(context.Background(), "owner", "repo", 123)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if result.RawDiff != mockDiff {
		t.Errorf("expected raw diff %q, got %q", mockDiff, result.RawDiff)
	}
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(result.Files))
	}
	if result.TotalFiles != 1 {
		t.Errorf("expected total files 1, got %d", result.TotalFiles)
	}
}
