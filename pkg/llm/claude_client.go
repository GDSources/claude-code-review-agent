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

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/llm/claude"
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

// claudeReviewResponse represents the expected JSON response format from Claude
type claudeReviewResponse struct {
	Comments []claudeReviewComment `json:"comments"`
	Summary  string                `json:"summary"`
}

// claudeReviewComment represents a single review comment in Claude's JSON response
type claudeReviewComment struct {
	Filename   string `json:"filename"`
	LineNumber int    `json:"line_number"`
	Comment    string `json:"comment"`
	Severity   string `json:"severity"`
	Type       string `json:"type"`
	Category   string `json:"category,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
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

// String implements the Stringer interface to prevent accidental API key exposure
func (c *ClaudeClient) String() string {
	return fmt.Sprintf("ClaudeClient{model: %s, apiKey: %s, maxTokens: %d, temperature: %.2f, baseURL: %s}",
		c.config.Model, claude.MaskAPIKey(c.config.APIKey), c.config.MaxTokens, c.config.Temperature, c.config.BaseURL)
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

// parseClaudeResponse extracts review comments from Claude's JSON response
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

	// Try to parse as JSON first
	comments, summary, err := c.parseJSONResponse(responseText)
	if err != nil {
		// Fallback to text parsing for backward compatibility
		comments, summary := c.parseReviewText(responseText)
		return comments, summary, nil
	}

	return comments, summary, nil
}

// parseJSONResponse parses Claude's JSON response into structured review comments
func (c *ClaudeClient) parseJSONResponse(responseText string) ([]ReviewComment, string, error) {
	// Clean up the response text - sometimes Claude wraps JSON in markdown code blocks
	cleanedText := strings.TrimSpace(responseText)

	// Remove markdown code blocks if present
	if strings.HasPrefix(cleanedText, "```json") {
		cleanedText = strings.TrimPrefix(cleanedText, "```json")
		cleanedText = strings.TrimSuffix(cleanedText, "```")
		cleanedText = strings.TrimSpace(cleanedText)
	} else if strings.HasPrefix(cleanedText, "```") {
		cleanedText = strings.TrimPrefix(cleanedText, "```")
		cleanedText = strings.TrimSuffix(cleanedText, "```")
		cleanedText = strings.TrimSpace(cleanedText)
	}

	// Parse the JSON response
	var claudeResp claudeReviewResponse
	if err := json.Unmarshal([]byte(cleanedText), &claudeResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Convert Claude's response to our internal format
	var comments []ReviewComment
	var skippedCount int

	for _, claudeComment := range claudeResp.Comments {
		// Validate required fields
		if claudeComment.Filename == "" {
			skippedCount++
			continue
		}

		if claudeComment.Comment == "" {
			skippedCount++
			continue
		}

		// Validate line number - this is critical!
		if claudeComment.LineNumber <= 0 {
			skippedCount++
			continue
		}

		// Map severity with validation
		severity := c.mapSeverity(claudeComment.Severity)

		// Map comment type with validation
		commentType := c.mapCommentType(claudeComment.Type)

		comment := ReviewComment{
			Filename:   claudeComment.Filename,
			LineNumber: claudeComment.LineNumber,
			Comment:    claudeComment.Comment,
			Severity:   severity,
			Type:       commentType,
			Category:   claudeComment.Category,
			Suggestion: claudeComment.Suggestion,
		}

		comments = append(comments, comment)
	}

	// If we skipped comments, this indicates potential issues with LLM following instructions
	if skippedCount > 0 && len(comments) == 0 {
		return nil, "", fmt.Errorf("all %d comments were invalid (missing filename, comment, or valid line number)", skippedCount)
	}

	return comments, claudeResp.Summary, nil
}

// mapSeverity maps Claude's severity strings to our Severity enum
func (c *ClaudeClient) mapSeverity(severity string) Severity {
	switch strings.ToLower(severity) {
	case "info", "informational":
		return SeverityInfo
	case "minor", "low":
		return SeverityMinor
	case "major", "high":
		return SeverityMajor
	case "critical", "blocker":
		return SeverityCritical
	default:
		return SeverityMinor // Default fallback
	}
}

