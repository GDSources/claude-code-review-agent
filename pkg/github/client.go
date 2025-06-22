package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// PR Comment related structures

// CreatePullRequestCommentRequest represents a request to create a PR comment
type CreatePullRequestCommentRequest struct {
	Body     string `json:"body"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`
	CommitID string `json:"commit_id"`
}

// PullRequestComment represents a comment on a pull request
type PullRequestComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"`
	CommitID  string `json:"commit_id"`
	User      User   `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
}

// CommentPostingResult represents the result of batch comment posting
type CommentPostingResult struct {
	SuccessfulComments []PullRequestComment `json:"successful_comments"`
	FailedComments     []FailedComment      `json:"failed_comments"`
}

// FailedComment represents a comment that failed to post
type FailedComment struct {
	Request CreatePullRequestCommentRequest `json:"request"`
	Error   string                          `json:"error"`
}

// IssueComment represents a general comment on an issue/PR (not line-specific)
type IssueComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	User      User   `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
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

// Diff-related data structures
type PullRequestFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	BlobURL     string `json:"blob_url"`
	RawURL      string `json:"raw_url"`
	ContentsURL string `json:"contents_url"`
	Patch       string `json:"patch"`
	SHA         string `json:"sha"`
	PreviousSHA string `json:"previous_filename"`
}

type DiffResult struct {
	Files      []PullRequestFile `json:"files"`
	RawDiff    string            `json:"raw_diff"`
	TotalFiles int               `json:"total_files"`
}

// makeRequestWithCustomAccept makes a GitHub API request with custom Accept header
func (c *Client) makeRequestWithCustomAccept(ctx context.Context, method, endpoint, acceptHeader string) (*http.Response, error) {
	url := c.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", acceptHeader)
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

// GetPullRequestDiff fetches the unified diff for a pull request
func (c *Client) GetPullRequestDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	resp, err := c.makeRequestWithCustomAccept(ctx, "GET", endpoint, "application/vnd.github.v3.diff")
	if err != nil {
		return "", fmt.Errorf("failed to get PR diff: %w", err)
	}
	defer resp.Body.Close()

	diffBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read diff response: %w", err)
	}

	return string(diffBytes), nil
}

// GetPullRequestFiles fetches the list of files changed in a pull request
func (c *Client) GetPullRequestFiles(ctx context.Context, owner, repo string, prNumber int) ([]PullRequestFile, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, prNumber)
	resp, err := c.makeRequest(ctx, "GET", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR files: %w", err)
	}
	defer resp.Body.Close()

	var files []PullRequestFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode PR files response: %w", err)
	}

	return files, nil
}

// GetPullRequestDiffWithFiles fetches both the unified diff and file metadata
func (c *Client) GetPullRequestDiffWithFiles(ctx context.Context, owner, repo string, prNumber int) (*DiffResult, error) {
	// Fetch the unified diff
	diff, err := c.GetPullRequestDiff(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	// Fetch the file metadata
	files, err := c.GetPullRequestFiles(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	return &DiffResult{
		Files:      files,
		RawDiff:    diff,
		TotalFiles: len(files),
	}, nil
}

// makeRequestWithBody makes an HTTP request with a JSON body
func (c *Client) makeRequestWithBody(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	url := c.baseURL + endpoint

	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
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

// CreatePullRequestComment creates a new comment on a pull request
func (c *Client) CreatePullRequestComment(ctx context.Context, owner, repo string, prNumber int, comment CreatePullRequestCommentRequest) (*PullRequestComment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repo, prNumber)

	resp, err := c.makeRequestWithBody(ctx, "POST", endpoint, comment)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR comment: %w", err)
	}
	defer resp.Body.Close()

	var prComment PullRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&prComment); err != nil {
		return nil, fmt.Errorf("failed to decode PR comment response: %w", err)
	}

	return &prComment, nil
}

// CreatePullRequestComments creates multiple comments on a pull request
func (c *Client) CreatePullRequestComments(ctx context.Context, owner, repo string, prNumber int, comments []CreatePullRequestCommentRequest) (*CommentPostingResult, error) {
	result := &CommentPostingResult{
		SuccessfulComments: make([]PullRequestComment, 0),
		FailedComments:     make([]FailedComment, 0),
	}

	for _, comment := range comments {
		prComment, err := c.CreatePullRequestComment(ctx, owner, repo, prNumber, comment)
		if err != nil {
			result.FailedComments = append(result.FailedComments, FailedComment{
				Request: comment,
				Error:   err.Error(),
			})
		} else {
			result.SuccessfulComments = append(result.SuccessfulComments, *prComment)
		}
	}

	return result, nil
}

// GetPullRequestComments retrieves existing comments on a pull request
func (c *Client) GetPullRequestComments(ctx context.Context, owner, repo string, prNumber int) ([]PullRequestComment, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repo, prNumber)

	resp, err := c.makeRequest(ctx, "GET", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR comments: %w", err)
	}
	defer resp.Body.Close()

	var comments []PullRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode PR comments response: %w", err)
	}

	return comments, nil
}

// ReviewCommentInput represents input for creating a GitHub comment (avoids import cycle)
type ReviewCommentInput struct {
	Filename   string
	LineNumber int
	Comment    string
}

// ConvertReviewCommentToGitHub converts a ReviewCommentInput to GitHub API format
func ConvertReviewCommentToGitHub(reviewComment ReviewCommentInput, commitID string) (CreatePullRequestCommentRequest, bool) {
	// Skip comments without valid line numbers
	if reviewComment.LineNumber <= 0 {
		return CreatePullRequestCommentRequest{}, false
	}

	return CreatePullRequestCommentRequest{
		Body:     reviewComment.Comment,
		Path:     reviewComment.Filename,
		Line:     reviewComment.LineNumber,
		Side:     "RIGHT", // Always comment on the new version
		CommitID: commitID,
	}, true
}

// CreateIssueComment creates a general comment on an issue/PR
func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*IssueComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueNumber)
	
	requestBody := map[string]string{
		"body": body,
	}
	
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create issue comment: HTTP %d - %s", resp.StatusCode, string(bodyBytes))
	}
	
	var comment IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &comment, nil
}

// UpdateIssueComment updates an existing issue comment
func (c *Client) UpdateIssueComment(ctx context.Context, owner, repo string, commentID int, body string) (*IssueComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", c.baseURL, owner, repo, commentID)
	
	requestBody := map[string]string{
		"body": body,
	}
	
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update issue comment: HTTP %d - %s", resp.StatusCode, string(bodyBytes))
	}
	
	var comment IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &comment, nil
}

// FindProgressComment finds an existing progress comment by looking for a marker
func (c *Client) FindProgressComment(ctx context.Context, owner, repo string, issueNumber int) (*IssueComment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueNumber)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get issue comments: HTTP %d - %s", resp.StatusCode, string(bodyBytes))
	}
	
	var comments []IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Look for the progress comment marker
	const progressMarker = "review-agent:progress-comment"
	for _, comment := range comments {
		if strings.Contains(comment.Body, progressMarker) {
			return &comment, nil
		}
	}
	
	// Return nil if no progress comment found (not an error)
	return nil, nil
}
