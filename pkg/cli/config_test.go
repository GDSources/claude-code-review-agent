package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewEnvLoader(t *testing.T) {
	loader := NewEnvLoader()

	if loader == nil {
		t.Fatal("expected EnvLoader to be created")
	}

	if len(loader.searchPaths) == 0 {
		t.Error("expected search paths to be populated")
	}

	// Verify search paths contain expected directories
	currentDir, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()
	expectedPaths := []string{
		currentDir,
		homeDir,
		filepath.Join(homeDir, ".config", "review-agent"),
	}

	for i, expectedPath := range expectedPaths {
		if i >= len(loader.searchPaths) {
			t.Errorf("missing expected path: %s", expectedPath)
			continue
		}
		if loader.searchPaths[i] != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, loader.searchPaths[i])
		}
	}
}

func TestEnvLoader_LoadEnvFile_FileNotFound(t *testing.T) {
	// Create a temporary directory that doesn't contain .env file
	tempDir := t.TempDir()

	loader := &EnvLoader{
		searchPaths: []string{tempDir},
	}

	// Should not error when no .env file is found
	err := loader.LoadEnvFile()
	if err != nil {
		t.Errorf("expected no error when .env file not found, got: %v", err)
	}
}

func TestEnvLoader_LoadEnvFile_Success(t *testing.T) {
	// Create temporary directory with .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")

	envContent := `GH_TOKEN=test-github-token
CLAUDE_API_KEY=test-claude-key
# This is a comment
QUOTED_VALUE="quoted value"
SINGLE_QUOTED='single quoted'
EMPTY_VALUE=
EQUALS_IN_VALUE=key=value=more
`

	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Save original env vars
	originalVars := map[string]string{
		"GH_TOKEN":        os.Getenv("GH_TOKEN"),
		"CLAUDE_API_KEY":  os.Getenv("CLAUDE_API_KEY"),
		"QUOTED_VALUE":    os.Getenv("QUOTED_VALUE"),
		"SINGLE_QUOTED":   os.Getenv("SINGLE_QUOTED"),
		"EMPTY_VALUE":     os.Getenv("EMPTY_VALUE"),
		"EQUALS_IN_VALUE": os.Getenv("EQUALS_IN_VALUE"),
	}

	// Clear env vars for test
	for key := range originalVars {
		os.Unsetenv(key)
	}

	// Restore original env vars after test
	defer func() {
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	loader := &EnvLoader{
		searchPaths: []string{tempDir},
	}

	err = loader.LoadEnvFile()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify environment variables were set
	tests := []struct {
		key      string
		expected string
	}{
		{"GH_TOKEN", "test-github-token"},
		{"CLAUDE_API_KEY", "test-claude-key"},
		{"QUOTED_VALUE", "quoted value"},
		{"SINGLE_QUOTED", "single quoted"},
		{"EMPTY_VALUE", ""},
		{"EQUALS_IN_VALUE", "key=value=more"},
	}

	for _, tt := range tests {
		actual := os.Getenv(tt.key)
		if actual != tt.expected {
			t.Errorf("expected %s=%s, got %s", tt.key, tt.expected, actual)
		}
	}
}

func TestEnvLoader_LoadEnvFile_PrecedenceRespected(t *testing.T) {
	// Create temporary directory with .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")

	envContent := `GH_TOKEN=env-file-token
CLAUDE_API_KEY=env-file-key
`

	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Set environment variables (should take precedence)
	originalGitHub := os.Getenv("GH_TOKEN")
	originalClaude := os.Getenv("CLAUDE_API_KEY")

	os.Setenv("GH_TOKEN", "env-var-token")
	// Leave CLAUDE_API_KEY unset to test .env file fallback
	os.Unsetenv("CLAUDE_API_KEY")

	defer func() {
		if originalGitHub != "" {
			os.Setenv("GH_TOKEN", originalGitHub)
		} else {
			os.Unsetenv("GH_TOKEN")
		}
		if originalClaude != "" {
			os.Setenv("CLAUDE_API_KEY", originalClaude)
		} else {
			os.Unsetenv("CLAUDE_API_KEY")
		}
	}()

	loader := &EnvLoader{
		searchPaths: []string{tempDir},
	}

	err = loader.LoadEnvFile()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Environment variable should be preserved
	if os.Getenv("GH_TOKEN") != "env-var-token" {
		t.Errorf("expected GH_TOKEN=env-var-token, got %s", os.Getenv("GH_TOKEN"))
	}

	// .env file value should be used when env var is not set
	if os.Getenv("CLAUDE_API_KEY") != "env-file-key" {
		t.Errorf("expected CLAUDE_API_KEY=env-file-key, got %s", os.Getenv("CLAUDE_API_KEY"))
	}
}

func TestEnvLoader_LoadEnvFile_MalformedFile(t *testing.T) {
	// Create temporary directory with malformed .env file
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")

	envContent := `VALID_KEY=valid_value
INVALID_LINE_NO_EQUALS
ANOTHER_VALID=value
`

	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	loader := &EnvLoader{
		searchPaths: []string{tempDir},
	}

	err = loader.LoadEnvFile()
	if err == nil {
		t.Error("expected error for malformed .env file")
		return
	}

	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected error message to contain 'invalid format', got: %v", err)
	}
}

