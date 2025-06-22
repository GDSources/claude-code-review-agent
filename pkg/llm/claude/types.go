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

// ResourceLimits defines limits for memory and processing to prevent resource exhaustion
type ResourceLimits struct {
	MaxCodebaseSize   int64 `json:"max_codebase_size"`   // Maximum codebase size in bytes
	MaxFileCount      int   `json:"max_file_count"`      // Maximum number of files to process
	MaxPromptLength   int   `json:"max_prompt_length"`   // Maximum prompt length in characters
	MaxResponseLength int   `json:"max_response_length"` // Maximum response length in characters
	MaxContextLines   int   `json:"max_context_lines"`   // Maximum context lines per file
}

// DefaultResourceLimits provides sensible defaults for resource limits
var DefaultResourceLimits = ResourceLimits{
	MaxCodebaseSize:   50 * 1024 * 1024, // 50MB
	MaxFileCount:      1000,             // 1000 files max
	MaxPromptLength:   500000,           // 500KB prompt max
	MaxResponseLength: 100000,           // 100KB response max
	MaxContextLines:   50,               // 50 lines context per file
}
