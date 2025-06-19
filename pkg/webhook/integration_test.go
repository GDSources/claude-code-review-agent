package webhook

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookIntegration_EndToEnd(t *testing.T) {
	secret := "test-integration-secret"
	validator := NewHMACValidator(secret)
	orchestrator := &mockReviewOrchestrator{}
	processor := NewGitHubEventProcessor(orchestrator)
	handler := NewHandler(validator, processor)

	pullRequestPayload := `{
		"action": "opened",
		"number": 42,
		"pull_request": {
			"id": 123456,
			"number": 42,
			"title": "Add amazing feature",
			"body": "This PR adds an amazing new feature",
			"state": "open",
			"head": {
				"ref": "feature/amazing",
				"sha": "abc123def456ghi789",
				"repo": {
					"id": 789,
					"name": "awesome-repo",
					"full_name": "company/awesome-repo",
					"private": true,
					"owner": {
						"id": 1001,
						"login": "company"
					}
				}
			},
			"base": {
				"ref": "main",
				"sha": "def456ghi789abc123",
				"repo": {
					"id": 789,
					"name": "awesome-repo",
					"full_name": "company/awesome-repo",
					"private": true,
					"owner": {
						"id": 1001,
						"login": "company"
					}
				}
			},
			"user": {
				"id": 2002,
				"login": "developer"
			}
		},
		"repository": {
			"id": 789,
			"name": "awesome-repo",
			"full_name": "company/awesome-repo",
			"private": true,
			"owner": {
				"id": 1001,
				"login": "company"
			}
		}
	}`

	tests := []struct {
		name               string
		method             string
		contentType        string
		githubEvent        string
		payload            string
		generateSignature  bool
		orchestratorFails  bool
		expectedStatusCode int
		expectedError      string
		expectOrchestrator bool
	}{
		{
			name:               "successful end-to-end flow",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			payload:            pullRequestPayload,
			generateSignature:  true,
			expectedStatusCode: http.StatusOK,
			expectOrchestrator: true,
		},
		{
			name:               "missing signature fails at validation layer",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			payload:            pullRequestPayload,
			generateSignature:  false,
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Validation failed",
			expectOrchestrator: false,
		},
		{
			name:               "wrong method fails at HTTP layer",
			method:             http.MethodGet,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			payload:            pullRequestPayload,
			generateSignature:  true,
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedError:      "Method not allowed",
			expectOrchestrator: false,
		},
		{
			name:               "orchestrator failure propagates",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			payload:            pullRequestPayload,
			generateSignature:  true,
			orchestratorFails:  true,
			expectedStatusCode: http.StatusInternalServerError,
			expectedError:      "Failed to process event",
			expectOrchestrator: true,
		},
		{
			name:               "ping event succeeds without orchestrator call",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "ping",
			payload:            `{"zen": "Keep it simple."}`,
			generateSignature:  true,
			expectedStatusCode: http.StatusOK,
			expectOrchestrator: false,
		},
		{
			name:               "closed PR action ignored",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			payload:            strings.Replace(pullRequestPayload, `"action": "opened"`, `"action": "closed"`, 1),
			generateSignature:  true,
			expectedStatusCode: http.StatusOK,
			expectOrchestrator: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset orchestrator state
			orchestrator.receivedEvents = nil
			orchestrator.processedEvents = 0
			orchestrator.shouldFail = tt.orchestratorFails
			if tt.orchestratorFails {
				orchestrator.error = fmt.Errorf("orchestrator processing failed")
			}

			req := httptest.NewRequest(tt.method, "/webhook", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", tt.contentType)
			if tt.githubEvent != "" {
				req.Header.Set("X-GitHub-Event", tt.githubEvent)
			}
			if tt.generateSignature {
				signature := validator.GenerateSignature([]byte(tt.payload))
				req.Header.Set("X-Hub-Signature-256", signature)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Verify HTTP response
			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			if tt.expectedError != "" {
				if !strings.Contains(rr.Body.String(), tt.expectedError) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.expectedError, rr.Body.String())
				}
			}

			// Verify orchestrator interaction
			if tt.expectOrchestrator && orchestrator.processedEvents != 1 {
				t.Errorf("expected orchestrator to be called once, got %d calls", orchestrator.processedEvents)
			}
			if !tt.expectOrchestrator && orchestrator.processedEvents != 0 {
				t.Errorf("expected orchestrator not to be called, got %d calls", orchestrator.processedEvents)
			}

			// Verify event data propagation
			if tt.expectOrchestrator && len(orchestrator.receivedEvents) > 0 {
				event := orchestrator.receivedEvents[0]
				if tt.githubEvent == "pull_request" {
					if event.Number != 42 {
						t.Errorf("expected PR number 42, got %d", event.Number)
					}
					if event.PullRequest.Title != "Add amazing feature" {
						t.Errorf("expected PR title 'Add amazing feature', got '%s'", event.PullRequest.Title)
					}
					if event.Repository.FullName != "company/awesome-repo" {
						t.Errorf("expected repo 'company/awesome-repo', got '%s'", event.Repository.FullName)
					}
				}
			}
		})
	}
}

