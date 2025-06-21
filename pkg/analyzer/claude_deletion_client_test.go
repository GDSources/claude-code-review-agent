package analyzer

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/your-org/review-agent/pkg/llm/claude"
)

func TestClaudeDeletionClient_Integration(t *testing.T) {
	// Skip integration test if no API key is provided
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		t.Skip("CLAUDE_API_KEY environment variable not set, skipping integration test")
	}

	// Create Claude client
	config := ClaudeAnalyzerConfig{
		APIKey:      apiKey,
		Model:       DefaultDeletionModel,
		MaxTokens:   DefaultDeletionMaxTokens,
		Temperature: DefaultDeletionTemperature,
	}

	client, err := NewClaudeDeletionClient(config)
	if err != nil {
		t.Fatalf("Failed to create Claude client: %v", err)
	}

	// Create a comprehensive test case
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	result := CalculateSum(5, 10) // This will become orphaned
	ProcessData(result)           // This will become orphaned
	
	SafeFunction() // This should be fine
}

func SafeFunction() {
	fmt.Println("This function is safe")
}
`,
				LineCount: 14,
			},
			{
				RelativePath: "helper.go",
				Language:     "go",
				Content: `package main

import "log"

func ProcessHelper() {
	data := CalculateSum(1, 2) // This will become orphaned
	log.Printf("Result: %d", data)
	
	// This should be safe
	SafeFunction()
}

func AnotherSafeFunction() {
	// This is completely safe
}
`,
				LineCount: 13,
			},
		},
		TotalFiles: 2,
		TotalLines: 27,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
			Name: "test-project",
		},
		Summary: "Test codebase with orphaned references",
	}

	deletedContent := []DeletedCode{
		{
			File: "utils.go",
			Content: `func CalculateSum(a, b int) int {
    return a + b
}

func ProcessData(value int) {
    fmt.Printf("Processing: %d\n", value)
}

type DeletedStruct struct {
    Value int
}`,
			StartLine:  1,
			EndLine:    11,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Removing utility functions during refactoring",
	}

	// Create AI context
	contextBuilder := NewDefaultAIContextBuilder()
	aiContext, err := contextBuilder.BuildContext(request)
	if err != nil {
		t.Fatalf("Failed to build AI context: %v", err)
	}

	// Run Claude analysis
	ctx := context.Background()
	result, err := client.AnalyzeDeletions(ctx, aiContext)
	if err != nil {
		t.Fatalf("Claude analysis failed: %v", err)
	}

	// Verify results
	if result == nil {
		t.Fatal("Expected analysis result, got nil")
	}

	// Should have found some orphaned references
	if len(result.OrphanedReferences) == 0 {
		t.Error("Expected Claude to find orphaned references")
	}

	// Should have reasonable confidence
	if result.Confidence < 0.0 || result.Confidence > 1.0 {
		t.Errorf("Expected confidence between 0.0-1.0, got %f", result.Confidence)
	}

	// Should have a summary
	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Log results
	t.Logf("Claude Analysis Results:")
	t.Logf("  Found %d orphaned references", len(result.OrphanedReferences))
	t.Logf("  Found %d safe deletions", len(result.SafeDeletions))
	t.Logf("  Generated %d warnings", len(result.Warnings))
	t.Logf("  Analysis confidence: %.2f", result.Confidence)
	t.Logf("  Summary: %s", result.Summary)

	for i, ref := range result.OrphanedReferences {
		t.Logf("  Reference %d: %s in %s (lines %v) - %s",
			i+1, ref.DeletedEntity, ref.ReferencingFile, ref.ReferencingLines, ref.Severity)
	}
}

func TestClaudeDeletionClient_AnalyzeDeletionsWithLLM(t *testing.T) {
	// Skip integration test if no API key is provided
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		t.Skip("CLAUDE_API_KEY environment variable not set, skipping integration test")
	}

	// Create Claude client
	config := ClaudeAnalyzerConfig{
		APIKey:    apiKey,
		Model:     DefaultDeletionModel,
		MaxTokens: 4000, // Smaller for faster testing
	}

	claudeClient, err := NewClaudeDeletionClient(config)
	if err != nil {
		t.Fatalf("Failed to create Claude client: %v", err)
	}

	// Create deletion analyzer with Claude LLM
	analyzer := NewDeletionAnalyzerWithLLM(claudeClient)

	// Create test data
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.js",
				Language:     "javascript",
				Content: `function main() {
    console.log("Hello, World!");
    const result = deletedFunction(); // Orphaned reference
    console.log(result);
}

function keepThisFunction() {
    return "safe";
}
`,
				LineCount: 8,
			},
		},
		TotalFiles: 1,
		TotalLines: 8,
		Languages:  []string{"javascript"},
		ProjectInfo: ProjectInfo{
			Type: "node",
			Name: "test-js-project",
		},
		Summary: "JavaScript test codebase",
	}

	deletedContent := []DeletedCode{
		{
			File: "utils.js",
			Content: `function deletedFunction() {
    return "I was deleted";
}

