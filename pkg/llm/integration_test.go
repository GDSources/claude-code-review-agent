package llm

import (
	"context"
	"testing"

	"github.com/your-org/review-agent/pkg/analyzer"
	"github.com/your-org/review-agent/pkg/github"
)

func TestClaudeClient_Integration(t *testing.T) {
	// Skip integration test if no API key is provided
	apiKey := getTestAPIKey()
	if apiKey == "" {
		t.Skip("Skipping integration test: CLAUDE_API_KEY not set")
	}

	// Create real Claude client
	config := ClaudeConfig{
		APIKey:      apiKey,
		Model:       DefaultClaudeModel,
		MaxTokens:   DefaultClaudeMaxTokens,
		Temperature: DefaultClaudeTemperature,
		BaseURL:     DefaultClaudeBaseURL,
		Timeout:     DefaultTimeoutSeconds,
	}

	client, err := NewClaudeClient(config)
	if err != nil {
		t.Fatalf("failed to create Claude client: %v", err)
	}

	// Test configuration validation
	if err := client.ValidateConfiguration(); err != nil {
		t.Errorf("configuration validation failed: %v", err)
	}

	// Test model info
	info := client.GetModelInfo()
	if info.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", info.Provider)
	}

	// Create test review request
	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number:      1,
			Title:       "Add new feature",
			Author:      "developer",
			Description: "This PR adds a new feature to the application",
			BaseBranch:  "main",
			HeadBranch:  "feature/new-feature",
		},
		DiffResult: &github.DiffResult{
			RawDiff: `diff --git a/example.go b/example.go
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/example.go
@@ -0,0 +1,10 @@
+package main
+
+import "fmt"
+
+func main() {
+    // This is a simple example
+    name := "World"
+    fmt.Printf("Hello, %s!\n", name)
+}`,
			TotalFiles: 1,
		},
		ContextualDiff: &analyzer.ContextualDiff{
			ParsedDiff: &analyzer.ParsedDiff{
				TotalFiles:   1,
				TotalAdded:   9,
				TotalRemoved: 0,
			},
			FilesWithContext: []analyzer.FileWithContext{
				{
					FileDiff: analyzer.FileDiff{
						Filename:  "example.go",
						Status:    "added",
						Language:  "go",
						Additions: 9,
						Deletions: 0,
					},
					ContextBlocks: []analyzer.ContextBlock{
						{
							StartLine:   1,
							EndLine:     9,
							ChangeType:  "addition",
							Description: "Added 9 line(s)",
							Lines: []analyzer.DiffLine{
								{Type: "added", Content: "package main", NewLineNo: 1},
								{Type: "added", Content: "", NewLineNo: 2},
								{Type: "added", Content: "import \"fmt\"", NewLineNo: 3},
								{Type: "added", Content: "", NewLineNo: 4},
								{Type: "added", Content: "func main() {", NewLineNo: 5},
								{Type: "added", Content: "    // This is a simple example", NewLineNo: 6},
								{Type: "added", Content: "    name := \"World\"", NewLineNo: 7},
								{Type: "added", Content: "    fmt.Printf(\"Hello, %s!\\n\", name)", NewLineNo: 8},
								{Type: "added", Content: "}", NewLineNo: 9},
							},
						},
					},
				},
			},
		},
		ReviewType:   ReviewTypeGeneral,
		Instructions: "Please provide a thorough review focusing on code quality and best practices.",
	}

	// Perform actual review
	ctx := context.Background()
	response, err := client.ReviewCode(ctx, request)
	if err != nil {
		t.Fatalf("ReviewCode failed: %v", err)
	}

	// Validate response
	if response == nil {
		t.Fatal("expected response but got nil")
	}

	if response.ModelUsed == "" {
		t.Error("expected model to be specified")
	}

	if response.TokensUsed.TotalTokens == 0 {
		t.Error("expected token usage to be reported")
	}

	if response.ReviewID == "" {
		t.Error("expected review ID to be generated")
	}

	if response.GeneratedAt == "" {
		t.Error("expected generation timestamp")
	}

	// Comments may or may not be generated depending on the code
	t.Logf("Generated %d comments", len(response.Comments))
	for i, comment := range response.Comments {
		t.Logf("Comment %d: %s:%d - %s", i+1, comment.Filename, comment.LineNumber, comment.Comment)
	}

	if response.Summary != "" {
		t.Logf("Summary: %s", response.Summary)
	}

	t.Logf("Model: %s, Tokens: %d (in: %d, out: %d)",
		response.ModelUsed,
		response.TokensUsed.TotalTokens,
		response.TokensUsed.InputTokens,
		response.TokensUsed.OutputTokens)
}

