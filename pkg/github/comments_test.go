package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMakeRequestWithBody(t *testing.T) {
	expectedBody := map[string]string{"test": "data"}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		
		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		
		// Verify authorization header
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("expected Authorization header with Bearer token")
		}
		
		// Verify body content
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		
		if body["test"] != "data" {
			t.Errorf("expected body.test = 'data', got %s", body["test"])
		}
		
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	}))
	defer server.Close()
	
	client := &Client{
		token:      "test-token",
		baseURL:    server.URL,
		httpClient: &http.Client{},
	}
	
	ctx := context.Background()
	resp, err := client.makeRequestWithBody(ctx, "POST", "/test", expectedBody)
	if err != nil {
		t.Fatalf("makeRequestWithBody failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestCreatePullRequestComment_Success(t *testing.T) {
	expectedComment := CreatePullRequestCommentRequest{
		Body:     "This is a test comment",
		Path:     "main.go", 
		Line:     15,
		Side:     "RIGHT",
		CommitID: "abc123def456",
	}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify endpoint
		expectedPath := "/repos/testowner/testrepo/pulls/123/comments"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		
		// Verify method
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		
		// Verify request body
		var requestBody CreatePullRequestCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		
		if requestBody.Body != expectedComment.Body {
			t.Errorf("expected body '%s', got '%s'", expectedComment.Body, requestBody.Body)
		}
		
		if requestBody.Path != expectedComment.Path {
			t.Errorf("expected path '%s', got '%s'", expectedComment.Path, requestBody.Path)
		}
		
		// Return successful response
		response := PullRequestComment{
			ID:       12345,
			Body:     requestBody.Body,
			Path:     requestBody.Path,
			Line:     requestBody.Line,
			CommitID: requestBody.CommitID,
			User: User{
				Login: "testuser",
			},
			CreatedAt: "2023-01-01T12:00:00Z",
		}
		
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	client := &Client{
		token:      "test-token",
		baseURL:    server.URL,
		httpClient: &http.Client{},
	}
	
	ctx := context.Background()
	comment, err := client.CreatePullRequestComment(ctx, "testowner", "testrepo", 123, expectedComment)
	if err != nil {
		t.Fatalf("CreatePullRequestComment failed: %v", err)
	}
	
	if comment.ID != 12345 {
		t.Errorf("expected comment ID 12345, got %d", comment.ID)
	}
	
	if comment.Body != expectedComment.Body {
		t.Errorf("expected comment body '%s', got '%s'", expectedComment.Body, comment.Body)
	}
}

func TestCreatePullRequestComment_APIError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  string
	}{
		{
			name:          "authentication error",
			statusCode:    401,
			responseBody:  `{"message": "Bad credentials"}`,
			expectedError: "GitHub API returned status 401",
		},
		{
			name:          "validation error",
			statusCode:    422,
			responseBody:  `{"message": "Validation failed", "errors": [{"field": "line", "message": "Invalid line number"}]}`,
			expectedError: "GitHub API returned status 422",
		},
		{
			name:          "server error",
			statusCode:    500,
			responseBody:  `{"message": "Internal server error"}`,
			expectedError: "GitHub API returned status 500",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()
			
			client := &Client{
				token:      "test-token",
				baseURL:    server.URL,
				httpClient: &http.Client{},
			}
			
			request := CreatePullRequestCommentRequest{
				Body:     "Test comment",
				Path:     "test.go",
				Line:     1,
				CommitID: "abc123",
			}
			
			ctx := context.Background()
			_, err := client.CreatePullRequestComment(ctx, "owner", "repo", 1, request)
			
			if err == nil {
				t.Error("expected error but got none")
			}
			
			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("expected error to contain '%s', got: %v", tt.expectedError, err)
			}
		})
	}
}

func TestConvertReviewCommentToGitHub(t *testing.T) {
	tests := []struct {
		name           string
		reviewComment  ReviewCommentInput
		commitID       string
		expectedResult CreatePullRequestCommentRequest
		shouldConvert  bool
	}{
		{
			name: "valid line comment",
			reviewComment: ReviewCommentInput{
				Filename:   "main.go",
				LineNumber: 15,
				Comment:    "Consider adding error handling here",
			},
			commitID: "abc123def456",
			expectedResult: CreatePullRequestCommentRequest{
				Body:     "Consider adding error handling here",
				Path:     "main.go",
				Line:     15,
				Side:     "RIGHT",
				CommitID: "abc123def456",
			},
			shouldConvert: true,
		},
		{
			name: "general file comment (line 0)",
			reviewComment: ReviewCommentInput{
				Filename:   "utils.go",
				LineNumber: 0,
				Comment:    "Overall file structure looks good",
			},
			commitID:      "abc123def456",
			shouldConvert: false, // Should not convert general comments
		},
		{
			name: "negative line number",
			reviewComment: ReviewCommentInput{
				Filename:   "test.go",
				LineNumber: -1,
				Comment:    "Invalid line number",
			},
			commitID:      "abc123def456",
			shouldConvert: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, shouldConvert := ConvertReviewCommentToGitHub(tt.reviewComment, tt.commitID)
			
			if shouldConvert != tt.shouldConvert {
				t.Errorf("expected shouldConvert %v, got %v", tt.shouldConvert, shouldConvert)
			}
			
			if tt.shouldConvert {
				if result.Body != tt.expectedResult.Body {
					t.Errorf("expected body '%s', got '%s'", tt.expectedResult.Body, result.Body)
				}
				
				if result.Path != tt.expectedResult.Path {
					t.Errorf("expected path '%s', got '%s'", tt.expectedResult.Path, result.Path)
				}
				
				if result.Line != tt.expectedResult.Line {
					t.Errorf("expected line %d, got %d", tt.expectedResult.Line, result.Line)
				}
				
				if result.Side != tt.expectedResult.Side {
					t.Errorf("expected side '%s', got '%s'", tt.expectedResult.Side, result.Side)
				}
				
				if result.CommitID != tt.expectedResult.CommitID {
					t.Errorf("expected commit ID '%s', got '%s'", tt.expectedResult.CommitID, result.CommitID)
				}
			}
		})
	}
}

