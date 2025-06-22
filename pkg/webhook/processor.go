package webhook

import (
	"encoding/json"
	"fmt"
)

type ReviewOrchestrator interface {
	HandlePullRequest(event *PullRequestEvent) (*ReviewResult, error)
}

// ReviewResult contains the outcome of a review operation
type ReviewResult struct {
	CommentsPosted int    `json:"comments_posted"`
	Status         string `json:"status"`
	Summary        string `json:"summary,omitempty"`
}

type GitHubEventProcessor struct {
	orchestrator ReviewOrchestrator
}

func NewGitHubEventProcessor(orchestrator ReviewOrchestrator) *GitHubEventProcessor {
	return &GitHubEventProcessor{
		orchestrator: orchestrator,
	}
}

type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  Repository  `json:"repository"`
}

type PullRequest struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Head   Branch `json:"head"`
	Base   Branch `json:"base"`
	User   User   `json:"user"`
}

type Repository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Owner    User   `json:"owner"`
}

type Branch struct {
	Ref  string     `json:"ref"`
	SHA  string     `json:"sha"`
	Repo Repository `json:"repo"`
}

type User struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

func (p *GitHubEventProcessor) Process(eventType string, payload []byte) error {
	switch eventType {
	case "pull_request":
		return p.processPullRequest(payload)
	case "ping":
		return p.processPing(payload)
	default:
		return fmt.Errorf("unsupported event type: %s", eventType)
	}
}

func (p *GitHubEventProcessor) processPullRequest(payload []byte) error {
	var event PullRequestEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("failed to parse pull request event: %w", err)
	}

	if !p.shouldProcessPullRequestAction(event.Action) {
		return nil
	}

	_, err := p.orchestrator.HandlePullRequest(&event)
	if err != nil {
		return fmt.Errorf("failed to handle pull request event: %w", err)
	}

	return nil
}

func (p *GitHubEventProcessor) processPing(payload []byte) error {
	var pingEvent map[string]interface{}
	if err := json.Unmarshal(payload, &pingEvent); err != nil {
		return fmt.Errorf("failed to parse ping event: %w", err)
	}
	return nil
}

func (p *GitHubEventProcessor) shouldProcessPullRequestAction(action string) bool {
	processableActions := map[string]bool{
		"opened":      true,
		"synchronize": true,
		"reopened":    true,
	}
	return processableActions[action]
}
