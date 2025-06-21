package analyzer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultAIContextBuilder_BuildContext(t *testing.T) {
	// Create a sample deletion analysis request
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	DeletedFunction() // This will become orphaned
}

func KeepThisFunction() {
	// This function should be safe
}
`,
				LineCount: 11,
			},
			{
				RelativePath: "helper.go",
				Language:     "go",
				Content: `package main

func HelperFunction() {
	// This references deleted code
	UtilityFunction("helper") // This will become orphaned
}

func AnotherFunction() {
	// This is safe
}
`,
				LineCount: 9,
			},
		},
		TotalFiles: 2,
		TotalLines: 20,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
			Name: "test-project",
		},
		Summary: "Test codebase with 2 Go files",
	}

	deletedContent := []DeletedCode{
		{
			File: "utils.go",
			Content: `func DeletedFunction() {
    fmt.Println("This function was deleted")
}

func UtilityFunction(param string) string {
    return "processed: " + param
}`,
			StartLine:  1,
			EndLine:    7,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Removed utility functions during refactoring",
	}

	// Build context
	builder := NewDefaultAIContextBuilder()
	context, err := builder.BuildContext(request)
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	// Test that context was created
	if context == nil {
		t.Fatal("Expected context to be created, got nil")
	}

	// Test system prompt
	if context.SystemPrompt == "" {
		t.Error("Expected non-empty system prompt")
	}

	// Test user prompt
	if context.UserPrompt == "" {
		t.Error("Expected non-empty user prompt")
	}

	// Test codebase context contains file information
	if context.CodebaseContext == "" {
		t.Error("Expected non-empty codebase context")
	}

	// Verify codebase context contains file names
	if !strings.Contains(context.CodebaseContext, "main.go") {
		t.Error("Expected codebase context to contain main.go")
	}
	if !strings.Contains(context.CodebaseContext, "helper.go") {
		t.Error("Expected codebase context to contain helper.go")
	}

	// Test deletion context
	if context.DeletionContext == "" {
		t.Error("Expected non-empty deletion context")
	}

	// Verify deletion context contains deleted function names
	if !strings.Contains(context.DeletionContext, "DeletedFunction") {
		t.Error("Expected deletion context to contain DeletedFunction")
	}
	if !strings.Contains(context.DeletionContext, "UtilityFunction") {
		t.Error("Expected deletion context to contain UtilityFunction")
	}

	// Test instructions
	if context.Instructions == "" {
		t.Error("Expected non-empty instructions")
	}

	// Test expected format is valid JSON structure
	if context.ExpectedFormat == nil {
		t.Error("Expected non-empty expected format")
	}

	// Verify expected format can be marshaled to JSON
	_, err = json.Marshal(context.ExpectedFormat)
	if err != nil {
		t.Errorf("Expected format should be valid JSON structure: %v", err)
	}

	// Test that expected format contains required fields
	if _, exists := context.ExpectedFormat["orphaned_references"]; !exists {
		t.Error("Expected format should contain orphaned_references field")
	}

	t.Logf("System prompt length: %d characters", len(context.SystemPrompt))
	t.Logf("User prompt length: %d characters", len(context.UserPrompt))
	t.Logf("Codebase context length: %d characters", len(context.CodebaseContext))
	t.Logf("Deletion context length: %d characters", len(context.DeletionContext))
}

func TestDefaultAIContextBuilder_FormatCodebaseContext(t *testing.T) {
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content:      "package main\n\nfunc main() {}\n",
				LineCount:    3,
				Size:         25,
			},
			{
				RelativePath: "utils.go",
				Language:     "go",
				Content:      "package main\n\nfunc UtilFunction() {}\n",
				LineCount:    3,
				Size:         30,
			},
		},
		TotalFiles: 2,
		TotalLines: 6,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
			Name: "test-project",
		},
		Summary: "Test codebase",
	}

	builder := NewDefaultAIContextBuilder()
	context := builder.formatCodebaseContext(codebase)

	// Test that context is properly formatted
	if context == "" {
		t.Error("Expected non-empty formatted context")
	}

	// Test that project info is included
	if !strings.Contains(context, "go") {
		t.Error("Expected context to contain project type")
	}

	// Test that file information is included
	if !strings.Contains(context, "main.go") {
		t.Error("Expected context to contain main.go")
	}
	if !strings.Contains(context, "utils.go") {
		t.Error("Expected context to contain utils.go")
	}

	// Test that file content is included
	if !strings.Contains(context, "func main()") {
		t.Error("Expected context to contain function content")
	}

	// Test that summary statistics are included
	if !strings.Contains(context, "2") { // Total files
		t.Error("Expected context to contain file count")
	}

	t.Logf("Formatted context:\n%s", context)
}

func TestDefaultAIContextBuilder_FormatDeletionContext(t *testing.T) {
	deletedContent := []DeletedCode{
		{
			File:       "utils.go",
			Content:    "func DeletedFunction() {\n    // deleted code\n}",
			StartLine:  1,
			EndLine:    3,
			Language:   "go",
			ChangeType: "deleted",
		},
		{
			File:       "main.go",
			Content:    "func AnotherDeleted() {}",
			StartLine:  5,
			EndLine:    5,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	builder := NewDefaultAIContextBuilder()
	context := builder.formatDeletionContext(deletedContent, "Refactoring cleanup")

	// Test that context is properly formatted
	if context == "" {
		t.Error("Expected non-empty deletion context")
	}

	// Test that deletion context includes change context
	if !strings.Contains(context, "Refactoring cleanup") {
		t.Error("Expected context to contain change context")
	}

	// Test that file information is included
	if !strings.Contains(context, "utils.go") {
		t.Error("Expected context to contain utils.go")
	}
	if !strings.Contains(context, "main.go") {
		t.Error("Expected context to contain main.go")
	}

	// Test that deleted content is included
	if !strings.Contains(context, "DeletedFunction") {
		t.Error("Expected context to contain DeletedFunction")
	}
	if !strings.Contains(context, "AnotherDeleted") {
		t.Error("Expected context to contain AnotherDeleted")
	}

	// Test that line information is included
	if !strings.Contains(context, "1-3") {
		t.Error("Expected context to contain line range")
	}

	t.Logf("Deletion context:\n%s", context)
}

func TestDefaultAIContextBuilder_CreateExpectedFormat(t *testing.T) {
	builder := NewDefaultAIContextBuilder()
	format := builder.createExpectedFormat()

	// Test that format is not nil
	if format == nil {
		t.Fatal("Expected format to be created, got nil")
	}

	// Test that required fields are present
	requiredFields := []string{
		"orphaned_references",
		"safe_deletions",
		"warnings",
		"summary",
		"confidence",
	}

	for _, field := range requiredFields {
		if _, exists := format[field]; !exists {
			t.Errorf("Expected format to contain field '%s'", field)
		}
	}

	// Test that orphaned_references has the correct structure
	if orphanedRefs, ok := format["orphaned_references"].([]map[string]interface{}); ok {
		if len(orphanedRefs) > 0 {
			ref := orphanedRefs[0]
			expectedRefFields := []string{
				"deleted_entity",
				"referencing_file",
				"referencing_lines",
				"reference_type",
				"context",
				"severity",
				"suggestion",
			}

			for _, field := range expectedRefFields {
				if _, exists := ref[field]; !exists {
					t.Errorf("Expected orphaned reference to contain field '%s'", field)
				}
			}
		}
	} else {
		t.Error("Expected orphaned_references to be an array of objects")
	}

	// Test that format can be marshaled to JSON
	jsonData, err := json.MarshalIndent(format, "", "  ")
	if err != nil {
		t.Errorf("Expected format should be valid JSON: %v", err)
	}

	t.Logf("Expected format JSON:\n%s", string(jsonData))
}

func TestDefaultAIContextBuilder_EmptyCodebase(t *testing.T) {
	// Test with empty codebase
	request := &DeletionAnalysisRequest{
		Codebase: &FlattenedCodebase{
			Files:      []FileContent{},
			TotalFiles: 0,
			TotalLines: 0,
			Languages:  []string{},
		},
		DeletedContent: []DeletedCode{},
		Context:        "Empty test",
	}

	builder := NewDefaultAIContextBuilder()
	context, err := builder.BuildContext(request)
	if err != nil {
		t.Fatalf("BuildContext should handle empty codebase: %v", err)
	}

	if context == nil {
		t.Fatal("Expected context even for empty codebase")
	}

	// Should still have system prompt and instructions
	if context.SystemPrompt == "" {
		t.Error("Expected system prompt even for empty codebase")
	}
	if context.Instructions == "" {
		t.Error("Expected instructions even for empty codebase")
	}
}