func TestCreatePullRequestComments_Batch(t *testing.T) {
	requestCount := 0
	expectedComments := []CreatePullRequestCommentRequest{
		{
			Body:     "First comment",
			Path:     "file1.go",
			Line:     10,
			Side:     "RIGHT",
			CommitID: "abc123",
		},
		{
			Body:     "Second comment", 
			Path:     "file2.go",
			Line:     20,
			Side:     "RIGHT",
			CommitID: "abc123",
		},
	}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		
		var requestBody CreatePullRequestCommentRequest
		json.NewDecoder(r.Body).Decode(&requestBody)
		
		// Verify we receive expected comments
		found := false
		for _, expected := range expectedComments {
			if expected.Body == requestBody.Body && expected.Path == requestBody.Path {
				found = true
				break
			}
		}
		
		if !found {
			t.Errorf("unexpected comment received: %+v", requestBody)
		}
		
		response := PullRequestComment{
			ID:   int64(requestCount),
			Body: requestBody.Body,
			Path: requestBody.Path,
			Line: requestBody.Line,
		}
		
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	client := &Client{
		token:      "test-token",
		baseURL:    server.URL,
		httpClient: &http.Client{},
	}
	
	ctx := context.Background()
	results, err := client.CreatePullRequestComments(ctx, "owner", "repo", 123, expectedComments)
	if err != nil {
		t.Fatalf("CreatePullRequestComments failed: %v", err)
	}
	
	if len(results.SuccessfulComments) != 2 {
		t.Errorf("expected 2 successful comments, got %d", len(results.SuccessfulComments))
	}
	
	if len(results.FailedComments) != 0 {
		t.Errorf("expected 0 failed comments, got %d", len(results.FailedComments))
	}
	
	if requestCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", requestCount)
	}
}

func TestCreatePullRequestComments_PartialFailure(t *testing.T) {
	requestCount := 0
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		
		var requestBody CreatePullRequestCommentRequest
		json.NewDecoder(r.Body).Decode(&requestBody)
		
		// First comment succeeds, second fails
		if requestCount == 1 {
			response := PullRequestComment{
				ID:   12345,
				Body: requestBody.Body,
				Path: requestBody.Path,
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message": "Invalid line number"}`))
		}
	}))
	defer server.Close()
	
	client := &Client{
		token:      "test-token",
		baseURL:    server.URL,
		httpClient: &http.Client{},
	}
	
	comments := []CreatePullRequestCommentRequest{
		{Body: "Success comment", Path: "file1.go", Line: 10, CommitID: "abc123"},
		{Body: "Fail comment", Path: "file2.go", Line: -1, CommitID: "abc123"}, // Invalid line
	}
	
	ctx := context.Background()
	results, err := client.CreatePullRequestComments(ctx, "owner", "repo", 123, comments)
	
	// Should not return error for partial failure
	if err != nil {
		t.Errorf("unexpected error for partial failure: %v", err)
	}
	
	if len(results.SuccessfulComments) != 1 {
		t.Errorf("expected 1 successful comment, got %d", len(results.SuccessfulComments))
	}
	
	if len(results.FailedComments) != 1 {
		t.Errorf("expected 1 failed comment, got %d", len(results.FailedComments))
	}
	
	if results.FailedComments[0].Error == "" {
		t.Error("expected error message for failed comment")
	}
}

func TestGetPullRequestComments(t *testing.T) {
	expectedComments := []PullRequestComment{
		{
			ID:   12345,
			Body: "Existing comment 1",
			Path: "main.go",
			Line: 10,
			User: User{Login: "reviewer1"},
		},
		{
			ID:   12346,
			Body: "Existing comment 2", 
			Path: "utils.go",
			Line: 25,
			User: User{Login: "reviewer2"},
		},
	}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/repos/owner/repo/pulls/123/comments"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		
		if r.Method != "GET" {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedComments)
	}))
	defer server.Close()
	
	client := &Client{
		token:      "test-token",
		baseURL:    server.URL,
		httpClient: &http.Client{},
	}
	
	ctx := context.Background()
	comments, err := client.GetPullRequestComments(ctx, "owner", "repo", 123)
	if err != nil {
		t.Fatalf("GetPullRequestComments failed: %v", err)
	}
	
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
	
	if comments[0].ID != 12345 {
		t.Errorf("expected first comment ID 12345, got %d", comments[0].ID)
	}
	
	if comments[1].Body != "Existing comment 2" {
		t.Errorf("expected second comment body 'Existing comment 2', got '%s'", comments[1].Body)
	}
}