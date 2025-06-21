package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/review-agent/pkg/analyzer"
)

// ClaudeClient implements the CodeReviewer interface for Anthropic's Claude
type ClaudeClient struct {
	config     ClaudeConfig
	httpClient *http.Client
}

// Claude API request/response structures
type claudeRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []claudeMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	System      string          `json:"system,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Model   string          `json:"model"`
	Usage   claudeUsage     `json:"usage"`
	ID      string          `json:"id"`
	Type    string          `json:"type"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeError struct {
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type claudeErrorResponse struct {
	Error claudeError `json:"error"`
}

// NewClaudeClient creates a new Claude client with the given configuration
func NewClaudeClient(config ClaudeConfig) (*ClaudeClient, error) {
	// Set defaults first
	if config.Model == "" {
		config.Model = DefaultClaudeModel
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = DefaultClaudeMaxTokens
	}
	if config.Temperature == 0 {
		config.Temperature = DefaultClaudeTemperature
	}
	if config.BaseURL == "" {
		config.BaseURL = DefaultClaudeBaseURL
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeoutSeconds
	}

	// Validate configuration after applying defaults
	if err := validateClaudeConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	httpClient := &http.Client{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	return &ClaudeClient{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// ReviewCode implements the CodeReviewer interface
func (c *ClaudeClient) ReviewCode(ctx context.Context, request *ReviewRequest) (*ReviewResponse, error) {
	// Generate the review prompt
	systemPrompt := c.generateSystemPrompt(request.ReviewType)
	userPrompt := c.generateUserPrompt(request)

	// Check token limits and chunk if necessary
	chunks := c.chunkRequestIfNeeded(systemPrompt, userPrompt, request)

	var allComments []ReviewComment
	var totalTokens TokenUsage
	var summary strings.Builder

	// Process each chunk
	for i, chunk := range chunks {
		chunkResponse, err := c.processChunk(ctx, systemPrompt, chunk, i+1, len(chunks))
		if err != nil {
			return nil, fmt.Errorf("failed to process chunk %d: %w", i+1, err)
		}

		allComments = append(allComments, chunkResponse.Comments...)
		totalTokens.InputTokens += chunkResponse.TokensUsed.InputTokens
		totalTokens.OutputTokens += chunkResponse.TokensUsed.OutputTokens

		if chunkResponse.Summary != "" {
			if summary.Len() > 0 {
				summary.WriteString("\n\n")
			}
			summary.WriteString(chunkResponse.Summary)
		}
	}

	totalTokens.TotalTokens = totalTokens.InputTokens + totalTokens.OutputTokens

	return &ReviewResponse{
		Comments:    allComments,
		Summary:     summary.String(),
		ModelUsed:   c.config.Model,
		TokensUsed:  totalTokens,
		ReviewID:    fmt.Sprintf("claude-%d", time.Now().Unix()),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ValidateConfiguration checks if the Claude configuration is valid
func (c *ClaudeClient) ValidateConfiguration() error {
	return validateClaudeConfig(c.config)
}

// GetModelInfo returns information about the configured Claude model
func (c *ClaudeClient) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:        c.config.Model,
		Version:     c.config.Model,
		MaxTokens:   c.config.MaxTokens,
		Provider:    "anthropic",
		Description: fmt.Sprintf("Claude model %s with max %d tokens", c.config.Model, c.config.MaxTokens),
	}
}

// processChunk processes a single chunk of the review request
func (c *ClaudeClient) processChunk(ctx context.Context, systemPrompt, userPrompt string, chunkNum, totalChunks int) (*ReviewResponse, error) {
	// Add chunk information if multiple chunks
	finalUserPrompt := userPrompt
	if totalChunks > 1 {
		finalUserPrompt = fmt.Sprintf("This is chunk %d of %d for this code review.\n\n%s", chunkNum, totalChunks, userPrompt)
	}

	// Create Claude API request
	claudeReq := claudeRequest{
		Model:       c.config.Model,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		System:      systemPrompt,
		Messages: []claudeMessage{
			{
				Role:    "user",
				Content: finalUserPrompt,
			},
		},
	}

	// Make API request with retry logic
	claudeResp, err := c.makeRequestWithRetry(ctx, claudeReq)
	if err != nil {
		return nil, fmt.Errorf("Claude API request failed: %w", err)
	}

	// Parse response into review comments
	comments, summary, err := c.parseClaudeResponse(claudeResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %w", err)
	}

	return &ReviewResponse{
		Comments: comments,
		Summary:  summary,
		TokensUsed: TokenUsage{
			InputTokens:  claudeResp.Usage.InputTokens,
			OutputTokens: claudeResp.Usage.OutputTokens,
			TotalTokens:  claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		},
	}, nil
}

// makeRequestWithRetry makes HTTP request to Claude API with exponential backoff retry
func (c *ClaudeClient) makeRequestWithRetry(ctx context.Context, req claudeRequest) (*claudeResponse, error) {
	maxRetries := 3
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.makeRequest(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if we should retry
		if !c.shouldRetry(err) {
			break
		}
	}

	return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
}

// makeRequest makes a single HTTP request to Claude API
func (c *ClaudeClient) makeRequest(ctx context.Context, req claudeRequest) (*claudeResponse, error) {
	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.config.BaseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "review-agent/1.0")

	// Make request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle HTTP errors
	if httpResp.StatusCode != http.StatusOK {
		return nil, c.handleHTTPError(httpResp.StatusCode, respBody)
	}

	// Parse successful response
	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &claudeResp, nil
}

// handleHTTPError processes HTTP error responses
func (c *ClaudeClient) handleHTTPError(statusCode int, body []byte) error {
	var errResp claudeErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", statusCode, string(body))
	}

	switch statusCode {
	case 400:
		return fmt.Errorf("bad request: %s", errResp.Error.Message)
	case 401:
		return fmt.Errorf("authentication failed: %s", errResp.Error.Message)
	case 403:
		return fmt.Errorf("forbidden: %s", errResp.Error.Message)
	case 429:
		return fmt.Errorf("rate limit exceeded: %s", errResp.Error.Message)
	case 500, 502, 503:
		return fmt.Errorf("server error: %s", errResp.Error.Message)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, errResp.Error.Message)
	}
}

// shouldRetry determines if an error is retryable
func (c *ClaudeClient) shouldRetry(err error) bool {
	errStr := err.Error()
	// Retry on rate limits and server errors
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection")
}

// parseClaudeResponse extracts review comments from Claude's response
func (c *ClaudeClient) parseClaudeResponse(resp *claudeResponse) ([]ReviewComment, string, error) {
	if len(resp.Content) == 0 {
		return nil, "", fmt.Errorf("empty response content")
	}

	var fullText strings.Builder
	for _, content := range resp.Content {
		if content.Type == "text" {
			fullText.WriteString(content.Text)
		}
	}

	responseText := fullText.String()
	if responseText == "" {
		return nil, "", fmt.Errorf("no text content in response")
	}

	// Parse the response text to extract structured review comments
	comments, summary := c.parseReviewText(responseText)

	return comments, summary, nil
}

// parseReviewText parses the Claude response text into structured review comments
func (c *ClaudeClient) parseReviewText(text string) ([]ReviewComment, string) {
	var comments []ReviewComment
	var summary string

	// This is a simplified parser - in a production system, you'd want more sophisticated parsing
	// Look for structured patterns in the response
	lines := strings.Split(text, "\n")

	var currentComment *ReviewComment
	var summaryLines []string
	inSummary := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for summary section
		if strings.Contains(strings.ToLower(line), "summary") || strings.Contains(strings.ToLower(line), "overview") {
			inSummary = true
			continue
		}

		if inSummary {
			summaryLines = append(summaryLines, line)
			continue
		}

		// Look for file references (simplified pattern)
		if strings.HasPrefix(line, "File:") || strings.Contains(line, ".go") || strings.Contains(line, ".js") || strings.Contains(line, ".py") {
			// If we have a previous comment, save it
			if currentComment != nil {
				comments = append(comments, *currentComment)
			}

			// Start new comment
			filename := c.extractFilename(line)
			if filename != "" {
				currentComment = &ReviewComment{
					Filename: filename,
					Type:     CommentTypeSuggestion,
					Severity: SeverityMinor,
				}
			}
			continue
		}

		// If we have a current comment, accumulate content
		if currentComment != nil {
			if currentComment.Comment == "" {
				currentComment.Comment = line
			} else {
				currentComment.Comment += " " + line
			}
		}
	}

	// Add the last comment
	if currentComment != nil {
		comments = append(comments, *currentComment)
	}

	if len(summaryLines) > 0 {
		summary = strings.Join(summaryLines, " ")
	}

	return comments, summary
}