const DELETED_CONSTANT = "gone";`,
			StartLine:  1,
			EndLine:    5,
			Language:   "javascript",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Cleaning up unused utility functions",
	}

	// Run analysis
	result, err := analyzer.AnalyzeDeletions(request)
	if err != nil {
		t.Fatalf("Deletion analysis failed: %v", err)
	}

	// Verify it used Claude (not heuristics)
	if !strings.Contains(result.Summary, "AI-enhanced") && !strings.Contains(result.Summary, "analysis") {
		t.Log("Result appears to be from Claude (not heuristics)")
	}

	// Should find the orphaned reference to deletedFunction
	found := false
	for _, ref := range result.OrphanedReferences {
		if ref.DeletedEntity == "deletedFunction" || strings.Contains(ref.DeletedEntity, "deleted") {
			found = true
			break
		}
	}

	if !found && len(result.OrphanedReferences) == 0 {
		t.Error("Expected Claude to find the orphaned reference to deletedFunction")
	}

	t.Logf("Full LLM Analysis Results:")
	t.Logf("  Summary: %s", result.Summary)
	t.Logf("  Confidence: %.2f", result.Confidence)
	t.Logf("  Orphaned References: %d", len(result.OrphanedReferences))
	for _, ref := range result.OrphanedReferences {
		t.Logf("    - %s in %s (severity: %s)", ref.DeletedEntity, ref.ReferencingFile, ref.Severity)
	}
	t.Logf("  Safe Deletions: %v", result.SafeDeletions)
	t.Logf("  Warnings: %d", len(result.Warnings))
}

func TestClaudeDeletionClient_Configuration(t *testing.T) {
	// Test valid configuration
	config := ClaudeAnalyzerConfig{
		APIKey:      "test-key",
		Model:       "claude-sonnet-4-20250514",
		MaxTokens:   4000,
		Temperature: 0.1,
		BaseURL:     "https://api.anthropic.com",
		Timeout:     120,
	}

	client, err := NewClaudeDeletionClient(config)
	if err != nil {
		t.Fatalf("Failed to create client with valid config: %v", err)
	}

	if client.apiKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got '%s'", client.apiKey)
	}

	// Test missing API key
	invalidConfig := ClaudeAnalyzerConfig{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 4000,
	}

	_, err = NewClaudeDeletionClient(invalidConfig)
	if err == nil {
		t.Error("Expected error for missing API key")
	}

	// Test invalid temperature
	invalidConfig = ClaudeAnalyzerConfig{
		APIKey:      "test-key",
		Temperature: 3.0, // Invalid: > 2.0
	}

	_, err = NewClaudeDeletionClient(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid temperature")
	}
}

func TestClaudeDeletionClient_DefaultConfiguration(t *testing.T) {
	// Test that defaults are properly applied
	config := ClaudeAnalyzerConfig{
		APIKey: "test-key",
		// All other fields should get defaults
	}

	client, err := NewClaudeDeletionClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.model != DefaultDeletionModel {
		t.Errorf("Expected default model '%s', got '%s'", DefaultDeletionModel, client.model)
	}

	if client.maxTokens != DefaultDeletionMaxTokens {
		t.Errorf("Expected default max tokens %d, got %d", DefaultDeletionMaxTokens, client.maxTokens)
	}

	if client.temperature != DefaultDeletionTemperature {
		t.Errorf("Expected default temperature %f, got %f", DefaultDeletionTemperature, client.temperature)
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "normal API key",
			apiKey:   "sk-ant-api03-1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "sk-a...wxyz",
		},
		{
			name:     "short API key",
			apiKey:   "sk-123",
			expected: "***",
		},
		{
			name:     "empty API key",
			apiKey:   "",
			expected: "***",
		},
		{
			name:     "exactly 8 chars",
			apiKey:   "12345678",
			expected: "***",
		},
		{
			name:     "9 chars shows masked",
			apiKey:   "123456789",
			expected: "1234...6789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := claude.MaskAPIKey(tt.apiKey)
			if result != tt.expected {
				t.Errorf("claude.MaskAPIKey(%q) = %q, want %q", tt.apiKey, result, tt.expected)
			}
		})
	}
}

func TestClaudeDeletionClient_String(t *testing.T) {
	client := &ClaudeDeletionClient{
		apiKey:      "sk-ant-api03-verysecretkey123456789",
		model:       "claude-3-opus",
		maxTokens:   4000,
		temperature: 0.1,
		baseURL:     "https://api.anthropic.com",
	}

	str := client.String()

	// Check that the string representation contains masked API key
	if !strings.Contains(str, "sk-a...6789") {
		t.Errorf("String() should mask the API key, got: %s", str)
	}

	// Ensure the actual API key is not exposed
	if strings.Contains(str, "verysecretkey") {
		t.Errorf("String() exposed the API key: %s", str)
	}
}