func TestEnvLoader_LoadEnvFile_MultipleSearchPaths(t *testing.T) {
	// Create multiple temporary directories
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()
	tempDir3 := t.TempDir()

	// Only create .env file in the second directory
	envFile2 := filepath.Join(tempDir2, ".env")
	envContent := `SEARCH_PATH_TEST=found-in-second-path`

	err := os.WriteFile(envFile2, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}

	// Save and clear environment variable
	original := os.Getenv("SEARCH_PATH_TEST")
	os.Unsetenv("SEARCH_PATH_TEST")
	defer func() {
		if original != "" {
			os.Setenv("SEARCH_PATH_TEST", original)
		} else {
			os.Unsetenv("SEARCH_PATH_TEST")
		}
	}()

	loader := &EnvLoader{
		searchPaths: []string{tempDir1, tempDir2, tempDir3},
	}

	err = loader.LoadEnvFile()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should find the .env file in the second directory
	if os.Getenv("SEARCH_PATH_TEST") != "found-in-second-path" {
		t.Errorf("expected SEARCH_PATH_TEST=found-in-second-path, got %s", os.Getenv("SEARCH_PATH_TEST"))
	}
}

func TestEnvLoader_CreateSampleEnvFile(t *testing.T) {
	// Change to temporary directory
	originalDir, _ := os.Getwd()
	tempDir := t.TempDir()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(originalDir) }()

	loader := NewEnvLoader()

	err := loader.CreateSampleEnvFile()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify .env.example file was created
	expectedFile := filepath.Join(tempDir, ".env.example")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Error("expected .env.example file to be created")
	}

	// Read and verify content
	content, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Errorf("failed to read .env.example file: %v", err)
	}

	contentStr := string(content)

	// Verify expected keys are present
	expectedKeys := []string{
		"GH_TOKEN=",
		"CLAUDE_API_KEY=",
		"WEBHOOK_SECRET=",
	}

	for _, key := range expectedKeys {
		if !strings.Contains(contentStr, key) {
			t.Errorf("expected .env.example to contain '%s'", key)
		}
	}

	// Verify comments are present
	if !strings.Contains(contentStr, "# Review Agent Configuration") {
		t.Error("expected .env.example to contain configuration comment")
	}

	if !strings.Contains(contentStr, "# GitHub Personal Access Token") {
		t.Error("expected .env.example to contain GitHub token comment")
	}
}

func TestEnvLoader_ParseLine(t *testing.T) {
	loader := NewEnvLoader()

	tests := []struct {
		name        string
		line        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid key-value pair",
			line:        "KEY=value",
			expectError: false,
		},
		{
			name:        "key with spaces",
			line:        "KEY_WITH_SPACES = value with spaces",
			expectError: false,
		},
		{
			name:        "quoted value",
			line:        "QUOTED=\"quoted value\"",
			expectError: false,
		},
		{
			name:        "single quoted value",
			line:        "SINGLE_QUOTED='single quoted'",
			expectError: false,
		},
		{
			name:        "value with equals sign",
			line:        "URL=https://example.com?param=value",
			expectError: false,
		},
		{
			name:        "empty value",
			line:        "EMPTY_KEY=",
			expectError: false,
		},
		{
			name:        "no equals sign",
			line:        "INVALID_LINE",
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name:        "only equals sign",
			line:        "=value",
			expectError: false, // Empty key is technically valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.parseLine(tt.line, "test.env", 1)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.errorMsg != "" && (err == nil || !strings.Contains(err.Error(), tt.errorMsg)) {
				t.Errorf("expected error to contain '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}