// extractFilename extracts filename from a line that references a file
func (c *ClaudeClient) extractFilename(line string) string {
	// Simple filename extraction - can be enhanced
	line = strings.TrimPrefix(line, "File:")
	line = strings.TrimSpace(line)

	// Look for common file extensions
	extensions := []string{".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".h", ".rb", ".php", ".cs"}
	for _, ext := range extensions {
		if strings.Contains(line, ext) {
			// Extract the filename
			words := strings.Fields(line)
			for _, word := range words {
				if strings.Contains(word, ext) {
					return strings.Trim(word, "`:,")
				}
			}
		}
	}

	return ""
}

// Utility functions

func validateClaudeConfig(config ClaudeConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if config.MaxTokens <= 0 {
		return fmt.Errorf("max tokens must be positive")
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	return nil
}

// chunkRequestIfNeeded splits large requests into smaller chunks if needed
func (c *ClaudeClient) chunkRequestIfNeeded(systemPrompt, userPrompt string, request *ReviewRequest) []string {
	// Estimate token count (rough approximation: 1 token ≈ 4 characters)
	totalLength := len(systemPrompt) + len(userPrompt)
	estimatedTokens := totalLength / 4

	// Reserve tokens for response (use 80% of max tokens for input)
	maxInputTokens := int(float64(c.config.MaxTokens) * 0.8)

	if estimatedTokens <= maxInputTokens {
		return []string{userPrompt}
	}

	// If too large, we need to chunk the diff data
	// This is a simplified chunking strategy
	chunks := []string{}

	if request.ContextualDiff != nil && len(request.ContextualDiff.FilesWithContext) > 1 {
		// Split by files
		chunkSize := len(request.ContextualDiff.FilesWithContext) / 2
		if chunkSize == 0 {
			chunkSize = 1
		}

		for i := 0; i < len(request.ContextualDiff.FilesWithContext); i += chunkSize {
			end := i + chunkSize
			if end > len(request.ContextualDiff.FilesWithContext) {
				end = len(request.ContextualDiff.FilesWithContext)
			}

			// Create a chunk with subset of files
			chunkPrompt := c.generateUserPromptForFiles(request, request.ContextualDiff.FilesWithContext[i:end])
			chunks = append(chunks, chunkPrompt)
		}
	} else {
		// Fallback: just split the prompt in half
		midpoint := len(userPrompt) / 2
		chunks = []string{
			userPrompt[:midpoint],
			userPrompt[midpoint:],
		}
	}

	return chunks
}

// Prompt generation functions

// generateSystemPrompt creates the system prompt based on review type
func (c *ClaudeClient) generateSystemPrompt(reviewType ReviewType) string {
	basePrompt := `You are an expert code reviewer assistant. Your task is to review code changes in a pull request and provide helpful, constructive feedback.

Guidelines:
- Focus on code quality, security, performance, and maintainability
- Provide specific, actionable feedback
- Be constructive and helpful in your tone
- Point out issues
- Do not point out good practices
- When suggesting changes, explain the reasoning
- Use clear, professional language
- Format your response with clear sections for different files

Response Format:
For each file with issues or suggestions, use this format:

File: [filename]
[Your detailed review comments for this file]

Summary:
[Overall summary of the code changes and key recommendations]
`

	switch reviewType {
	case ReviewTypeSecurity:
		return basePrompt + `

Special Focus: SECURITY REVIEW
- Look for potential security vulnerabilities (SQL injection, XSS, authentication bypass, etc.)
- Check for proper input validation and sanitization
- Verify secure handling of sensitive data
- Review authentication and authorization logic
- Check for hardcoded secrets or credentials
- Assess cryptographic implementations`

	case ReviewTypePerformance:
		return basePrompt + `

Special Focus: PERFORMANCE REVIEW
- Identify potential performance bottlenecks
- Look for inefficient algorithms or data structures
- Check for unnecessary database queries or API calls
- Review memory usage and potential leaks
- Assess concurrent programming patterns
- Identify optimization opportunities`

	case ReviewTypeStyle:
		return basePrompt + `

Special Focus: CODE STYLE AND STANDARDS
- Check adherence to coding standards and conventions
- Review code formatting and consistency
- Assess naming conventions for variables, functions, and classes
- Check code organization and structure
- Review documentation and comments
- Verify proper error handling patterns`

	case ReviewTypeBugs:
		return basePrompt + `

Special Focus: BUG DETECTION
- Look for potential bugs and logic errors
- Check for edge cases and error conditions
- Review null pointer and boundary condition handling
- Assess error handling and recovery
- Look for race conditions in concurrent code
- Check for proper resource cleanup`

	case ReviewTypeTests:
		return basePrompt + `

Special Focus: TEST QUALITY
- Review test coverage and completeness
- Check test case quality and edge case coverage
- Assess test maintainability and clarity
- Review mock usage and test isolation
- Check for proper assertions and test data
- Evaluate integration and unit test balance`

	default: // ReviewTypeGeneral
		return basePrompt + `

Focus: GENERAL CODE REVIEW
- Overall code quality and maintainability
- Logic correctness and clarity
- Proper error handling
- Code organization and structure
- Performance considerations
- Security best practices`
	}
}

// generateUserPrompt creates the main user prompt for the review request
func (c *ClaudeClient) generateUserPrompt(request *ReviewRequest) string {
	var prompt strings.Builder

	prompt.WriteString("Please review the following pull request:\n\n")

	// Add PR information
	prompt.WriteString(fmt.Sprintf("**Pull Request #%d: %s**\n", request.PullRequestInfo.Number, request.PullRequestInfo.Title))
	prompt.WriteString(fmt.Sprintf("Author: %s\n", request.PullRequestInfo.Author))
	prompt.WriteString(fmt.Sprintf("Base branch: %s → Head branch: %s\n\n", request.PullRequestInfo.BaseBranch, request.PullRequestInfo.HeadBranch))

	if request.PullRequestInfo.Description != "" {
		prompt.WriteString("**Description:**\n")
		prompt.WriteString(request.PullRequestInfo.Description)
		prompt.WriteString("\n\n")
	}

	// Add custom instructions if provided
	if request.Instructions != "" {
		prompt.WriteString("**Special Instructions:**\n")
		prompt.WriteString(request.Instructions)
		prompt.WriteString("\n\n")
	}

	// Add diff information
	if request.ContextualDiff != nil {
		prompt.WriteString("**Code Changes:**\n\n")

		if len(request.ContextualDiff.FilesWithContext) > 0 {
			return c.generateUserPromptForFiles(request, request.ContextualDiff.FilesWithContext)
		}
	}

	// Fallback to raw diff if contextual diff is not available
	if request.DiffResult != nil {
		prompt.WriteString("**Raw Diff:**\n")
		prompt.WriteString("```diff\n")
		prompt.WriteString(request.DiffResult.RawDiff)
		prompt.WriteString("\n```\n\n")
	}

	prompt.WriteString("Please provide your review focusing on the requested review type and following the guidelines above.")

	return prompt.String()
}

// generateUserPromptForFiles creates a prompt for specific files (used for chunking)
func (c *ClaudeClient) generateUserPromptForFiles(request *ReviewRequest, files []analyzer.FileWithContext) string {
	var prompt strings.Builder

	prompt.WriteString("Please review the following pull request:\n\n")

	// Add PR information
	prompt.WriteString(fmt.Sprintf("**Pull Request #%d: %s**\n", request.PullRequestInfo.Number, request.PullRequestInfo.Title))
	prompt.WriteString(fmt.Sprintf("Author: %s\n", request.PullRequestInfo.Author))
	prompt.WriteString(fmt.Sprintf("Base branch: %s → Head branch: %s\n\n", request.PullRequestInfo.BaseBranch, request.PullRequestInfo.HeadBranch))

	if request.Instructions != "" {
		prompt.WriteString("**Special Instructions:**\n")
		prompt.WriteString(request.Instructions)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("**Code Changes:**\n\n")

	for _, file := range files {
		prompt.WriteString(fmt.Sprintf("### File: %s\n", file.Filename))
		prompt.WriteString(fmt.Sprintf("Status: %s\n", file.Status))
		if file.Language != "" {
			prompt.WriteString(fmt.Sprintf("Language: %s\n", file.Language))
		}
		prompt.WriteString("\n")

		// Show context blocks if available
		if len(file.ContextBlocks) > 0 {
			for i, block := range file.ContextBlocks {
				prompt.WriteString(fmt.Sprintf("**Change Block %d:**\n", i+1))
				prompt.WriteString(fmt.Sprintf("Type: %s\n", block.ChangeType))
				if block.Description != "" {
					prompt.WriteString(fmt.Sprintf("Description: %s\n", block.Description))
				}
				prompt.WriteString(fmt.Sprintf("Lines: %d-%d\n\n", block.StartLine, block.EndLine))

				prompt.WriteString("```diff\n")
				for _, line := range block.Lines {
					var prefix string
					switch line.Type {
					case "added":
						prefix = "+"
					case "removed":
						prefix = "-"
					default:
						prefix = " "
					}
					prompt.WriteString(fmt.Sprintf("%s%s\n", prefix, line.Content))
				}
				prompt.WriteString("```\n\n")
			}
		} else if len(file.Hunks) > 0 {
			// Fallback to showing hunks directly
			for i, hunk := range file.Hunks {
				prompt.WriteString(fmt.Sprintf("**Hunk %d:**\n", i+1))
				prompt.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount))

				prompt.WriteString("```diff\n")
				for _, line := range hunk.Lines {
					var prefix string
					switch line.Type {
					case "added":
						prefix = "+"
					case "removed":
						prefix = "-"
					default:
						prefix = " "
					}
					prompt.WriteString(fmt.Sprintf("%s%s\n", prefix, line.Content))
				}
				prompt.WriteString("```\n\n")
			}
		}

		prompt.WriteString("---\n\n")
	}

	prompt.WriteString("Please provide your review focusing on the requested review type and following the guidelines above. ")
	prompt.WriteString("For each file, identify specific issues, suggestions, or positive aspects of the code changes.")

	return prompt.String()
}
