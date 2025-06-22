package analyzer

import (
	"testing"
)

func TestExtractDeletedContent(t *testing.T) {
	// Test extracting deleted content from a diff
	diff := &ParsedDiff{
		Files: []FileDiff{
			{
				Filename: "main.go",
				Status:   "modified",
				Language: "go",
				Hunks: []DiffHunk{
					{
						OldStart: 1,
						OldCount: 10,
						NewStart: 1,
						NewCount: 8,
						Lines: []DiffLine{
							{Type: "context", Content: "package main"},
							{Type: "context", Content: ""},
							{Type: "removed", Content: "func DeletedFunction() {", OldLineNo: 3},
							{Type: "removed", Content: "    fmt.Println(\"deleted\")", OldLineNo: 4},
							{Type: "removed", Content: "}", OldLineNo: 5},
							{Type: "context", Content: ""},
							{Type: "context", Content: "func main() {"},
							{Type: "added", Content: "    // Updated main function"},
							{Type: "context", Content: "    fmt.Println(\"Hello, World!\")"},
							{Type: "context", Content: "}"},
						},
					},
				},
			},
			{
				Filename: "utils.go",
				Status:   "deleted",
				Language: "go",
				Hunks: []DiffHunk{
					{
						OldStart: 1,
						OldCount: 5,
						NewStart: 0,
						NewCount: 0,
						Lines: []DiffLine{
							{Type: "removed", Content: "package main", OldLineNo: 1},
							{Type: "removed", Content: "", OldLineNo: 2},
							{Type: "removed", Content: "func UtilityFunction() {", OldLineNo: 3},
							{Type: "removed", Content: "    // Utility logic", OldLineNo: 4},
							{Type: "removed", Content: "}", OldLineNo: 5},
						},
					},
				},
			},
		},
	}

	deletedContent := extractDeletedContent(diff)

	// Test that we found deleted content
	if len(deletedContent) == 0 {
		t.Fatal("Expected to find deleted content, got none")
	}

	// Test main.go deleted content
	mainDeleted := findDeletedContentByFile(deletedContent, "main.go")
	if mainDeleted == nil {
		t.Fatal("Expected to find deleted content for main.go")
	}

	expectedMainContent := `func DeletedFunction() {
    fmt.Println("deleted")
}`
	if mainDeleted.Content != expectedMainContent {
		t.Errorf("Expected main.go deleted content:\n%s\nGot:\n%s", expectedMainContent, mainDeleted.Content)
	}

	if mainDeleted.StartLine != 3 || mainDeleted.EndLine != 5 {
		t.Errorf("Expected main.go lines 3-5, got %d-%d", mainDeleted.StartLine, mainDeleted.EndLine)
	}

	if mainDeleted.ChangeType != "deleted" {
		t.Errorf("Expected change type 'deleted', got '%s'", mainDeleted.ChangeType)
	}

	// Test utils.go deleted content (entire file)
	utilsDeleted := findDeletedContentByFile(deletedContent, "utils.go")
	if utilsDeleted == nil {
		t.Fatal("Expected to find deleted content for utils.go")
	}

	expectedUtilsContent := `package main

func UtilityFunction() {
    // Utility logic
}`
	if utilsDeleted.Content != expectedUtilsContent {
		t.Errorf("Expected utils.go deleted content:\n%s\nGot:\n%s", expectedUtilsContent, utilsDeleted.Content)
	}

	if utilsDeleted.ChangeType != "deleted" {
		t.Errorf("Expected change type 'deleted', got '%s'", utilsDeleted.ChangeType)
	}

	t.Logf("Found %d pieces of deleted content", len(deletedContent))
	for _, deleted := range deletedContent {
		t.Logf("File: %s, Lines: %d-%d, Type: %s", deleted.File, deleted.StartLine, deleted.EndLine, deleted.ChangeType)
	}
}

