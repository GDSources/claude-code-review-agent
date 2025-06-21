package claude

// Common Claude API structures used across different clients
// These types are shared between the main code review client and
// specialized clients like the deletion analysis client.

// BaseRequest represents the common structure for all Claude API requests
type BaseRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
}

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BaseResponse represents the common structure for all Claude API responses
type BaseResponse struct {
	Content []Content `json:"content"`
	Model   string    `json:"model"`
	Usage   Usage     `json:"usage"`
	ID      string    `json:"id"`
	Type    string    `json:"type"`
}

// Content represents a piece of content in Claude's response
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Error represents Claude API error details
type Error struct {
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// ErrorResponse represents the structure of Claude API error responses
type ErrorResponse struct {
	Error Error `json:"error"`
}

// HTTPStatusError represents HTTP status-specific error information
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e HTTPStatusError) Error() string {
	return e.Message
}
