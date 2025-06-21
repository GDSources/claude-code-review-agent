package analyzer

import (
	"testing"
)

func TestParseDiff(t *testing.T) {
	tests := []struct {
		name            string
		rawDiff         string
		expectedFiles   int
		expectedAdded   int
		expectedRemoved int
		expectError     bool
	}{
		{
			name:            "empty diff",
			rawDiff:         "",
			expectedFiles:   0,
			expectedAdded:   0,
			expectedRemoved: 0,
			expectError:     false,
		},
		{
			name: "single file modification",
			rawDiff: `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,4 +1,5 @@
 package main
 
 func main() {
+	fmt.Println("Hello, World!")
 }`,
			expectedFiles:   1,
			expectedAdded:   1,
			expectedRemoved: 0,
			expectError:     false,
		},
		{
			name: "multiple files with various changes",
			rawDiff: `diff --git a/file1.go b/file1.go
index 1111111..2222222 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 package main
 
+// New comment
 func main() {
@@ -10,2 +11,1 @@ func helper() {
-	oldCode := true
-	return oldCode
+	return false
 }
diff --git a/file2.go b/file2.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/file2.go
@@ -0,0 +1,3 @@
+package utils
+
+// New file
diff --git a/file3.go b/file3.go
deleted file mode 100644
index 4444444..0000000
--- a/file3.go
+++ /dev/null
@@ -1,2 +0,0 @@
-// This file is deleted
-package old`,
			expectedFiles:   3,
			expectedAdded:   5,
			expectedRemoved: 4,
			expectError:     false,
		},
	}

	analyzer := NewDefaultDiffAnalyzer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := analyzer.ParseDiff(tt.rawDiff)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError {
				if len(parsed.Files) != tt.expectedFiles {
					t.Errorf("expected %d files, got %d", tt.expectedFiles, len(parsed.Files))
				}
				if parsed.TotalAdded != tt.expectedAdded {
					t.Errorf("expected %d added lines, got %d", tt.expectedAdded, parsed.TotalAdded)
				}
				if parsed.TotalRemoved != tt.expectedRemoved {
					t.Errorf("expected %d removed lines, got %d", tt.expectedRemoved, parsed.TotalRemoved)
				}
				if parsed.TotalFiles != tt.expectedFiles {
					t.Errorf("expected total files %d, got %d", tt.expectedFiles, parsed.TotalFiles)
				}

				// Test file details for non-empty diffs
				if tt.expectedFiles > 0 {
					file := parsed.Files[0]
					switch tt.name {
					case "single file modification":
						if file.Filename != "main.go" {
							t.Errorf("expected filename 'main.go', got '%s'", file.Filename)
						}
						if file.Status != "modified" {
							t.Errorf("expected status 'modified', got '%s'", file.Status)
						}
						if file.Language != "go" {
							t.Errorf("expected language 'go', got '%s'", file.Language)
						}
						if len(file.Hunks) != 1 {
							t.Errorf("expected 1 hunk, got %d", len(file.Hunks))
						}
					case "multiple files with various changes":
						// Test first file
						if file.Filename != "file1.go" {
							t.Errorf("expected filename 'file1.go', got '%s'", file.Filename)
						}
						if file.Status != "modified" {
							t.Errorf("expected status 'modified', got '%s'", file.Status)
						}

						// Test second file (added)
						if len(parsed.Files) > 1 {
							file2 := parsed.Files[1]
							if file2.Status != "added" {
								t.Errorf("expected file2 status 'added', got '%s'", file2.Status)
							}
						}

						// Test third file (deleted)
						if len(parsed.Files) > 2 {
							file3 := parsed.Files[2]
							if file3.Status != "deleted" {
								t.Errorf("expected file3 status 'deleted', got '%s'", file3.Status)
							}
						}
					}
				}
			}
		})
	}
}