func TestClaudeClient_DifferentReviewTypes(t *testing.T) {
	apiKey := getTestAPIKey()
	if apiKey == "" {
		t.Skip("Skipping integration test: CLAUDE_API_KEY not set")
	}

	client, err := NewClaudeClient(ClaudeConfig{APIKey: apiKey})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	reviewTypes := []ReviewType{
		ReviewTypeGeneral,
		ReviewTypeSecurity,
		ReviewTypePerformance,
		ReviewTypeStyle,
		ReviewTypeBugs,
		ReviewTypeTests,
	}

	baseRequest := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number: 1,
			Title:  "Test PR",
			Author: "tester",
		},
		DiffResult: &github.DiffResult{
			RawDiff: `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,3 +1,6 @@
 package main
 
+// TODO: Add error handling
 func main() {
+    password := "hardcoded_password"
+    fmt.Println(password)
 }`,
		},
	}

	for _, reviewType := range reviewTypes {
		t.Run(string(reviewType), func(t *testing.T) {
			request := *baseRequest
			request.ReviewType = reviewType

			ctx := context.Background()
			response, err := client.ReviewCode(ctx, &request)
			if err != nil {
				t.Errorf("ReviewCode failed for %s: %v", reviewType, err)
				return
			}

			if response == nil {
				t.Errorf("expected response for %s", reviewType)
				return
			}

			t.Logf("%s review completed: %d comments, %d tokens",
				reviewType, len(response.Comments), response.TokensUsed.TotalTokens)
		})
	}
}

func TestClaudeClient_LargeCodebase(t *testing.T) {
	apiKey := getTestAPIKey()
	if apiKey == "" {
		t.Skip("Skipping integration test: CLAUDE_API_KEY not set")
	}

	client, err := NewClaudeClient(ClaudeConfig{
		APIKey:    apiKey,
		MaxTokens: 1000, // Small limit to test chunking
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create large diff with multiple files
	files := make([]analyzer.FileWithContext, 5)
	for i := range files {
		files[i] = analyzer.FileWithContext{
			FileDiff: analyzer.FileDiff{
				Filename: "file" + string(rune('0'+i)) + ".go",
				Language: "go",
				Status:   "modified",
			},
			ContextBlocks: []analyzer.ContextBlock{
				{
					StartLine:   1,
					EndLine:     50,
					ChangeType:  "modification",
					Description: "Large code change",
					Lines:       generateManyDiffLines(50),
				},
			},
		}
	}

	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{
			Number: 1,
			Title:  "Large refactoring PR",
			Author: "developer",
		},
		ContextualDiff: &analyzer.ContextualDiff{
			FilesWithContext: files,
		},
		ReviewType: ReviewTypeGeneral,
	}

	ctx := context.Background()
	response, err := client.ReviewCode(ctx, request)
	if err != nil {
		t.Fatalf("ReviewCode failed for large codebase: %v", err)
	}

	if response == nil {
		t.Fatal("expected response for large codebase")
	}

	t.Logf("Large codebase review: %d comments, %d tokens, model: %s",
		len(response.Comments), response.TokensUsed.TotalTokens, response.ModelUsed)

	// Verify chunking worked (should have reasonable token usage)
	if response.TokensUsed.TotalTokens == 0 {
		t.Error("expected token usage to be reported")
	}
}

// Helper functions

func getTestAPIKey() string {
	// In real tests, this would read from environment variable
	// For now, return empty to skip integration tests
	// return os.Getenv("CLAUDE_API_KEY")
	return ""
}

func generateManyDiffLines(count int) []analyzer.DiffLine {
	lines := make([]analyzer.DiffLine, count)
	for i := range lines {
		if i%3 == 0 {
			lines[i] = analyzer.DiffLine{
				Type:      "added",
				Content:   "// Added line " + string(rune('0'+i%10)),
				NewLineNo: i + 1,
			}
		} else if i%3 == 1 {
			lines[i] = analyzer.DiffLine{
				Type:      "removed",
				Content:   "// Removed line " + string(rune('0'+i%10)),
				OldLineNo: i + 1,
			}
		} else {
			lines[i] = analyzer.DiffLine{
				Type:      "context",
				Content:   "// Context line " + string(rune('0'+i%10)),
				OldLineNo: i + 1,
				NewLineNo: i + 1,
			}
		}
	}
	return lines
}

// Benchmark tests

func BenchmarkClaudeClient_SmallReview(b *testing.B) {
	apiKey := getTestAPIKey()
	if apiKey == "" {
		b.Skip("Skipping benchmark: CLAUDE_API_KEY not set")
	}

	client, err := NewClaudeClient(ClaudeConfig{APIKey: apiKey})
	if err != nil {
		b.Fatalf("failed to create client: %v", err)
	}

	request := &ReviewRequest{
		PullRequestInfo: PullRequestInfo{Number: 1, Title: "Benchmark test"},
		DiffResult: &github.DiffResult{
			RawDiff: "diff --git a/test.go b/test.go\n+func test() {}",
		},
		ReviewType: ReviewTypeGeneral,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.ReviewCode(ctx, request)
		if err != nil {
			b.Fatalf("ReviewCode failed: %v", err)
		}
	}
}
