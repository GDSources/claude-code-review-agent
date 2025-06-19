package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	error    error
}

func (m *mockCommandExecutor) Execute(command string, args ...string) error {
	m.commands = append(m.commands, command)
	m.args = append(m.args, args)
	return m.error
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
