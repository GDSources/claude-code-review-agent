package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// parseJSON is a helper function to parse JSON with error handling
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