func TestExtractContext(t *testing.T) {
	rawDiff := `diff --git a/example.go b/example.go
index 1234567..abcdefg 100644
--- a/example.go
+++ b/example.go
@@ -1,8 +1,9 @@
 package main
 
 import "fmt"
 
 func main() {
+	// New comment
 	fmt.Println("Hello")
 	fmt.Println("World")
 }`

	analyzer := NewDefaultDiffAnalyzer()
	parsed, err := analyzer.ParseDiff(rawDiff)
	if err != nil {
		t.Fatalf("failed to parse diff: %v", err)
	}

	contextual, err := analyzer.ExtractContext(parsed, 3)
	if err != nil {
		t.Fatalf("failed to extract context: %v", err)
	}

	if len(contextual.FilesWithContext) != 1 {
		t.Errorf("expected 1 file with context, got %d", len(contextual.FilesWithContext))
	}

	fileWithContext := contextual.FilesWithContext[0]
	if len(fileWithContext.ContextBlocks) == 0 {
		t.Error("expected at least one context block")
	}

	// Test context block properties
	block := fileWithContext.ContextBlocks[0]
	if block.ChangeType != "addition" {
		t.Errorf("expected change type 'addition', got '%s'", block.ChangeType)
	}
	if block.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		filename         string
		expectedLanguage string
	}{
		{"main.go", "go"},
		{"script.js", "javascript"},
		{"component.tsx", "typescript"},
		{"app.py", "python"},
		{"Main.java", "java"},
		{"program.cpp", "cpp"},
		{"header.h", "c"},
		{"config.json", "json"},
		{"style.css", "css"},
		{"README.md", "markdown"},
		{"docker-compose.yml", "yaml"},
		{"unknown.xyz", "plaintext"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := detectLanguage(tt.filename)
			if result != tt.expectedLanguage {
				t.Errorf("expected language '%s' for '%s', got '%s'",
					tt.expectedLanguage, tt.filename, result)
			}
		})
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name             string
		header           string
		expectedOldStart int
		expectedOldCount int
		expectedNewStart int
		expectedNewCount int
		expectError      bool
	}{
		{
			name:             "basic hunk header",
			header:           "@@ -1,4 +1,5 @@",
			expectedOldStart: 1,
			expectedOldCount: 4,
			expectedNewStart: 1,
			expectedNewCount: 5,
			expectError:      false,
		},
		{
			name:             "hunk header with function name",
			header:           "@@ -10,3 +12,4 @@ func main() {",
			expectedOldStart: 10,
			expectedOldCount: 3,
			expectedNewStart: 12,
			expectedNewCount: 4,
			expectError:      false,
		},
		{
			name:             "hunk header without count",
			header:           "@@ -1 +1,2 @@",
			expectedOldStart: 1,
			expectedOldCount: 1,
			expectedNewStart: 1,
			expectedNewCount: 2,
			expectError:      false,
		},
		{
			name:        "invalid header",
			header:      "invalid header",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunk, err := parseHunkHeader(tt.header)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError {
				if hunk.OldStart != tt.expectedOldStart {
					t.Errorf("expected old start %d, got %d", tt.expectedOldStart, hunk.OldStart)
				}
				if hunk.OldCount != tt.expectedOldCount {
					t.Errorf("expected old count %d, got %d", tt.expectedOldCount, hunk.OldCount)
				}
				if hunk.NewStart != tt.expectedNewStart {
					t.Errorf("expected new start %d, got %d", tt.expectedNewStart, hunk.NewStart)
				}
				if hunk.NewCount != tt.expectedNewCount {
					t.Errorf("expected new count %d, got %d", tt.expectedNewCount, hunk.NewCount)
				}
			}
		})
	}
}

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		name             string
		line             string
		expectedFilename string
		expectError      bool
	}{
		{
			name:             "standard git diff header",
			line:             "diff --git a/src/main.go b/src/main.go",
			expectedFilename: "src/main.go",
			expectError:      false,
		},
		{
			name:             "renamed file",
			line:             "diff --git a/old/path.go b/new/path.go",
			expectedFilename: "old/path.go",
			expectError:      false,
		},
		{
			name:        "invalid header",
			line:        "diff --git invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, err := extractFilename(tt.line)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && filename != tt.expectedFilename {
				t.Errorf("expected filename '%s', got '%s'", tt.expectedFilename, filename)
			}
		})
	}
}