// mapCommentType maps Claude's type strings to our CommentType enum
func (c *ClaudeClient) mapCommentType(commentType string) CommentType {
	switch strings.ToLower(commentType) {
	case "suggestion", "improvement":
		return CommentTypeSuggestion
	case "issue", "problem", "bug":
		return CommentTypeIssue
	case "praise", "positive":
		return CommentTypePraise
	case "question", "clarification":
		return CommentTypeQuestion
	case "nitpick", "style", "minor":
		return CommentTypeNitpick
	default:
		return CommentTypeSuggestion // Default fallback
	}
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

	// Validate model is supported
	if !isValidClaudeModel(config.Model) {
		return fmt.Errorf("unsupported model '%s'. Available models: %v", config.Model, AvailableClaudeModels)
	}

	return nil
}

// isValidClaudeModel checks if the provided model is in the list of supported models
func isValidClaudeModel(model string) bool {
	for _, validModel := range AvailableClaudeModels {
		if model == validModel {
			return true
		}
	}
	return false
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
- Focus ONLY on actual problems that could cause bugs, security issues, or performance problems
- Provide specific, actionable feedback with clear impact explanation
- Be constructive and helpful in your tone
- Only comment on code that has demonstrable issues or risks
- When suggesting changes, explain the specific negative consequence if not fixed

CRITICAL DO NOT RULES:
- DO NOT suggest adding comments, documentation, or docstrings unless missing docs cause actual confusion about complex logic
- DO NOT explain what code does - only comment on actual problems
- DO NOT suggest general improvements without demonstrable negative impact
- DO NOT comment on variable naming unless it causes genuine confusion or bugs
- DO NOT comment on formatting/style unless it significantly affects readability or could cause errors
- DO NOT suggest "best practices" without explaining the specific risk of current approach
- DO NOT create comments for preference or opinion - only for concrete issues

QUALITY THRESHOLD:
Every comment must meet this test: "If this issue is not addressed, what specific problem could occur?"
If you cannot identify a concrete negative consequence, do not comment.

CRITICAL: You MUST respond with valid JSON in the following format:

{
  "comments": [
    {
      "filename": "path/to/file.ext",
      "line_number": 42,
      "comment": "Detailed explanation of the issue or suggestion",
      "severity": "minor|major|critical",
      "type": "issue|suggestion|nitpick",
      "category": "security|performance|style|bugs|maintainability",
      "suggestion": "Optional: specific code suggestion to fix the issue"
    }
  ],
  "summary": "Overall summary of the code changes and key recommendations"
}

Line Number Instructions:
- Extract line numbers from diff hunks like "@@ -10,5 +15,7 @@" (this means new file starts at line 15)
- For additions (+ lines), use the NEW file line number (right side of diff)
- For modifications, use the line number where the NEW code appears
- For deletions, use the line number of the surrounding context
- Count line numbers incrementally from the hunk start position
- NEVER use line_number: 0 - always provide a specific line number > 0
- If you cannot determine a specific line, use the closest reasonable line number

Example diff analysis:
` + "`" + `diff
@@ -8,4 +8,6 @@ function example() {
   const a = 1;        // line 9 (context)
-  const b = 2;        // line 10 (old, being removed)  
+  const b = 3;        // line 10 (new replacement)
+  const c = 4;        // line 11 (new addition)
   return a + b;       // line 12 (context)
` + "`" + `

For the change "const b = 3", use line_number: 10
For the addition "const c = 4", use line_number: 11

Field Specifications:
- filename: Exact file path as shown in the diff
- line_number: Integer > 0, never use 0
- comment: Clear, specific feedback explaining the problem and its impact (50-200 characters recommended)
- severity: Use strictly: "minor" (could cause minor bugs/issues), "major" (likely to cause significant problems), "critical" (will definitely cause failures/security issues)
- type: "issue" (concrete problem that needs fixing), "suggestion" (improvement with clear benefit), avoid "nitpick" unless truly critical
- category: General category of the feedback
- suggestion: Optional specific code to fix the issue

IMPACT REQUIREMENT:
Every comment must explain WHY it matters. Use this format:
"[Problem description] - this could cause [specific negative consequence]"

Examples of HIGH-QUALITY comments:
 "Missing null check before dereferencing user.email - this will cause NullPointerException when user is not authenticated"
 "Race condition: shared counter accessed without synchronization - this could cause data corruption in concurrent requests"
 "SQL injection vulnerability: user input concatenated directly into query - this allows attackers to execute arbitrary SQL"
 "Memory leak: file handle not closed in error path - this will exhaust file descriptors under high load"

Examples of LOW-QUALITY comments to AVOID:
 "Consider adding documentation to explain this function"
 "This variable name could be more descriptive"
 "You might want to use a constant here"
 "This function is doing a lot of things"
 "Consider extracting this into a separate method"
`

	switch reviewType {
	case ReviewTypeSecurity:
		return basePrompt + `

Special Focus: SECURITY REVIEW
ONLY comment on actual security vulnerabilities that could be exploited:
- SQL injection, XSS, CSRF vulnerabilities (not general input validation suggestions)
- Authentication bypass or authorization flaws (not "add authentication" suggestions)
- Hardcoded secrets, passwords, or API keys in code
- Insecure cryptographic implementations (weak algorithms, poor key management)
- Data exposure through logging, error messages, or unprotected endpoints
- Path traversal, command injection, or code injection vulnerabilities

Focus on exploitable vulnerabilities, not security "best practices" without clear attack vectors.
Remember: Return JSON format with specific line numbers for each concrete security vulnerability found.`

	case ReviewTypePerformance:
		return basePrompt + `

Special Focus: PERFORMANCE REVIEW
ONLY comment on actual performance problems that will impact users:
- O(n²) or worse algorithms where O(n log n) alternatives exist
- Memory leaks (resources not freed, growing caches, event listeners not removed)
- Unnecessary database queries in loops (N+1 problems)
- Blocking operations on main threads
- Resource contention or deadlock potential in concurrent code
- Excessive object allocation in hot paths

Focus on measurable performance degradation, not theoretical optimizations.
Remember: Return JSON format with specific line numbers for each concrete performance issue found.`

	case ReviewTypeStyle:
		return basePrompt + `

Special Focus: CODE STYLE AND STANDARDS
ONLY comment on style issues that could cause bugs or significant confusion:
- Inconsistent error handling patterns that could hide failures
- Confusing variable names that make bugs likely (single letters in complex logic, misleading names)
- Missing error handling that could cause silent failures
- Code organization that violates team standards and causes maintainability issues

Avoid commenting on minor formatting, preference-based naming, or cosmetic issues.
Remember: Return JSON format with specific line numbers for each style issue that impacts correctness.`

	case ReviewTypeBugs:
		return basePrompt + `

Special Focus: BUG DETECTION
ONLY comment on actual bugs and logic errors that will cause runtime failures:
- Null pointer dereferences, array bounds violations, type errors
- Logic errors in conditionals or loops that produce wrong results
- Unhandled edge cases that will cause crashes or incorrect behavior
- Race conditions, deadlocks, or thread safety violations
- Resource leaks (memory, file handles, database connections not closed)
- Exception handling gaps that could cause silent failures

Focus on code that will definitely malfunction, not code that "might" have issues.
Remember: Return JSON format with specific line numbers for each concrete bug found.`

	case ReviewTypeTests:
		return basePrompt + `

Special Focus: TEST QUALITY
ONLY comment on test issues that could hide bugs or cause flaky tests:
- Missing assertions that could let bugs pass silently
- Tests that don't actually test the claimed functionality
- Flaky tests with race conditions or environment dependencies
- Tests that modify global state and affect other tests
- Missing negative test cases for error conditions that could hide real bugs

Avoid comments about test organization, naming conventions, or minor improvements.
Remember: Return JSON format with specific line numbers for each test reliability issue found.`

	default: // ReviewTypeGeneral
		return basePrompt + `

Focus: GENERAL CODE REVIEW
ONLY comment on issues that could cause runtime problems or significant maintainability issues:
- Logic errors that produce incorrect results
- Missing error handling that could cause crashes
- Obvious security vulnerabilities (SQL injection, XSS, etc.)
- Performance issues that will impact users (inefficient algorithms, memory leaks)
- Resource management problems (connections not closed, memory not freed)

Prioritize bugs and security issues over style preferences or minor improvements.
Remember: Return JSON format with specific line numbers for each concrete issue found.`
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

	prompt.WriteString("Please provide your review in the JSON format specified above. ")
	prompt.WriteString("Focus on the requested review type and include specific line numbers for each comment. ")
	prompt.WriteString("Remember: NEVER use line_number: 0, always provide specific line numbers > 0 based on the diff hunk positions.")

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

	prompt.WriteString("Please provide your review in the JSON format specified above. ")
	prompt.WriteString("Focus on the requested review type and include specific line numbers for each comment. ")
	prompt.WriteString("For each file, identify specific issues and suggestions with exact line numbers from the diff hunks. ")
	prompt.WriteString("Remember: NEVER use line_number: 0, always provide specific line numbers > 0 based on the diff hunk positions.")

	return prompt.String()
}