func TestWebhookIntegration_SecurityBoundary(t *testing.T) {
	secret := "super-secret-key"
	validator := NewHMACValidator(secret)
	orchestrator := &mockReviewOrchestrator{}
	processor := NewGitHubEventProcessor(orchestrator)
	handler := NewHandler(validator, processor)

	payload := `{"action": "opened", "number": 1}`

	tests := []struct {
		name               string
		secret             string
		payload            string
		expectedStatusCode int
		expectProcessing   bool
	}{
		{
			name:               "correct secret allows processing",
			secret:             secret,
			payload:            payload,
			expectedStatusCode: http.StatusOK,
			expectProcessing:   true,
		},
		{
			name:               "wrong secret blocks processing",
			secret:             "wrong-secret",
			payload:            payload,
			expectedStatusCode: http.StatusUnauthorized,
			expectProcessing:   false,
		},
		{
			name:               "modified payload fails validation",
			secret:             secret,
			payload:            `{"action": "opened", "number": 2}`, // Different payload
			expectedStatusCode: http.StatusUnauthorized,
			expectProcessing:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator.receivedEvents = nil
			orchestrator.processedEvents = 0

			// Generate signature with potentially wrong secret/payload
			wrongValidator := NewHMACValidator(tt.secret)
			signature := wrongValidator.GenerateSignature([]byte(payload)) // Always sign original payload

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")
			req.Header.Set("X-Hub-Signature-256", signature)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			if tt.expectProcessing && orchestrator.processedEvents != 1 {
				t.Errorf("expected processing to occur, got %d calls", orchestrator.processedEvents)
			}
			if !tt.expectProcessing && orchestrator.processedEvents != 0 {
				t.Errorf("expected no processing, got %d calls", orchestrator.processedEvents)
			}
		})
	}
}

func TestWebhookIntegration_ErrorPropagation(t *testing.T) {
	secret := "test-secret"
	validator := NewHMACValidator(secret)
	orchestrator := &mockReviewOrchestrator{}
	processor := NewGitHubEventProcessor(orchestrator)
	handler := NewHandler(validator, processor)

	payload := `{"action": "opened", "number": 123}`
	signature := validator.GenerateSignature([]byte(payload))

	// Test error propagation from each layer
	errorTests := []struct {
		name               string
		setupError         func()
		expectedStatusCode int
		errorLayer         string
	}{
		{
			name: "orchestrator error propagates",
			setupError: func() {
				orchestrator.shouldFail = true
				orchestrator.error = fmt.Errorf("review service unavailable")
			},
			expectedStatusCode: http.StatusInternalServerError,
			errorLayer:         "business logic",
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state
			orchestrator.receivedEvents = nil
			orchestrator.processedEvents = 0
			orchestrator.shouldFail = false
			orchestrator.error = nil

			// Setup error condition
			tt.setupError()

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")
			req.Header.Set("X-Hub-Signature-256", signature)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d from %s layer, got %d", tt.expectedStatusCode, tt.errorLayer, rr.Code)
			}
		})
	}
}

func TestWebhookIntegration_LayerIsolation(t *testing.T) {
	// Test that each layer can be independently replaced/mocked
	secret := "isolation-test-secret"

	// Test with different validator implementations
	t.Run("validator layer isolation", func(t *testing.T) {
		// Custom validator that always fails
		alwaysFailValidator := &mockValidator{shouldFail: true, error: fmt.Errorf("custom validation error")}
		orchestrator := &mockReviewOrchestrator{}
		processor := NewGitHubEventProcessor(orchestrator)
		handler := NewHandler(alwaysFailValidator, processor)

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "pull_request")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected custom validator to fail with 401, got %d", rr.Code)
		}
		if orchestrator.processedEvents != 0 {
			t.Error("orchestrator should not be called when validator fails")
		}
	})

	// Test with different processor implementations
	t.Run("processor layer isolation", func(t *testing.T) {
		// Custom processor that always fails
		alwaysFailProcessor := &mockEventProcessor{shouldFail: true, error: fmt.Errorf("custom processing error")}
		validator := NewHMACValidator(secret)
		handler := NewHandler(validator, alwaysFailProcessor)

		payload := `{}`
		signature := validator.GenerateSignature([]byte(payload))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "pull_request")
		req.Header.Set("X-Hub-Signature-256", signature)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected custom processor to fail with 500, got %d", rr.Code)
		}
	})
}
