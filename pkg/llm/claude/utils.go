package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// MaskAPIKey returns a masked version of the API key for logging
func MaskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}

// HandleHTTPError converts HTTP status codes to appropriate error messages
func HandleHTTPError(statusCode int, respBody []byte) error {
	var errResp ErrorResponse
	if err := parseJSON(respBody, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: failed to parse error response", statusCode)
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("authentication failed (check API key): %s", errResp.Error.Message)
	case http.StatusForbidden:
		return fmt.Errorf("forbidden (check API permissions): %s", errResp.Error.Message)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limit exceeded: %s", errResp.Error.Message)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return fmt.Errorf("server error: %s", errResp.Error.Message)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, errResp.Error.Message)
	}
}

// ShouldRetry determines if an error is retryable
func ShouldRetry(err error) bool {
	errStr := err.Error()
	// Retry on rate limits and server errors
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "server error") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection")
}

// ExtractTextContent extracts all text content from Claude's response
func ExtractTextContent(resp *BaseResponse) string {
	var fullText strings.Builder
	for _, content := range resp.Content {
		if content.Type == "text" {
			fullText.WriteString(content.Text)
		}
	}
	return fullText.String()
}

// SanitizeInput sanitizes user input to prevent injection attacks
func SanitizeInput(input string) string {
	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if unicode.IsPrint(r) || r == '\n' || r == '\t' || r == '\r' {
			result.WriteRune(r)
		}
	}
	
	sanitized := result.String()
	
	// Limit input length to prevent memory issues
	const maxInputLength = 100000 // 100KB limit
	if len(sanitized) > maxInputLength {
		sanitized = sanitized[:maxInputLength] + "... [truncated]"
	}
	
	return sanitized
}

// ValidateAPIKey validates that an API key has a reasonable format
func ValidateAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}
	
	// Check for reasonable API key patterns (Claude keys start with sk-ant-)
	if !strings.HasPrefix(apiKey, "sk-ant-") {
		return fmt.Errorf("API key format appears invalid")
	}
	
	// Check length (typical Claude API keys are around 100+ characters)
	if len(apiKey) < 50 {
		return fmt.Errorf("API key appears too short")
	}
	
	// Check for suspicious characters
	suspiciousPattern := regexp.MustCompile(`[<>{}|&;$()'"\\]`)
	if suspiciousPattern.MatchString(apiKey) {
		return fmt.Errorf("API key contains suspicious characters")
	}
	
	return nil
}

// SanitizeAndValidatePrompt sanitizes and validates prompt content
func SanitizeAndValidatePrompt(prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}
	
	sanitized := SanitizeInput(prompt)
	
	// Check for potential prompt injection patterns
	suspiciousPatterns := []string{
		"ignore previous instructions",
		"disregard the above",
		"forget everything above",
		"<script>",
		"javascript:",
		"eval(",
	}
	
	lowerPrompt := strings.ToLower(sanitized)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerPrompt, pattern) {
			return "", fmt.Errorf("prompt contains potentially malicious content")
		}
	}
	
	return sanitized, nil
}

// ValidateResourceLimits validates that input adheres to resource limits
func ValidateResourceLimits(content string, limits ResourceLimits) error {
	if len(content) > limits.MaxPromptLength {
		return fmt.Errorf("content exceeds maximum prompt length (%d > %d)", 
			len(content), limits.MaxPromptLength)
	}
	return nil
}

// TruncateWithLimits truncates content to fit within resource limits
func TruncateWithLimits(content string, limits ResourceLimits) string {
	if len(content) <= limits.MaxPromptLength {
		return content
	}
	
	truncated := content[:limits.MaxPromptLength-20] // Leave room for truncation notice
	return truncated + "\n... [truncated for size]"
}

// ValidateCodebaseSize checks if codebase size is within limits
func ValidateCodebaseSize(totalSize int64, fileCount int, limits ResourceLimits) error {
	if totalSize > limits.MaxCodebaseSize {
		return fmt.Errorf("codebase size exceeds limit (%d MB > %d MB)", 
			totalSize/(1024*1024), limits.MaxCodebaseSize/(1024*1024))
	}
	
	if fileCount > limits.MaxFileCount {
		return fmt.Errorf("file count exceeds limit (%d > %d)", 
			fileCount, limits.MaxFileCount)
	}
	
	return nil
}

// parseJSON is a helper function to parse JSON with error handling
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