func TestCreateDeletionAnalysisRequest(t *testing.T) {
	// Create a sample flattened codebase
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	DeletedFunction() // This will become an orphaned reference
}

func KeepThisFunction() {
	// This function is kept
}
`,
				LineCount: 12,
			},
			{
				RelativePath: "helper.go",
				Language:     "go",
				Content: `package main

func HelperFunction() {
	// This function references deleted code
	UtilityFunction() // This will become an orphaned reference
}
`,
				LineCount: 6,
			},
		},
		TotalFiles: 2,
		TotalLines: 18,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
		},
		Summary: "Test codebase with 2 Go files",
	}

	// Create deleted content
	deletedContent := []DeletedCode{
		{
			File: "deleted.go",
			Content: `func DeletedFunction() {
    fmt.Println("This function was deleted")
}

func UtilityFunction() {
    // This utility was removed
}`,
			StartLine:  1,
			EndLine:    7,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := createDeletionAnalysisRequest(codebase, deletedContent, "Code cleanup and refactoring")

	// Test request structure
	if request.Codebase != codebase {
		t.Error("Expected codebase to be preserved in request")
	}

	if len(request.DeletedContent) != 1 {
		t.Errorf("Expected 1 deleted content entry, got %d", len(request.DeletedContent))
	}

	if request.Context != "Code cleanup and refactoring" {
		t.Errorf("Expected context 'Code cleanup and refactoring', got '%s'", request.Context)
	}

	if request.DeletedContent[0].File != "deleted.go" {
		t.Errorf("Expected deleted file 'deleted.go', got '%s'", request.DeletedContent[0].File)
	}

	t.Logf("Created deletion analysis request with %d files and %d deleted entries",
		len(request.Codebase.Files), len(request.DeletedContent))
}

func TestDeletionAnalyzer_WithAIContextBuilder_Integration(t *testing.T) {
	// Create a comprehensive test case that uses the AI context builder
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
}

func (d DeletedStruct) DeletedMethod() {
    // This method was deleted
}`,
			StartLine:  1,
			EndLine:    15,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Removing utility functions and structs during refactoring",
	}

	// Test that AI context builder integration works
	analyzer := NewDefaultDeletionAnalyzer()

	// Test that we can build AI context successfully
	contextBuilder := NewDefaultAIContextBuilder()
	aiContext, err := contextBuilder.BuildContext(request)
	if err != nil {
		t.Fatalf("Failed to build AI context: %v", err)
	}

	// Verify the AI context contains the expected information
	if aiContext.SystemPrompt == "" {
		t.Error("Expected AI context to have system prompt")
	}
	if aiContext.UserPrompt == "" {
		t.Error("Expected AI context to have user prompt")
	}
	if aiContext.CodebaseContext == "" {
		t.Error("Expected AI context to have codebase context")
	}
	if aiContext.DeletionContext == "" {
		t.Error("Expected AI context to have deletion context")
	}

	// Test analysis with the enhanced context
	result, err := analyzer.AnalyzeDeletions(request)
	if err != nil {
		t.Fatalf("AnalyzeDeletions failed: %v", err)
	}

	// Verify results
	if result == nil {
		t.Fatal("Expected analysis result, got nil")
	}

	// Should find the orphaned references to CalculateSum and ProcessData
	if len(result.OrphanedReferences) == 0 {
		t.Error("Expected to find orphaned references")
	}

	// Verify specific references are found
	foundCalculateSum := false
	foundProcessData := false
	for _, ref := range result.OrphanedReferences {
		if ref.DeletedEntity == "CalculateSum" {
			foundCalculateSum = true
		}
		if ref.DeletedEntity == "ProcessData" {
			foundProcessData = true
		}
	}

	if !foundCalculateSum {
		t.Error("Expected to find reference to CalculateSum")
	}
	if !foundProcessData {
		t.Error("Expected to find reference to ProcessData")
	}

	// Should have reasonable confidence
	if result.Confidence < 0.5 || result.Confidence > 1.0 {
		t.Errorf("Expected confidence between 0.5-1.0, got %f", result.Confidence)
	}

	// Should have warnings for orphaned references
	if len(result.OrphanedReferences) > 0 && len(result.Warnings) == 0 {
		t.Error("Expected warnings when orphaned references found")
	}

	t.Logf("AI Context Integration Test Results:")
	t.Logf("  System prompt length: %d chars", len(aiContext.SystemPrompt))
	t.Logf("  User prompt length: %d chars", len(aiContext.UserPrompt))
	t.Logf("  Codebase context length: %d chars", len(aiContext.CodebaseContext))
	t.Logf("  Deletion context length: %d chars", len(aiContext.DeletionContext))
	t.Logf("  Found %d orphaned references", len(result.OrphanedReferences))
	t.Logf("  Analysis confidence: %.2f", result.Confidence)
	t.Logf("  Summary: %s", result.Summary)
}

