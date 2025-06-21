package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EnvLoader struct {
	searchPaths []string
}

func NewEnvLoader() *EnvLoader {
	// Define common search paths for .env files
	currentDir, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()

	return &EnvLoader{
		searchPaths: []string{
			currentDir,
			homeDir,
			filepath.Join(homeDir, ".config", "review-agent"),
		},
	}
}

func (e *EnvLoader) LoadEnvFile() error {
	for _, path := range e.searchPaths {
		envFile := filepath.Join(path, ".env")
		err := e.loadFromFile(envFile)
		if err == nil {
			return nil // Successfully loaded from this file
		}

		// If file exists but has parsing errors, return the error
		if !os.IsNotExist(err) {
			return err
		}
		// If file doesn't exist, continue to next search path
	}

	// No .env file found, which is okay
	return nil
}

func (e *EnvLoader) loadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if err := e.parseLine(line, filename, lineNum); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading %s: %w", filename, err)
	}

	return nil
}

func (e *EnvLoader) parseLine(line, filename string, lineNum int) error {
	// Split on first '=' to handle values that contain '='
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format in %s at line %d: %s", filename, lineNum, line)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	// Remove quotes if present
	if len(value) >= 2 {
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}
	}

	// Only set if not already set (environment variables take precedence)
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}

	return nil
}

func (e *EnvLoader) CreateSampleEnvFile() error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	envFile := filepath.Join(currentDir, ".env.example")

	content := `# Review Agent Configuration
# Copy this file to .env and update with your actual keys

# GitHub Personal Access Token
# Get from: https://github.com/settings/tokens
GITHUB_TOKEN=your_github_token_here

# Claude API Key (Anthropic)
# Get from: https://console.anthropic.com/
CLAUDE_API_KEY=your_claude_api_key_here

# Optional: Claude model to use (default: claude-sonnet-4-20250514)
# Available models:
#   claude-3-5-haiku-20241022     - Fast and cost-effective
#   claude-3-5-sonnet-20241022    - Balanced performance and cost
#   claude-3-5-sonnet-20250106    - Enhanced capabilities with recent improvements
#   claude-3-7-haiku-20250109     - Fast and efficient with improved reasoning
#   claude-3-7-sonnet-20250109    - Enhanced capabilities and better performance
#   claude-sonnet-4-20250514      - Most capable, best for complex analysis (recommended)
# CLAUDE_MODEL=claude-sonnet-4-20250514

# Optional: Webhook secret for server mode
# WEBHOOK_SECRET=your_webhook_secret_here
`

	return os.WriteFile(envFile, []byte(content), 0644)
}
