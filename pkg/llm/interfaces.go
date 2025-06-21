package llm

import (
	"context"

	"github.com/GDSources/claude-code-review-agent/pkg/analyzer"
	"github.com/GDSources/claude-code-review-agent/pkg/github"
)

// CodeReviewer interface for LLM-based code review providers
type CodeReviewer interface {
	ReviewCode(ctx context.Context, request *ReviewRequest) (*ReviewResponse, error)
	ValidateConfiguration() error
	GetModelInfo() ModelInfo
}

// ReviewRequest contains all data needed for LLM code review
type ReviewRequest struct {
	PullRequestInfo PullRequestInfo          `json:"pull_request_info"`
	DiffResult      *github.DiffResult       `json:"diff_result"`
	ContextualDiff  *analyzer.ContextualDiff `json:"contextual_diff"`
	ReviewType      ReviewType               `json:"review_type"`
	Instructions    string                   `json:"instructions,omitempty"`
}

// ReviewResponse contains the LLM's code review results
type ReviewResponse struct {
	Comments    []ReviewComment `json:"comments"`
	Summary     string          `json:"summary"`
	ModelUsed   string          `json:"model_used"`
	TokensUsed  TokenUsage      `json:"tokens_used"`
	ReviewID    string          `json:"review_id"`
	GeneratedAt string          `json:"generated_at"`
}

// ReviewComment represents a single review comment
type ReviewComment struct {
	Filename   string      `json:"filename"`
	LineNumber int         `json:"line_number,omitempty"`
	LineRange  *LineRange  `json:"line_range,omitempty"`
	Comment    string      `json:"comment"`
	Severity   Severity    `json:"severity"`
	Type       CommentType `json:"type"`
	Suggestion string      `json:"suggestion,omitempty"`
	Category   string      `json:"category,omitempty"`
}

// Supporting data structures

type PullRequestInfo struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description,omitempty"`
	BaseBranch  string `json:"base_branch"`
	HeadBranch  string `json:"head_branch"`
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ModelInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	MaxTokens   int    `json:"max_tokens"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}

// Enums

type ReviewType string

const (
	ReviewTypeGeneral     ReviewType = "general"
	ReviewTypeSecurity    ReviewType = "security"
	ReviewTypePerformance ReviewType = "performance"
	ReviewTypeStyle       ReviewType = "style"
	ReviewTypeBugs        ReviewType = "bugs"
	ReviewTypeTests       ReviewType = "tests"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityMinor    Severity = "minor"
	SeverityMajor    Severity = "major"
	SeverityCritical Severity = "critical"
)

type CommentType string

const (
	CommentTypeSuggestion CommentType = "suggestion"
	CommentTypeIssue      CommentType = "issue"
	CommentTypePraise     CommentType = "praise"
	CommentTypeQuestion   CommentType = "question"
	CommentTypeNitpick    CommentType = "nitpick"
)

// Provider-specific configurations

// ClaudeConfig contains Claude-specific configuration
type ClaudeConfig struct {
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	BaseURL     string  `json:"base_url"`
	Timeout     int     `json:"timeout_seconds"`
}

// Default configurations
const (
	DefaultClaudeModel       = "claude-sonnet-4-20250514"
	DefaultClaudeMaxTokens   = 4000
	DefaultClaudeTemperature = 0.1
	DefaultClaudeBaseURL     = "https://api.anthropic.com"
	DefaultTimeoutSeconds    = 120
)

// Available Claude models
const (
	ClaudeHaiku35        = "claude-3-5-haiku-20241022"  // Fast, cost-effective
	ClaudeSonnet35       = "claude-3-5-sonnet-20241022" // Balanced performance
	ClaudeSonnet35Latest = "claude-3-5-sonnet-20250106" // Latest Sonnet 3.5
	ClaudeHaiku37        = "claude-3-7-haiku-20250109"  // Claude 3.7 Haiku - Fast and efficient
	ClaudeSonnet37       = "claude-3-7-sonnet-20250109" // Claude 3.7 Sonnet - Enhanced capabilities
	ClaudeSonnet4        = "claude-sonnet-4-20250514"   // Most capable, highest cost (recommended)
)

// AvailableClaudeModels returns a list of supported Claude models
var AvailableClaudeModels = []string{
	ClaudeHaiku35,
	ClaudeSonnet35,
	ClaudeSonnet35Latest,
	ClaudeHaiku37,
	ClaudeSonnet37,
	ClaudeSonnet4,
}

// ModelDescriptions provides human-readable descriptions for each model
var ModelDescriptions = map[string]string{
	ClaudeHaiku35:        "Claude 3.5 Haiku - Fast and cost-effective, good for simple reviews",
	ClaudeSonnet35:       "Claude 3.5 Sonnet - Balanced performance and cost",
	ClaudeSonnet35Latest: "Claude 3.5 Sonnet (Latest) - Enhanced capabilities with recent improvements",
	ClaudeHaiku37:        "Claude 3.7 Haiku - Fast and efficient with improved reasoning",
	ClaudeSonnet37:       "Claude 3.7 Sonnet - Enhanced capabilities and better performance",
	ClaudeSonnet4:        "Claude Sonnet 4 - Most capable model, best for complex analysis (recommended)",
}
