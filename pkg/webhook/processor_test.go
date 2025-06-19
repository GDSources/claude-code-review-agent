package webhook

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type mockReviewOrchestrator struct {
	shouldFail      bool
	error           error
	receivedEvents  []*PullRequestEvent
	processedEvents int
}

func (m *mockReviewOrchestrator) HandlePullRequest(event *PullRequestEvent) error {
	m.receivedEvents = append(m.receivedEvents, event)
	m.processedEvents++
	if m.shouldFail {
		return m.error
	}
	return nil
}

func TestGitHubEventProcessor_ProcessPullRequest(t *testing.T) {
	pullRequestPayload := `{
		"action": "opened",
		"number": 123,
		"pull_request": {
			"id": 456,
			"number": 123,
			"title": "Test PR",
			"body": "This is a test pull request",
			"state": "open",
			"head": {
				"ref": "feature-branch",
				"sha": "abc123def456",
				"repo": {
					"id": 789,
					"name": "test-repo",
					"full_name": "owner/test-repo",
					"private": false,
					"owner": {
						"id": 1001,
						"login": "owner"
					}
				}
			},
			"base": {
				"ref": "main",
				"sha": "def456ghi789",
				"repo": {
					"id": 789,
					"name": "test-repo",
					"full_name": "owner/test-repo",
					"private": false,
					"owner": {
						"id": 1001,
						"login": "owner"
					}
				}
			},
			"user": {
				"id": 2002,
				"login": "contributor"
			}
		},
		"repository": {
			"id": 789,
			"name": "test-repo",
			"full_name": "owner/test-repo",
			"private": false,
			"owner": {
				"id": 1001,
				"login": "owner"
			}
		}
	}`

	tests := []struct {
		name            string
		eventType       string
		payload         string
		orchestratorErr error
		expectError     bool
		expectCall      bool
		errorContains   string
	}{
		{
			name:        "successful pull request processing",
			eventType:   "pull_request",
			payload:     pullRequestPayload,
			expectError: false,
			expectCall:  true,
		},
		{
			name:            "orchestrator fails",
			eventType:       "pull_request",
			payload:         pullRequestPayload,
			orchestratorErr: fmt.Errorf("review failed"),
			expectError:     true,
			expectCall:      true,
			errorContains:   "failed to handle pull request event",
		},
		{
			name:          "invalid JSON payload",
			eventType:     "pull_request",
			payload:       `{"invalid": json}`,
			expectError:   true,
			expectCall:    false,
			errorContains: "failed to parse pull request event",
		},
		{
			name:        "ping event",
			eventType:   "ping",
			payload:     `{"zen": "Speak like a human."}`,
			expectError: false,
			expectCall:  false,
		},
		{
			name:          "unsupported event type",
			eventType:     "issues",
			payload:       `{"action": "opened"}`,
			expectError:   true,
			expectCall:    false,
			errorContains: "unsupported event type: issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := &mockReviewOrchestrator{
				shouldFail: tt.orchestratorErr != nil,
				error:      tt.orchestratorErr,
			}
			processor := NewGitHubEventProcessor(orchestrator)

			err := processor.Process(tt.eventType, []byte(tt.payload))

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}

			if tt.expectCall && orchestrator.processedEvents != 1 {
				t.Errorf("expected orchestrator to be called once, got %d calls", orchestrator.processedEvents)
			}
			if !tt.expectCall && orchestrator.processedEvents != 0 {
				t.Errorf("expected orchestrator not to be called, got %d calls", orchestrator.processedEvents)
			}

			if tt.expectCall && len(orchestrator.receivedEvents) > 0 {
				event := orchestrator.receivedEvents[0]
				if event.Action != "opened" {
					t.Errorf("expected action 'opened', got '%s'", event.Action)
				}
				if event.Number != 123 {
					t.Errorf("expected number 123, got %d", event.Number)
				}
				if event.PullRequest.Title != "Test PR" {
					t.Errorf("expected title 'Test PR', got '%s'", event.PullRequest.Title)
				}
			}
		})
	}
}

