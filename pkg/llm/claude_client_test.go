package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
)

func TestNewClaudeClient(t *testing.T) {
	tests := []struct {
		name          string
		config        ClaudeConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid configuration",
			config: ClaudeConfig{
				APIKey:      "test-api-key",
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4000,
				Temperature: 0.1,
				BaseURL:     "https://api.anthropic.com",
				Timeout:     30,
			},
			expectError: false,
		},
		{
			name: "missing API key",
			config: ClaudeConfig{
				Model:       "claude-sonnet-4-20250514",
				MaxTokens:   4000,
				Temperature: 0.1,
			},
			expectError:   true,
			errorContains: "API key is required",
		},
		{
			name: "invalid max tokens",
			config: ClaudeConfig{
				APIKey:    "test-api-key",
				MaxTokens: -1,
			},
			expectError:   true,
			errorContains: "max tokens must be positive",
		},
		{
			name: "invalid temperature",
			config: ClaudeConfig{
				APIKey:      "test-api-key",
				MaxTokens:   4000,
				Temperature: 3.0,
			},
			expectError:   true,
			errorContains: "temperature must be between 0 and 2",
		},
		{
			name: "defaults applied",
			config: ClaudeConfig{
				APIKey: "test-api-key",
				// Don't set other values to test defaults
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClaudeClient(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if client == nil {
				t.Error("expected client to be created")
				return
			}

			// Check defaults were applied
			if client.config.Model == "" {
				t.Error("expected default model to be set")
			}
			if client.config.MaxTokens == 0 {
				t.Error("expected default max tokens to be set")
			}
		})
	}
}

func TestClaudeClient_ValidateConfiguration(t *testing.T) {
	client, err := NewClaudeClient(ClaudeConfig{
		APIKey: "test-key",
		// Defaults will be applied during creation
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.ValidateConfiguration()
	if err != nil {
		t.Errorf("expected configuration to be valid, got error: %v", err)
	}
}

func TestClaudeClient_GetModelInfo(t *testing.T) {
	config := ClaudeConfig{
		APIKey: "test-key",
		// Let defaults be applied
	}

	client, err := NewClaudeClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	info := client.GetModelInfo()

	if info.Name == "" {
		t.Error("expected model name to be set")
	}
	if info.MaxTokens == 0 {
		t.Error("expected max tokens to be set")
	}
	if info.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", info.Provider)
	}
}

func TestClaudeClient_ReviewCode(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Errorf("expected x-api-key 'test-api-key', got '%s'", r.Header.Get("x-api-key"))
		}

		// Mock Claude API response
		response := claudeResponse{
			Content: []claudeContent{
				{
					Type: "text",
					Text: "File: test.go\nThis is a test review comment.\n\nSummary:\nThe code looks good overall.",
				},
			},
			Model: "claude-sonnet-4-20250514",
			Usage: claudeUsage{
				InputTokens:  100,
				OutputTokens: 50,
			},
			ID:   "test-response-id",
			Type: "message",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with mock server URL
	config := ClaudeConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
		// Let other defaults be applied
	}

	client, err := NewClaudeClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create test request
	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number:     123,
			Title:      "Test PR",
			Author:     "testuser",
			BaseBranch: "main",
			HeadBranch: "feature-branch",
		},
		DiffResult: &github.DiffResult{
			RawDiff:    "diff --git a/test.go b/test.go\n+func test() {}",
			TotalFiles: 1,
		},
		ReviewType: ReviewTypeGeneral,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := client.ReviewCode(ctx, request)
	if err != nil {
		t.Fatalf("ReviewCode failed: %v", err)
	}

	if response == nil {
		t.Fatal("expected response but got nil")
	}

	if response.ModelUsed != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got '%s'", response.ModelUsed)
	}

	if response.TokensUsed.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", response.TokensUsed.InputTokens)
	}

	if response.TokensUsed.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", response.TokensUsed.OutputTokens)
	}

	if response.TokensUsed.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", response.TokensUsed.TotalTokens)
	}

	if len(response.Comments) == 0 {
		t.Error("expected at least one comment")
	}

	if response.Summary == "" {
		t.Error("expected summary to be populated")
	}
}

func TestClaudeClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:          "authentication error",
			statusCode:    401,
			responseBody:  `{"error": {"type": "authentication_error", "message": "Invalid API key"}}`,
			expectedError: "authentication failed",
		},
		{
			name:          "rate limit error",
			statusCode:    429,
			responseBody:  `{"error": {"type": "rate_limit_error", "message": "Rate limit exceeded"}}`,
			expectedError: "rate limit exceeded",
		},
		{
			name:          "server error",
			statusCode:    500,
			responseBody:  `{"error": {"type": "server_error", "message": "Internal server error"}}`,
			expectedError: "server error",
		},
		{
			name:          "bad request",
			statusCode:    400,
			responseBody:  `{"error": {"type": "invalid_request_error", "message": "Invalid request"}}`,
			expectedError: "bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			config := ClaudeConfig{
				APIKey:  "test-api-key",
				BaseURL: server.URL,
				// Let defaults be applied
			}

			client, err := NewClaudeClient(config)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			request := &ReviewRequest{
				PullRequestInfo: PullRequestInfo{Number: 1, Title: "Test"},
				ReviewType:      ReviewTypeGeneral,
			}

			ctx := context.Background()
			_, err = client.ReviewCode(ctx, request)

			if err == nil {
				t.Error("expected error but got none")
			} else if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("expected error to contain '%s', got: %v", tt.expectedError, err)
			}
		})
	}
}

func TestClaudeClient_ContextCancellation(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content": [], "model": "test", "usage": {"input_tokens": 0, "output_tokens": 0}}`))
	}))
	defer server.Close()

	config := ClaudeConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
		// Let defaults be applied
	}

	client, err := NewClaudeClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{Number: 1, Title: "Test"},
		ReviewType:      ReviewTypeGeneral,
	}

	// Cancel context after 100ms
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.ReviewCode(ctx, request)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestClaudeClient_TokenChunking(t *testing.T) {
	// Create mock server that tracks requests
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		response := claudeResponse{
			Content: []claudeContent{
				{
					Type: "text",
					Text: "File: test.go\nChunk review comment.",
				},
			},
			Model: "claude-sonnet-4-20250514",
			Usage: claudeUsage{
				InputTokens:  50,
				OutputTokens: 25,
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := ClaudeConfig{
		APIKey:    "test-api-key",
		BaseURL:   server.URL,
		MaxTokens: 100, // Very small to trigger chunking
		// Let other defaults be applied
	}

	client, err := NewClaudeClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create large request to trigger chunking
	files := make([]analyzer.FileWithContext, 3)
	for i := range files {
		files[i] = analyzer.FileWithContext{
			FileDiff: analyzer.FileDiff{
				Filename: "test.go",
				Language: "go",
			},
			ContextBlocks: []analyzer.ContextBlock{
				{
					StartLine:   1,
					EndLine:     10,
					ChangeType:  "addition",
					Description: strings.Repeat("This is a long description ", 50), // Make it long
				},
			},
		}
	}

	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number: 1,
			Title:  "Large PR",
		},
		ContextualDiff: &analyzer.ContextualDiff{
			FilesWithContext: files,
		},
		ReviewType: ReviewTypeGeneral,
	}

	ctx := context.Background()
	response, err := client.ReviewCode(ctx, request)
	if err != nil {
		t.Fatalf("ReviewCode failed: %v", err)
	}

	if requestCount < 2 {
		t.Errorf("expected chunking to trigger multiple requests, got %d", requestCount)
	}

	if response == nil {
		t.Fatal("expected response")
	}

	// Check that responses were aggregated
	if response.TokensUsed.TotalTokens != requestCount*75 {
		t.Errorf("expected aggregated token count, got %d", response.TokensUsed.TotalTokens)
	}
}