// Helper functions

func findDeletedContentByFile(deletedContent []DeletedCode, filename string) *DeletedCode {
	for _, deleted := range deletedContent {
		if deleted.File == filename {
			return &deleted
		}
	}
	return nil
}

func createDeletionAnalysisRequest(codebase *FlattenedCodebase, deletedContent []DeletedCode, context string) *DeletionAnalysisRequest {
	return &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        context,
	}
}

func TestDeletionAnalyzer_AnalyzeDeletions_Integration(t *testing.T) {
	// Create a sample codebase with references to code that will be deleted
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
	result := UtilityFunction("test") // This will become orphaned
	fmt.Println(result)
}

func KeepThisFunction() {
	// This function should be safe
}
`,
				LineCount: 14,
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
		TotalLines: 23,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
		},
		Summary: "Test codebase with orphaned references",
	}

	// Create deleted content that is referenced in the codebase
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

	// Create analysis request
	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Removed utility functions during refactoring",
	}

	// Run analysis
	analyzer := NewDefaultDeletionAnalyzer()
	result, err := analyzer.AnalyzeDeletions(request)
	if err != nil {
		t.Fatalf("AnalyzeDeletions failed: %v", err)
	}

	// Verify results
	if result == nil {
		t.Fatal("Expected analysis result, got nil")
	}

	// Should find orphaned references
	if len(result.OrphanedReferences) == 0 {
		t.Error("Expected to find orphaned references, got none")
	}

	// Should have reasonable confidence
	if result.Confidence < 0.5 || result.Confidence > 1.0 {
		t.Errorf("Expected confidence between 0.5-1.0, got %f", result.Confidence)
	}

	// Should have a summary
	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Should have warnings if issues found
	if len(result.OrphanedReferences) > 0 && len(result.Warnings) == 0 {
		t.Error("Expected warnings when orphaned references found")
	}

	// Verify specific orphaned references
	foundDeletedFunction := false
	foundUtilityFunction := false
	for _, ref := range result.OrphanedReferences {
		if ref.DeletedEntity == "DeletedFunction" {
			foundDeletedFunction = true
		}
		if ref.DeletedEntity == "UtilityFunction" {
			foundUtilityFunction = true
		}
	}

	if !foundDeletedFunction {
		t.Error("Expected to find reference to DeletedFunction")
	}
	if !foundUtilityFunction {
		t.Error("Expected to find reference to UtilityFunction")
	}

	t.Logf("Analysis complete:")
	t.Logf("  Found %d orphaned references", len(result.OrphanedReferences))
	t.Logf("  Confidence: %.2f", result.Confidence)
	t.Logf("  Warnings: %d", len(result.Warnings))
	t.Logf("  Summary: %s", result.Summary)

	for i, ref := range result.OrphanedReferences {
		t.Logf("  Reference %d: %s in %s (line %v)", i+1, ref.DeletedEntity, ref.ReferencingFile, ref.ReferencingLines)
	}
}
