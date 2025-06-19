package main

import (
	"os"
	"testing"
)

func TestLoadEnvConfig(t *testing.T) {
	// Save original env vars
	originalGitHub := os.Getenv("GITHUB_TOKEN")
	originalClaude := os.Getenv("CLAUDE_API_KEY")

	// Clean up after test
	defer func() {
		os.Setenv("GITHUB_TOKEN", originalGitHub)
		os.Setenv("CLAUDE_API_KEY", originalClaude)
	}()

	tests := []struct {
		name           string
		config         *Config
		envVars        map[string]string
		expectedGitHub string
		expectedClaude string
	}{
		{
			name:   "load from environment variables",
			config: &Config{},
			envVars: map[string]string{
				"GITHUB_TOKEN":   "env-github-token",
				"CLAUDE_API_KEY": "env-claude-key",
			},
			expectedGitHub: "env-github-token",
			expectedClaude: "env-claude-key",
		},
		{
			name: "flags take precedence over env vars",
			config: &Config{
				GitHubToken:  "flag-github-token",
				ClaudeAPIKey: "flag-claude-key",
			},
			envVars: map[string]string{
				"GITHUB_TOKEN":   "env-github-token",
				"CLAUDE_API_KEY": "env-claude-key",
			},
			expectedGitHub: "flag-github-token",
			expectedClaude: "flag-claude-key",
		},
		{
			name: "partial flags with env fallback",
			config: &Config{
				GitHubToken: "flag-github-token",
			},
			envVars: map[string]string{
				"CLAUDE_API_KEY": "env-claude-key",
			},
			expectedGitHub: "flag-github-token",
			expectedClaude: "env-claude-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			err := loadEnvConfig(tt.config)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.config.GitHubToken != tt.expectedGitHub {
				t.Errorf("expected GitHub token '%s', got '%s'", tt.expectedGitHub, tt.config.GitHubToken)
			}

			if tt.config.ClaudeAPIKey != tt.expectedClaude {
				t.Errorf("expected Claude key '%s', got '%s'", tt.expectedClaude, tt.config.ClaudeAPIKey)
			}

			// Clean up env vars for next test
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestValidateReviewConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		owner         string
		repo          string
		prNumber      int
		expectError   bool
		errorContains string
	}{
		{
			name: "valid configuration",
			config: &Config{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
			},
			owner:       "testowner",
			repo:        "testrepo",
			prNumber:    123,
			expectError: false,
		},
		{
			name: "missing GitHub token",
			config: &Config{
				ClaudeAPIKey: "valid-key",
			},
			owner:         "testowner",
			repo:          "testrepo",
			prNumber:      123,
			expectError:   true,
			errorContains: "GitHub token is required",
		},
		{
			name: "missing Claude API key",
			config: &Config{
				GitHubToken: "valid-token",
			},
			owner:         "testowner",
			repo:          "testrepo",
			prNumber:      123,
			expectError:   true,
			errorContains: "Claude API key is required",
		},
		{
			name: "missing owner",
			config: &Config{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
			},
			owner:         "",
			repo:          "testrepo",
			prNumber:      123,
			expectError:   true,
			errorContains: "repository owner is required",
		},
		{
			name: "missing repo",
			config: &Config{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
			},
			owner:         "testowner",
			repo:          "",
			prNumber:      123,
			expectError:   true,
			errorContains: "repository name is required",
		},
		{
			name: "invalid PR number - zero",
			config: &Config{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
			},
			owner:         "testowner",
			repo:          "testrepo",
			prNumber:      0,
			expectError:   true,
			errorContains: "valid pull request number is required",
		},
		{
			name: "invalid PR number - negative",
			config: &Config{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
			},
			owner:         "testowner",
			repo:          "testrepo",
			prNumber:      -5,
			expectError:   true,
			errorContains: "valid pull request number is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReviewConfig(tt.config, tt.owner, tt.repo, tt.prNumber)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || err.Error()[:len(tt.errorContains)] != tt.errorContains) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}
		})
	}
}

func TestConfigStruct(t *testing.T) {
	config := &Config{
		GitHubToken:  "test-token",
		ClaudeAPIKey: "test-key",
	}

	if config.GitHubToken != "test-token" {
		t.Errorf("expected GitHubToken 'test-token', got '%s'", config.GitHubToken)
	}

	if config.ClaudeAPIKey != "test-key" {
		t.Errorf("expected ClaudeAPIKey 'test-key', got '%s'", config.ClaudeAPIKey)
	}
}
