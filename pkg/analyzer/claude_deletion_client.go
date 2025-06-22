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

	"github.com/GDSources/claude-code-review-agent/pkg/llm/claude"
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

// ClaudeAnalyzerConfig contains Claude-specific configuration for deletion analysis
type ClaudeAnalyzerConfig struct {
	APIKey      string                `json:"api_key"`
	Model       string                `json:"model"`
	MaxTokens   int                   `json:"max_tokens"`
	Temperature float64               `json:"temperature"`
	BaseURL     string                `json:"base_url"`
	Timeout     int                   `json:"timeout_seconds"`
	Limits      claude.ResourceLimits `json:"resource_limits"`
}

// Default configurations for deletion analysis
const (
	DefaultDeletionModel       = "claude-sonnet-4-20250514"
	DefaultDeletionMaxTokens   = 8000 // Higher limit for deletion analysis
	DefaultDeletionTemperature = 0.1  // Low temperature for consistent analysis
	DefaultDeletionBaseURL     = "https://api.anthropic.com"
	DefaultDeletionTimeout     = 180 // 3 minutes for complex analysis
)

// Claude API request/response type aliases for deletion analysis
type claudeRequest = claude.BaseRequest
type claudeMessage = claude.Message
type claudeResponse = claude.BaseResponse

// NewClaudeDeletionClient creates a new Claude client for deletion analysis
func NewClaudeDeletionClient(config ClaudeAnalyzerConfig) (*ClaudeDeletionClient, error) {
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
	// Set default resource limits if not provided
	if config.Limits.MaxCodebaseSize == 0 {
		config.Limits = claude.DefaultResourceLimits
	}

	// Validate configuration
	if err := claude.ValidateAPIKey(config.APIKey); err != nil {
		return nil, fmt.Errorf("invalid API key: %w", err)
	}
	if config.MaxTokens <= 0 {
		return nil, fmt.Errorf("max tokens must be positive")
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return nil, fmt.Errorf("temperature must be between 0 and 2")
	}
	if config.Timeout < 10 || config.Timeout > 600 {
		return nil, fmt.Errorf("timeout must be between 10 and 600 seconds")
	}
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}
	// Validate URL format
	if !strings.HasPrefix(config.BaseURL, "http://") && !strings.HasPrefix(config.BaseURL, "https://") {
		return nil, fmt.Errorf("base URL must start with http:// or https://")
	}
	// Validate resource limits
	if config.Limits.MaxCodebaseSize <= 0 || config.Limits.MaxFileCount <= 0 {
		return nil, fmt.Errorf("resource limits must be positive")
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

// String implements the Stringer interface to prevent accidental API key exposure
func (c *ClaudeDeletionClient) String() string {
	return fmt.Sprintf("ClaudeDeletionClient{model: %s, apiKey: %s, maxTokens: %d, temperature: %.2f, baseURL: %s}",
		c.model, claude.MaskAPIKey(c.apiKey), c.maxTokens, c.temperature, c.baseURL)
}

// AnalyzeDeletions implements the LLMClient interface for deletion analysis
func (c *ClaudeDeletionClient) AnalyzeDeletions(ctx context.Context, aiContext *AIAnalysisContext) (*DeletionAnalysisResult, error) {
	// Validate input parameters with detailed error context
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil - ensure proper context is provided")
	}
	if aiContext == nil {
		return nil, fmt.Errorf("AI analysis context cannot be nil - ensure analysis context is properly initialized")
	}
	if aiContext.CodebaseContext == "" {
		return nil, fmt.Errorf("codebase context cannot be empty - ensure codebase has been properly flattened")
	}
	if aiContext.DeletionContext == "" {
		return nil, fmt.Errorf("deletion context cannot be empty - ensure deleted content has been extracted")
	}

	// Validate context sizes for memory safety
	if len(aiContext.CodebaseContext) > 1000000 { // 1MB limit
		return nil, fmt.Errorf("codebase context too large (%d bytes) - consider processing smaller chunks",
			len(aiContext.CodebaseContext))
	}

	// Combine all context into the user prompt
	userPrompt := c.buildUserPrompt(aiContext)

	// Create Claude API request
	claudeReq := claudeRequest{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		System:      aiContext.SystemPrompt,
		Messages: []claudeMessage{
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

	// Sanitize all input components
	userPrompt := claude.SanitizeInput(aiContext.UserPrompt)
	codebaseContext := claude.SanitizeInput(aiContext.CodebaseContext)
	deletionContext := claude.SanitizeInput(aiContext.DeletionContext)
	instructions := claude.SanitizeInput(aiContext.Instructions)

	prompt.WriteString(userPrompt)
	prompt.WriteString("\n\n")

	prompt.WriteString(codebaseContext)
	prompt.WriteString("\n\n")

	prompt.WriteString(deletionContext)
	prompt.WriteString("\n\n")

	prompt.WriteString(instructions)
	prompt.WriteString("\n\n")

	// Add expected format as example
	expectedFormatJSON, _ := json.MarshalIndent(aiContext.ExpectedFormat, "", "  ")
	prompt.WriteString("Expected JSON Response Format:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(string(expectedFormatJSON))
	prompt.WriteString("\n```\n\n")

	prompt.WriteString("Please provide your analysis in the exact JSON format above.")

	// Apply resource limits to the final prompt
	finalPrompt := prompt.String()
	if len(finalPrompt) > 500000 { // 500KB limit
		finalPrompt = claude.TruncateWithLimits(finalPrompt, claude.DefaultResourceLimits)
	}

	return finalPrompt
}

// makeRequestWithRetry makes HTTP request to Claude API with exponential backoff retry
func (c *ClaudeDeletionClient) makeRequestWithRetry(ctx context.Context, req claudeRequest) (*claudeResponse, error) {
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

	return nil, fmt.Errorf("all %d retry attempts failed, last error: %w - check API key, network connectivity, and Claude service status", maxRetries+1, lastErr)
}

// makeRequest makes a single HTTP request to Claude API
func (c *ClaudeDeletionClient) makeRequest(ctx context.Context, req claudeRequest) (*claudeResponse, error) {
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
	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &claudeResp, nil
}

// handleHTTPError processes HTTP error responses
func (c *ClaudeDeletionClient) handleHTTPError(statusCode int, body []byte) error {
	return claude.HandleHTTPError(statusCode, body)
}

// shouldRetry determines if an error is retryable
func (c *ClaudeDeletionClient) shouldRetry(err error) bool {
	return claude.ShouldRetry(err)
}

// parseClaudeResponse extracts deletion analysis result from Claude's JSON response
func (c *ClaudeDeletionClient) parseClaudeResponse(resp *claudeResponse) (*DeletionAnalysisResult, error) {
	responseText := claude.ExtractTextContent(resp)
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
