package main

import (
	"os"
	"path/filepath"
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

func TestLoadEnvConfig_WithEnvFile(t *testing.T) {
	// Create temporary directory and .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")
	
	envContent := `GITHUB_TOKEN=env-file-github-token
CLAUDE_API_KEY=env-file-claude-key
`
	
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}
	
	// Save original working directory and environment variables
	originalDir, _ := os.Getwd()
	originalGitHub := os.Getenv("GITHUB_TOKEN")
	originalClaude := os.Getenv("CLAUDE_API_KEY")
	
	// Change to temp directory and clear env vars
	os.Chdir(tempDir)
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("CLAUDE_API_KEY")
	
	// Restore after test
	defer func() {
		os.Chdir(originalDir)
		if originalGitHub != "" {
			os.Setenv("GITHUB_TOKEN", originalGitHub)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
		if originalClaude != "" {
			os.Setenv("CLAUDE_API_KEY", originalClaude)
		} else {
			os.Unsetenv("CLAUDE_API_KEY")
		}
	}()
	
	config := &Config{}
	err = loadEnvConfig(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	
	if config.GitHubToken != "env-file-github-token" {
		t.Errorf("expected GitHub token from .env file 'env-file-github-token', got '%s'", config.GitHubToken)
	}
	
	if config.ClaudeAPIKey != "env-file-claude-key" {
		t.Errorf("expected Claude key from .env file 'env-file-claude-key', got '%s'", config.ClaudeAPIKey)
	}
}

func TestLoadEnvConfig_PrecedenceOrder(t *testing.T) {
	// Create temporary directory and .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")
	
	envContent := `GITHUB_TOKEN=env-file-token
CLAUDE_API_KEY=env-file-key
`
	
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}
	
	// Save original working directory and environment variables
	originalDir, _ := os.Getwd()
	originalGitHub := os.Getenv("GITHUB_TOKEN")
	originalClaude := os.Getenv("CLAUDE_API_KEY")
	
	// Change to temp directory
	os.Chdir(tempDir)
	
	// Set environment variables (should take precedence over .env file)
	os.Setenv("GITHUB_TOKEN", "env-var-token")
	os.Unsetenv("CLAUDE_API_KEY") // Let this come from .env file
	
	// Restore after test
	defer func() {
		os.Chdir(originalDir)
		if originalGitHub != "" {
			os.Setenv("GITHUB_TOKEN", originalGitHub)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
		if originalClaude != "" {
			os.Setenv("CLAUDE_API_KEY", originalClaude)
		} else {
			os.Unsetenv("CLAUDE_API_KEY")
		}
	}()
	
	// Test with flags taking highest precedence
	config := &Config{
		GitHubToken: "flag-token", // Should override both env var and .env file
		// ClaudeAPIKey left empty to test env var vs .env file precedence
	}
	
	err = loadEnvConfig(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	
	// Flag takes precedence over everything
	if config.GitHubToken != "flag-token" {
		t.Errorf("expected flag token 'flag-token', got '%s'", config.GitHubToken)
	}
	
	// .env file value used when no flag or env var
	if config.ClaudeAPIKey != "env-file-key" {
		t.Errorf("expected .env file key 'env-file-key', got '%s'", config.ClaudeAPIKey)
	}
}

func TestLoadServerConfig(t *testing.T) {
	// Create temporary directory and .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")
	
	envContent := `GITHUB_TOKEN=env-file-github-token
CLAUDE_API_KEY=env-file-claude-key
WEBHOOK_SECRET=env-file-webhook-secret
PORT=3000
`
	
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}
	
	// Save original working directory and environment variables
	originalDir, _ := os.Getwd()
	originalGitHub := os.Getenv("GITHUB_TOKEN")
	originalClaude := os.Getenv("CLAUDE_API_KEY")
	originalWebhook := os.Getenv("WEBHOOK_SECRET")
	originalPort := os.Getenv("PORT")
	
	// Change to temp directory and clear env vars
	os.Chdir(tempDir)
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("CLAUDE_API_KEY")
	os.Unsetenv("WEBHOOK_SECRET")
	os.Unsetenv("PORT")
	
	// Restore after test
	defer func() {
		os.Chdir(originalDir)
		restoreEnvVar("GITHUB_TOKEN", originalGitHub)
		restoreEnvVar("CLAUDE_API_KEY", originalClaude)
		restoreEnvVar("WEBHOOK_SECRET", originalWebhook)
		restoreEnvVar("PORT", originalPort)
	}()
	
	config := &ServerConfig{Port: 8080} // Default port
	err = loadServerConfig(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	
	if config.GitHubToken != "env-file-github-token" {
		t.Errorf("expected GitHub token from .env file 'env-file-github-token', got '%s'", config.GitHubToken)
	}
	
	if config.ClaudeAPIKey != "env-file-claude-key" {
		t.Errorf("expected Claude key from .env file 'env-file-claude-key', got '%s'", config.ClaudeAPIKey)
	}
	
	if config.WebhookSecret != "env-file-webhook-secret" {
		t.Errorf("expected webhook secret from .env file 'env-file-webhook-secret', got '%s'", config.WebhookSecret)
	}
	
	if config.Port != 3000 {
		t.Errorf("expected port from .env file 3000, got %d", config.Port)
	}
}

func TestValidateServerConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        *ServerConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid configuration",
			config: &ServerConfig{
				GitHubToken:   "valid-token",
				ClaudeAPIKey:  "valid-key",
				WebhookSecret: "valid-secret",
				Port:          8080,
			},
			expectError: false,
		},
		{
			name: "missing GitHub token",
			config: &ServerConfig{
				ClaudeAPIKey:  "valid-key",
				WebhookSecret: "valid-secret",
				Port:          8080,
			},
			expectError:   true,
			errorContains: "GitHub token is required",
		},
		{
			name: "missing Claude API key",
			config: &ServerConfig{
				GitHubToken:   "valid-token",
				WebhookSecret: "valid-secret",
				Port:          8080,
			},
			expectError:   true,
			errorContains: "Claude API key is required",
		},
		{
			name: "missing webhook secret",
			config: &ServerConfig{
				GitHubToken:  "valid-token",
				ClaudeAPIKey: "valid-key",
				Port:         8080,
			},
			expectError:   true,
			errorContains: "webhook secret is required",
		},
		{
			name: "invalid port - zero",
			config: &ServerConfig{
				GitHubToken:   "valid-token",
				ClaudeAPIKey:  "valid-key",
				WebhookSecret: "valid-secret",
				Port:          0,
			},
			expectError:   true,
			errorContains: "invalid port number",
		},
		{
			name: "invalid port - too high",
			config: &ServerConfig{
				GitHubToken:   "valid-token",
				ClaudeAPIKey:  "valid-key",
				WebhookSecret: "valid-secret",
				Port:          70000,
			},
			expectError:   true,
			errorContains: "invalid port number",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerConfig(tt.config)
			
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.errorContains != "" && (err == nil || !containsString(err.Error(), tt.errorContains)) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
			}
		})
	}
}

func TestServerConfigStruct(t *testing.T) {
	config := &ServerConfig{
		GitHubToken:   "test-github-token",
		ClaudeAPIKey:  "test-claude-key",
		WebhookSecret: "test-webhook-secret",
		Port:          9000,
	}
	
	if config.GitHubToken != "test-github-token" {
		t.Errorf("expected GitHubToken 'test-github-token', got '%s'", config.GitHubToken)
	}
	
	if config.ClaudeAPIKey != "test-claude-key" {
		t.Errorf("expected ClaudeAPIKey 'test-claude-key', got '%s'", config.ClaudeAPIKey)
	}
	
	if config.WebhookSecret != "test-webhook-secret" {
		t.Errorf("expected WebhookSecret 'test-webhook-secret', got '%s'", config.WebhookSecret)
	}
	
	if config.Port != 9000 {
		t.Errorf("expected Port 9000, got %d", config.Port)
	}
}

// Helper functions for tests
func restoreEnvVar(key, value string) {
	if value != "" {
		os.Setenv(key, value)
	} else {
		os.Unsetenv(key)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