func TestGenerateSystemPrompt(t *testing.T) {
	client, err := NewClaudeClient(ClaudeConfig{
		APIKey: "test",
		// Let defaults be applied
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tests := []struct {
		reviewType      ReviewType
		expectedContent string
	}{
		{
			reviewType:      ReviewTypeSecurity,
			expectedContent: "SECURITY REVIEW",
		},
		{
			reviewType:      ReviewTypePerformance,
			expectedContent: "PERFORMANCE REVIEW",
		},
		{
			reviewType:      ReviewTypeStyle,
			expectedContent: "CODE STYLE AND STANDARDS",
		},
		{
			reviewType:      ReviewTypeBugs,
			expectedContent: "BUG DETECTION",
		},
		{
			reviewType:      ReviewTypeTests,
			expectedContent: "TEST QUALITY",
		},
		{
			reviewType:      ReviewTypeGeneral,
			expectedContent: "GENERAL CODE REVIEW",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.reviewType), func(t *testing.T) {
			prompt := client.generateSystemPrompt(tt.reviewType)

			if prompt == "" {
				t.Error("expected non-empty prompt")
			}

			if !strings.Contains(prompt, tt.expectedContent) {
				t.Errorf("expected prompt to contain '%s'", tt.expectedContent)
			}

			// Check that base guidelines are included
			if !strings.Contains(prompt, "Focus ONLY on actual problems") {
				t.Error("expected prompt to contain base guidelines")
			}
		})
	}
}

func TestGenerateUserPrompt(t *testing.T) {
	client, err := NewClaudeClient(ClaudeConfig{
		APIKey: "test",
		// Let defaults be applied
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number:      123,
			Title:       "Test PR",
			Author:      "testuser",
			Description: "This is a test PR",
			BaseBranch:  "main",
			HeadBranch:  "feature",
		},
		Instructions: "Please focus on performance",
		DiffResult: &github.DiffResult{
			RawDiff: "diff --git a/test.go b/test.go\n+func test() {}",
		},
		ReviewType: ReviewTypeGeneral,
	}

	prompt := client.generateUserPrompt(request)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Check that PR info is included
	if !strings.Contains(prompt, "Pull Request #123") {
		t.Error("expected prompt to contain PR number")
	}
	if !strings.Contains(prompt, "Test PR") {
		t.Error("expected prompt to contain PR title")
	}
	if !strings.Contains(prompt, "testuser") {
		t.Error("expected prompt to contain author")
	}
	if !strings.Contains(prompt, "This is a test PR") {
		t.Error("expected prompt to contain description")
	}
	if !strings.Contains(prompt, "Please focus on performance") {
		t.Error("expected prompt to contain instructions")
	}
}

func TestParseReviewText(t *testing.T) {
	client, err := NewClaudeClient(ClaudeConfig{
		APIKey: "test",
		// Let defaults be applied
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	text := `File: test.go
This is a review comment for the test file.
There are some issues to address.

File: main.go  
This file looks good but could be improved.

Summary:
Overall the code is well structured but needs some improvements.
`

	comments, summary := client.parseReviewText(text)

	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}

	if comments[0].Filename != "test.go" {
		t.Errorf("expected first comment filename 'test.go', got '%s'", comments[0].Filename)
	}

	if comments[1].Filename != "main.go" {
		t.Errorf("expected second comment filename 'main.go', got '%s'", comments[1].Filename)
	}

	if !strings.Contains(summary, "Overall the code is well structured") {
		t.Errorf("expected summary to be extracted, got '%s'", summary)
	}
}

func TestParseJSONResponse(t *testing.T) {
	client := &ClaudeClient{}

	tests := []struct {
		name             string
		jsonResponse     string
		expectedComments int
		expectedSummary  string
		expectError      bool
	}{
		{
			name: "valid JSON response with line numbers",
			jsonResponse: `{
				"comments": [
					{
						"filename": "main.go",
						"line_number": 15,
						"comment": "This function should have error handling",
						"severity": "major",
						"type": "issue",
						"category": "bugs"
					},
					{
						"filename": "utils.go",
						"line_number": 32,
						"comment": "Consider using a more descriptive variable name",
						"severity": "minor",
						"type": "suggestion",
						"category": "style"
					}
				],
				"summary": "Overall code quality is good with minor improvements needed"
			}`,
			expectedComments: 2,
			expectedSummary:  "Overall code quality is good with minor improvements needed",
			expectError:      false,
		},
		{
			name:             "JSON with markdown code blocks",
			jsonResponse:     "```json\n{\n  \"comments\": [\n    {\n      \"filename\": \"test.go\",\n      \"line_number\": 42,\n      \"comment\": \"Test comment\",\n      \"severity\": \"info\",\n      \"type\": \"suggestion\"\n    }\n  ],\n  \"summary\": \"Test summary\"\n}\n```",
			expectedComments: 1,
			expectedSummary:  "Test summary",
			expectError:      false,
		},
		{
			name: "invalid line numbers filtered out",
			jsonResponse: `{
				"comments": [
					{
						"filename": "main.go",
						"line_number": 0,
						"comment": "This should be filtered out",
						"severity": "info",
						"type": "suggestion"
					},
					{
						"filename": "main.go",
						"line_number": 25,
						"comment": "This should be included",
						"severity": "minor",
						"type": "issue"
					}
				],
				"summary": "Test with invalid line numbers"
			}`,
			expectedComments: 1, // Only one valid comment
			expectedSummary:  "Test with invalid line numbers",
			expectError:      false,
		},
		{
			name: "all comments invalid",
			jsonResponse: `{
				"comments": [
					{
						"filename": "",
						"line_number": 10,
						"comment": "Missing filename",
						"severity": "info",
						"type": "suggestion"
					},
					{
						"filename": "main.go",
						"line_number": 0,
						"comment": "Invalid line number",
						"severity": "info",
						"type": "suggestion"
					}
				],
				"summary": "All invalid comments"
			}`,
			expectedComments: 0,
			expectedSummary:  "",
			expectError:      true, // Should error when all comments are invalid
		},
		{
			name: "invalid JSON",
			jsonResponse: `{
				"comments": [
					{
						"filename": "main.go"
						"line_number": 15,
					}
				]
			}`, // Missing comma - invalid JSON
			expectedComments: 0,
			expectedSummary:  "",
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, summary, err := client.parseJSONResponse(tt.jsonResponse)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(comments) != tt.expectedComments {
				t.Errorf("expected %d comments, got %d", tt.expectedComments, len(comments))
			}

			if summary != tt.expectedSummary {
				t.Errorf("expected summary '%s', got '%s'", tt.expectedSummary, summary)
			}

			// Validate that all returned comments have valid line numbers
			for i, comment := range comments {
				if comment.LineNumber <= 0 {
					t.Errorf("comment %d has invalid line number: %d", i, comment.LineNumber)
				}
				if comment.Filename == "" {
					t.Errorf("comment %d has empty filename", i)
				}
				if comment.Comment == "" {
					t.Errorf("comment %d has empty comment", i)
				}
			}
		})
	}
}

func TestExtractFilename(t *testing.T) {
	client, err := NewClaudeClient(ClaudeConfig{
		APIKey: "test",
		// Let defaults be applied
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"File: test.go", "test.go"},
		{"Looking at main.py:", "main.py"},
		{"In the file src/utils.js there are issues", "src/utils.js"},
		{"No file extension here", ""},
		{"Multiple files: test.go and main.py", "test.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := client.extractFilename(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
