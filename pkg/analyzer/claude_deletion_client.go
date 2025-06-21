package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClaudeDeletionClient implements LLMClient for deletion analysis using Claude
type ClaudeDeletionClient struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
	httpClient  *http.Client
}

// ClaudeDeletionConfig contains Claude-specific configuration for deletion analysis
type ClaudeDeletionConfig struct {
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	BaseURL     string  `json:"base_url"`
	Timeout     int     `json:"timeout_seconds"`
}

// Default configurations for deletion analysis
const (
	DefaultDeletionModel       = "claude-sonnet-4-20250514"
	DefaultDeletionMaxTokens   = 8000 // Higher limit for deletion analysis
	DefaultDeletionTemperature = 0.1  // Low temperature for consistent analysis
	DefaultDeletionBaseURL     = "https://api.anthropic.com"
	DefaultDeletionTimeout     = 180  // 3 minutes for complex analysis
)

// Claude API structures for deletion analysis
type claudeDeletionRequest struct {
	Model       string                    `json:"model"`
	MaxTokens   int                       `json:"max_tokens"`
	Messages    []claudeDeletionMessage   `json:"messages"`
	Temperature float64                   `json:"temperature,omitempty"`
	System      string                    `json:"system,omitempty"`
}

type claudeDeletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeDeletionResponse struct {
	Content []claudeDeletionContent `json:"content"`
	Model   string                  `json:"model"`
	Usage   claudeDeletionUsage     `json:"usage"`
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
}

type claudeDeletionContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeDeletionUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeDeletionError struct {
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type claudeDeletionErrorResponse struct {
	Error claudeDeletionError `json:"error"`
}

// NewClaudeDeletionClient creates a new Claude client for deletion analysis
func NewClaudeDeletionClient(config ClaudeDeletionConfig) (*ClaudeDeletionClient, error) {
	// Set defaults
	if config.Model == "" {
		config.Model = DefaultDeletionModel
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = DefaultDeletionMaxTokens
	}
	if config.Temperature == 0 {
		config.Temperature = DefaultDeletionTemperature
	}
	if config.BaseURL == "" {
		config.BaseURL = DefaultDeletionBaseURL
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultDeletionTimeout
	}

	// Validate configuration
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.MaxTokens <= 0 {
		return nil, fmt.Errorf("max tokens must be positive")
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return nil, fmt.Errorf("temperature must be between 0 and 2")
	}

	httpClient := &http.Client{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	return &ClaudeDeletionClient{
		apiKey:      config.APIKey,
		model:       config.Model,
		maxTokens:   config.MaxTokens,
		temperature: config.Temperature,
		baseURL:     config.BaseURL,
		httpClient:  httpClient,
	}, nil
}

// AnalyzeDeletions implements the LLMClient interface for deletion analysis
func (c *ClaudeDeletionClient) AnalyzeDeletions(ctx context.Context, aiContext *AIAnalysisContext) (*DeletionAnalysisResult, error) {
	// Combine all context into the user prompt
	userPrompt := c.buildUserPrompt(aiContext)

	// Create Claude API request
	claudeReq := claudeDeletionRequest{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		System:      aiContext.SystemPrompt,
		Messages: []claudeDeletionMessage{
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
	}

	// Make API request with retry logic
	claudeResp, err := c.makeRequestWithRetry(ctx, claudeReq)
	if err != nil {
		return nil, fmt.Errorf("Claude API request failed: %w", err)
	}

	// Parse response into deletion analysis result
	result, err := c.parseClaudeResponse(claudeResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %w", err)
	}

	return result, nil
}

// buildUserPrompt combines the AI context into a single user prompt
func (c *ClaudeDeletionClient) buildUserPrompt(aiContext *AIAnalysisContext) string {
	var prompt strings.Builder
	
	prompt.WriteString(aiContext.UserPrompt)
	prompt.WriteString("\n\n")
	
	prompt.WriteString(aiContext.CodebaseContext)
	prompt.WriteString("\n\n")
	
	prompt.WriteString(aiContext.DeletionContext)
	prompt.WriteString("\n\n")
	
	prompt.WriteString(aiContext.Instructions)
	prompt.WriteString("\n\n")
	
	// Add expected format as example
	expectedFormatJSON, _ := json.MarshalIndent(aiContext.ExpectedFormat, "", "  ")
	prompt.WriteString("Expected JSON Response Format:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(string(expectedFormatJSON))
	prompt.WriteString("\n```\n\n")
	
	prompt.WriteString("Please provide your analysis in the exact JSON format above.")
	
	return prompt.String()
}

// makeRequestWithRetry makes HTTP request to Claude API with exponential backoff retry
func (c *ClaudeDeletionClient) makeRequestWithRetry(ctx context.Context, req claudeDeletionRequest) (*claudeDeletionResponse, error) {
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
func (c *ClaudeDeletionClient) makeRequest(ctx context.Context, req claudeDeletionRequest) (*claudeDeletionResponse, error) {
	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "review-agent-deletion-analyzer/1.0")

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
	var claudeResp claudeDeletionResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &claudeResp, nil
}

// handleHTTPError processes HTTP error responses
func (c *ClaudeDeletionClient) handleHTTPError(statusCode int, body []byte) error {
	var errResp claudeDeletionErrorResponse
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
func (c *ClaudeDeletionClient) shouldRetry(err error) bool {
	errStr := err.Error()
	// Retry on rate limits and server errors
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection")
}

// parseClaudeResponse extracts deletion analysis result from Claude's JSON response
func (c *ClaudeDeletionClient) parseClaudeResponse(resp *claudeDeletionResponse) (*DeletionAnalysisResult, error) {
	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response content")
	}

	var fullText strings.Builder
	for _, content := range resp.Content {
		if content.Type == "text" {
			fullText.WriteString(content.Text)
		}
	}

	responseText := fullText.String()
	if responseText == "" {
		return nil, fmt.Errorf("no text content in response")
	}

	// Parse JSON response
	result, err := c.parseJSONResponse(responseText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return result, nil
}

// parseJSONResponse parses Claude's JSON response into structured deletion analysis result
func (c *ClaudeDeletionClient) parseJSONResponse(responseText string) (*DeletionAnalysisResult, error) {
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

	// Parse the JSON response directly into our DeletionAnalysisResult structure
	var result DeletionAnalysisResult
	if err := json.Unmarshal([]byte(cleanedText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Validate the result
	if err := c.validateDeletionAnalysisResult(&result); err != nil {
		return nil, fmt.Errorf("invalid response structure: %w", err)
	}

	return &result, nil
}

// validateDeletionAnalysisResult validates the parsed result
func (c *ClaudeDeletionClient) validateDeletionAnalysisResult(result *DeletionAnalysisResult) error {
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1, got %f", result.Confidence)
	}

	// Validate orphaned references
	for i, ref := range result.OrphanedReferences {
		if ref.DeletedEntity == "" {
			return fmt.Errorf("orphaned reference %d: missing deleted entity", i)
		}
		if ref.ReferencingFile == "" {
			return fmt.Errorf("orphaned reference %d: missing referencing file", i)
		}
		if len(ref.ReferencingLines) == 0 {
			return fmt.Errorf("orphaned reference %d: missing referencing lines", i)
		}
	}

	// Validate warnings
	for i, warning := range result.Warnings {
		if warning.Type == "" {
			return fmt.Errorf("warning %d: missing type", i)
		}
		if warning.Message == "" {
			return fmt.Errorf("warning %d: missing message", i)
		}
	}

	return nil
}