package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type CommandExecutor interface {
	Execute(command string, args ...string) error
}

type defaultCommandExecutor struct{}

func (d *defaultCommandExecutor) Execute(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	return cmd.Run()
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