func TestGitHubEventProcessor_PullRequestActions(t *testing.T) {
	basePayload := map[string]interface{}{
		"number": 123,
		"pull_request": map[string]interface{}{
			"id":     456,
			"number": 123,
			"title":  "Test PR",
			"body":   "",
			"state":  "open",
			"head": map[string]interface{}{
				"ref": "feature",
				"sha": "abc123",
				"repo": map[string]interface{}{
					"id":        789,
					"name":      "test-repo",
					"full_name": "owner/test-repo",
					"private":   false,
					"owner": map[string]interface{}{
						"id":    1001,
						"login": "owner",
					},
				},
			},
			"base": map[string]interface{}{
				"ref": "main",
				"sha": "def456",
				"repo": map[string]interface{}{
					"id":        789,
					"name":      "test-repo",
					"full_name": "owner/test-repo",
					"private":   false,
					"owner": map[string]interface{}{
						"id":    1001,
						"login": "owner",
					},
				},
			},
			"user": map[string]interface{}{
				"id":    2002,
				"login": "contributor",
			},
		},
		"repository": map[string]interface{}{
			"id":        789,
			"name":      "test-repo",
			"full_name": "owner/test-repo",
			"private":   false,
			"owner": map[string]interface{}{
				"id":    1001,
				"login": "owner",
			},
		},
	}

	actions := []struct {
		action      string
		shouldCall  bool
		description string
	}{
		{"opened", true, "should process opened PRs"},
		{"synchronize", true, "should process synchronized PRs"},
		{"reopened", true, "should process reopened PRs"},
		{"closed", false, "should not process closed PRs"},
		{"edited", false, "should not process edited PRs"},
		{"assigned", false, "should not process assigned PRs"},
		{"labeled", false, "should not process labeled PRs"},
	}

	for _, action := range actions {
		t.Run(action.description, func(t *testing.T) {
			orchestrator := &mockReviewOrchestrator{}
			processor := NewGitHubEventProcessor(orchestrator)

			payload := basePayload
			payload["action"] = action.action
			payloadBytes, _ := json.Marshal(payload)

			err := processor.Process("pull_request", payloadBytes)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if action.shouldCall && orchestrator.processedEvents != 1 {
				t.Errorf("expected orchestrator to be called for action '%s', got %d calls", action.action, orchestrator.processedEvents)
			}
			if !action.shouldCall && orchestrator.processedEvents != 0 {
				t.Errorf("expected orchestrator not to be called for action '%s', got %d calls", action.action, orchestrator.processedEvents)
			}
		})
	}
}

func TestGitHubEventProcessor_PayloadValidation(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		expectError   bool
		errorContains string
	}{
		{
			name:    "empty payload",
			payload: `{}`,
		},
		{
			name:          "invalid JSON",
			payload:       `{invalid json`,
			expectError:   true,
			errorContains: "failed to parse pull request event",
		},
		{
			name:    "missing optional fields",
			payload: `{"action": "opened", "number": 1}`,
		},
		{
			name: "partial pull request data",
			payload: `{
				"action": "opened",
				"number": 1,
				"pull_request": {
					"id": 123,
					"title": "Test"
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := &mockReviewOrchestrator{}
			processor := NewGitHubEventProcessor(orchestrator)

			err := processor.Process("pull_request", []byte(tt.payload))

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errorContains)) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}
		})
	}
}

func TestGitHubEventProcessor_PingEvent(t *testing.T) {
	orchestrator := &mockReviewOrchestrator{}
	processor := NewGitHubEventProcessor(orchestrator)

	pingPayloads := []string{
		`{"zen": "Speak like a human."}`,
		`{"hook_id": 123, "zen": "Mind your words."}`,
		`{}`,
	}

	for i, payload := range pingPayloads {
		t.Run(fmt.Sprintf("ping payload %d", i+1), func(t *testing.T) {
			err := processor.Process("ping", []byte(payload))

			if err != nil {
				t.Errorf("ping event should not error, got: %v", err)
			}

			if orchestrator.processedEvents != 0 {
				t.Errorf("ping event should not trigger orchestrator, got %d calls", orchestrator.processedEvents)
			}
		})
	}
}

func TestGitHubEventProcessor_ShouldProcessPullRequestAction(t *testing.T) {
	processor := &GitHubEventProcessor{}

	tests := []struct {
		action   string
		expected bool
	}{
		{"opened", true},
		{"synchronize", true},
		{"reopened", true},
		{"closed", false},
		{"edited", false},
		{"assigned", false},
		{"unassigned", false},
		{"labeled", false},
		{"unlabeled", false},
		{"review_requested", false},
		{"", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("action_%s", tt.action), func(t *testing.T) {
			result := processor.shouldProcessPullRequestAction(tt.action)
			if result != tt.expected {
				t.Errorf("expected %v for action '%s', got %v", tt.expected, tt.action, result)
			}
		})
	}
}
