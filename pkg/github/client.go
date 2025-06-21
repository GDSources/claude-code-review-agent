package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommandExecutor interface {
	Execute(command string, args ...string) error
	ExecuteInDir(dir, command string, args ...string) error
	ExecuteInDirWithOutput(dir, command string, args ...string) ([]byte, error)
}

type defaultCommandExecutor struct{}

func (d *defaultCommandExecutor) Execute(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	return cmd.Run()
}

func (d *defaultCommandExecutor) ExecuteInDir(dir, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	return cmd.Run()
}

func (d *defaultCommandExecutor) ExecuteInDirWithOutput(dir, command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	return cmd.Output()
}

type Client struct {
	token       string
	httpClient  *http.Client
	baseURL     string
	cmdExecutor CommandExecutor
}

type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Company   string `json:"company"`
	Location  string `json:"location"`
	Bio       string `json:"bio"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:     "https://api.github.com",
		cmdExecutor: &defaultCommandExecutor{},
	}
}

func (c *Client) makeRequest(ctx context.Context, method, endpoint string) (*http.Response, error) {
	url := c.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "review-agent/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	return resp, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context) (*User, error) {
	resp, err := c.makeRequest(ctx, "GET", "/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &user, nil
}

func (c *Client) GetUser(ctx context.Context, username string) (*User, error) {
	endpoint := fmt.Sprintf("/users/%s", username)
	resp, err := c.makeRequest(ctx, "GET", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", username, err)
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &user, nil
}

func (c *Client) CloneRepository(ctx context.Context, owner, repo, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", c.token, owner, repo)

	if err := c.cmdExecutor.Execute("git", "clone", cloneURL, destination); err != nil {
		return fmt.Errorf("failed to clone repository %s/%s: %w", owner, repo, err)
	}

	return nil
}

func (c *Client) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	// Configure git credentials for this repository to ensure authentication works
	// We need to set the remote URL with the token for fetch operations to work
	if err := c.configureGitAuth(repoPath); err != nil {
		return fmt.Errorf("failed to configure git authentication: %w", err)
	}

	// First, fetch all branches to ensure we have the latest refs
	if err := c.cmdExecutor.ExecuteInDir(repoPath, "git", "fetch", "origin"); err != nil {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}

	// Try to checkout the branch (it might be a local branch or need to be created from remote)
	if err := c.cmdExecutor.ExecuteInDir(repoPath, "git", "checkout", branch); err != nil {
		// If checkout fails, try to create and checkout from origin
		if err := c.cmdExecutor.ExecuteInDir(repoPath, "git", "checkout", "-b", branch, "origin/"+branch); err != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
		}
	}

	return nil
}

// configureGitAuth configures git authentication for the repository
// by updating the remote origin URL to include the access token
func (c *Client) configureGitAuth(repoPath string) error {
	// Get the current remote origin URL to extract owner and repo
	output, err := c.cmdExecutor.ExecuteInDirWithOutput(repoPath, "git", "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("failed to get remote origin URL: %w", err)
	}

	currentURL := strings.TrimSpace(string(output))
	
	// Parse the GitHub repository from the URL
	owner, repo, err := c.parseGitHubURL(currentURL)
	if err != nil {
		return fmt.Errorf("failed to parse GitHub URL %s: %w", currentURL, err)
	}

	// Set the remote URL with authentication token
	authenticatedURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", c.token, owner, repo)
	if err := c.cmdExecutor.ExecuteInDir(repoPath, "git", "remote", "set-url", "origin", authenticatedURL); err != nil {
		return fmt.Errorf("failed to set authenticated remote URL: %w", err)
	}

	return nil
}

// parseGitHubURL extracts owner and repo from various GitHub URL formats
func (c *Client) parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle different URL formats:
	// https://github.com/owner/repo.git
	// https://x-access-token:token@github.com/owner/repo.git
	// git@github.com:owner/repo.git
	
	// Clean up the URL
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")
	
	// Handle HTTPS URLs
	if strings.Contains(url, "github.com/") {
		// Find the path after github.com/
		parts := strings.Split(url, "github.com/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("invalid GitHub URL format")
		}
		
		path := parts[1]
		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return "", "", fmt.Errorf("invalid GitHub URL format")
		}
		
		return pathParts[0], pathParts[1], nil
	}
	
	// Handle SSH URLs (git@github.com:owner/repo)
	if strings.HasPrefix(url, "git@github.com:") {
		path := strings.TrimPrefix(url, "git@github.com:")
		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return "", "", fmt.Errorf("invalid GitHub SSH URL format")
		}
		
		return pathParts[0], pathParts[1], nil
	}
	
	return "", "", fmt.Errorf("unsupported GitHub URL format: %s", url)
}
