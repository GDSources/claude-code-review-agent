package webhook

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockValidator struct {
	shouldFail bool
	error      error
}

func (m *mockValidator) Validate(req *http.Request, body []byte) error {
	if m.shouldFail {
		return m.error
	}
	return nil
}

type mockEventProcessor struct {
	shouldFail    bool
	error         error
	receivedEvent string
	receivedBody  []byte
}

func (m *mockEventProcessor) Process(eventType string, payload []byte) error {
	m.receivedEvent = eventType
	m.receivedBody = payload
	if m.shouldFail {
		return m.error
	}
	return nil
}

func TestHandler_HTTPLayer(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		contentType        string
		githubEvent        string
		body               string
		expectedStatusCode int
		expectedError      string
	}{
		{
			name:               "successful POST request",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			body:               `{"action": "opened"}`,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "reject GET method",
			method:             http.MethodGet,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			body:               `{"action": "opened"}`,
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedError:      "Method not allowed",
		},
		{
			name:               "reject PUT method",
			method:             http.MethodPut,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			body:               `{"action": "opened"}`,
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedError:      "Method not allowed",
		},
		{
			name:               "reject wrong content type",
			method:             http.MethodPost,
			contentType:        "text/plain",
			githubEvent:        "pull_request",
			body:               `{"action": "opened"}`,
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Content-Type must be application/json",
		},
		{
			name:               "reject missing GitHub event header",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "",
			body:               `{"action": "opened"}`,
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Missing X-GitHub-Event header",
		},
		{
			name:               "reject request body too large",
			method:             http.MethodPost,
			contentType:        "application/json",
			githubEvent:        "pull_request",
			body:               strings.Repeat("x", 2*1024*1024), // 2MB
			expectedStatusCode: http.StatusRequestEntityTooLarge,
			expectedError:      "Request body too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &mockValidator{}
			processor := &mockEventProcessor{}
			handler := NewHandler(validator, processor)

			req := httptest.NewRequest(tt.method, "/webhook", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			if tt.githubEvent != "" {
				req.Header.Set("X-GitHub-Event", tt.githubEvent)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			if tt.expectedError != "" {
				if !strings.Contains(rr.Body.String(), tt.expectedError) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.expectedError, rr.Body.String())
				}
			}

			if tt.expectedStatusCode == http.StatusOK {
				if processor.receivedEvent != tt.githubEvent {
					t.Errorf("expected event type '%s', got '%s'", tt.githubEvent, processor.receivedEvent)
				}
				if string(processor.receivedBody) != tt.body {
					t.Errorf("expected body '%s', got '%s'", tt.body, string(processor.receivedBody))
				}
			}
		})
	}
}

func TestHandler_ValidationLayer(t *testing.T) {
	tests := []struct {
		name                string
		validatorShouldFail bool
		validatorError      error
		expectedStatusCode  int
		expectedError       string
	}{
		{
			name:                "validation passes",
			validatorShouldFail: false,
			expectedStatusCode:  http.StatusOK,
		},
		{
			name:                "validation fails",
			validatorShouldFail: true,
			validatorError:      fmt.Errorf("invalid signature"),
			expectedStatusCode:  http.StatusUnauthorized,
			expectedError:       "Validation failed: invalid signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &mockValidator{
				shouldFail: tt.validatorShouldFail,
				error:      tt.validatorError,
			}
			processor := &mockEventProcessor{}
			handler := NewHandler(validator, processor)

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			if tt.expectedError != "" {
				if !strings.Contains(rr.Body.String(), tt.expectedError) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.expectedError, rr.Body.String())
				}
			}
		})
	}
}

func TestHandler_EventProcessingLayer(t *testing.T) {
	tests := []struct {
		name                string
		processorShouldFail bool
		processorError      error
		expectedStatusCode  int
		expectedError       string
	}{
		{
			name:                "event processing succeeds",
			processorShouldFail: false,
			expectedStatusCode:  http.StatusOK,
		},
		{
			name:                "event processing fails",
			processorShouldFail: true,
			processorError:      fmt.Errorf("failed to process pull request"),
			expectedStatusCode:  http.StatusInternalServerError,
			expectedError:       "Failed to process event: failed to process pull request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &mockValidator{}
			processor := &mockEventProcessor{
				shouldFail: tt.processorShouldFail,
				error:      tt.processorError,
			}
			handler := NewHandler(validator, processor)

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, rr.Code)
			}

			if tt.expectedError != "" {
				if !strings.Contains(rr.Body.String(), tt.expectedError) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.expectedError, rr.Body.String())
				}
			}
		})
	}
}

func TestHandler_MaxBodySize(t *testing.T) {
	validator := &mockValidator{}
	processor := &mockEventProcessor{}
	handler := NewHandler(validator, processor)
	handler.maxBodySize = 10 // Very small limit for testing

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("this is longer than 10 bytes"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status code %d, got %d", http.StatusRequestEntityTooLarge, rr.Code)
	}
}
