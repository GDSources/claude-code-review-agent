package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCodebaseFlattener_FlattenWorkspace(t *testing.T) {
	// Create a test directory structure
	testDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}

func helperFunction() string {
	return "helper"
}
`,
		"pkg/utils/utils.go": `package utils

func UtilityFunction() {
	// Some utility
}

type UtilityStruct struct {
	Name string
}
`,
		"cmd/cli/main.go": `package main

import "github.com/example/pkg/utils"

func main() {
	utils.UtilityFunction()
}
`,
		"README.md": `# Test Project

This is a test project.
`,
		"go.mod": `module github.com/example/test-project

go 1.19
`,
		// Files that should be excluded
		"node_modules/package/index.js": `// Should be excluded`,
		"vendor/dependency/lib.go":      `// Should be excluded`,
		".git/config":                   `// Should be excluded`,
	}

	// Create directories and files
	for filePath, content := range files {
		fullPath := filepath.Join(testDir, filePath)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	// Create flattener and test
	flattener := NewDefaultCodebaseFlattener()

	codebase, err := flattener.FlattenWorkspace(testDir)
	if err != nil {
		t.Fatalf("FlattenWorkspace failed: %v", err)
	}

	// Test basic properties
	if codebase.TotalFiles == 0 {
		t.Error("Expected to find some files, got none")
	}

	if codebase.TotalLines == 0 {
		t.Error("Expected to find some lines of code, got none")
	}

	// Test that Go files were included
	goFileFound := false
	for _, file := range codebase.Files {
		if file.Language == "go" {
			goFileFound = true
			break
		}
	}
	if !goFileFound {
		t.Error("Expected to find Go files")
	}

	// Test that excluded directories were skipped
	for _, file := range codebase.Files {
		if filepath.Base(filepath.Dir(file.RelativePath)) == "node_modules" ||
			filepath.Base(filepath.Dir(file.RelativePath)) == "vendor" ||
			filepath.Base(filepath.Dir(file.RelativePath)) == ".git" {
			t.Errorf("Expected to exclude file %s", file.RelativePath)
		}
	}

	// Test project info detection
	if codebase.ProjectInfo.Type != "go" {
		t.Errorf("Expected project type 'go', got '%s'", codebase.ProjectInfo.Type)
	}

	// Test that main.go is detected as a main file
	mainFileFound := false
	for _, mainFile := range codebase.ProjectInfo.MainFiles {
		if mainFile == "main.go" {
			mainFileFound = true
			break
		}
	}
	if !mainFileFound {
		t.Error("Expected to find main.go as a main file")
	}

	// Test that languages were detected
	if len(codebase.Languages) == 0 {
		t.Error("Expected to detect some languages")
	}

	// Test summary
	if codebase.Summary == "" {
		t.Error("Expected a non-empty summary")
	}

	t.Logf("Found %d files with %d total lines", codebase.TotalFiles, codebase.TotalLines)
	t.Logf("Languages: %v", codebase.Languages)
	t.Logf("Project type: %s", codebase.ProjectInfo.Type)
	t.Logf("Summary: %s", codebase.Summary)
}

func TestDefaultCodebaseFlattener_FlattenDiff(t *testing.T) {
	// Create a test directory structure
	testDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"main.go": `package main

func main() {
	fmt.Println("Hello, World!")
}
`,
		"utils.go": `package main

func UtilityFunction() {
	// Some utility
}
`,
		"other.go": `package main

func OtherFunction() {
	// Not in diff
}
`,
	}

	// Create files
	for filePath, content := range files {
		fullPath := filepath.Join(testDir, filePath)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	// Create a mock diff
	diff := &ParsedDiff{
		Files: []FileDiff{
			{
				Filename: "main.go",
				Status:   "modified",
				Language: "go",
			},
			{
				Filename: "utils.go",
				Status:   "modified",
				Language: "go",
			},
		},
	}

	// Create flattener and test
	flattener := NewDefaultCodebaseFlattener()

	codebase, err := flattener.FlattenDiff(testDir, diff)
	if err != nil {
		t.Fatalf("FlattenDiff failed: %v", err)
	}

	// Test that only diff files were included
	if codebase.TotalFiles != 2 {
		t.Errorf("Expected 2 files, got %d", codebase.TotalFiles)
	}

	// Test that the correct files were included
	expectedFiles := map[string]bool{"main.go": false, "utils.go": false}
	for _, file := range codebase.Files {
		if _, exists := expectedFiles[file.RelativePath]; exists {
			expectedFiles[file.RelativePath] = true
		} else {
			t.Errorf("Unexpected file included: %s", file.RelativePath)
		}
	}

	for fileName, found := range expectedFiles {
		if !found {
			t.Errorf("Expected file %s not found", fileName)
		}
	}

	t.Logf("Flattened %d files from diff", codebase.TotalFiles)
}

func TestDefaultCodebaseFlattener_ExcludePaths(t *testing.T) {
	testDir := t.TempDir()

	// Create files in excluded directories
	excludedFiles := map[string]string{
		"node_modules/package/index.js": `console.log("excluded");`,
		"vendor/lib/code.go":            `package lib`,
		".git/hooks/pre-commit":         `#!/bin/bash`,
		"dist/bundle.js":                `// bundled code`,
		"build/output.go":               `package build`,
		"target/debug/main.rs":          `fn main() {}`,
		".cache/temp.js":                `// cache`,
		"__pycache__/module.pyc":        `// compiled python`,
		".venv/lib/python.py":           `# virtual env`,
	}

	// Create directories and files
	for filePath, content := range excludedFiles {
		fullPath := filepath.Join(testDir, filePath)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	// Create a valid file that should be included
	validFile := filepath.Join(testDir, "main.go")
	err := os.WriteFile(validFile, []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("Failed to write valid file: %v", err)
	}

	flattener := NewDefaultCodebaseFlattener()
	codebase, err := flattener.FlattenWorkspace(testDir)
	if err != nil {
		t.Fatalf("FlattenWorkspace failed: %v", err)
	}

	// Should only find the valid file
	if codebase.TotalFiles != 1 {
		t.Errorf("Expected 1 file, got %d", codebase.TotalFiles)
		for _, file := range codebase.Files {
			t.Logf("Found file: %s", file.RelativePath)
		}
	}

	// Verify it's the correct file
	if len(codebase.Files) > 0 && codebase.Files[0].RelativePath != "main.go" {
		t.Errorf("Expected main.go, got %s", codebase.Files[0].RelativePath)
	}
}

func TestDefaultCodebaseFlattener_FileSize(t *testing.T) {
	testDir := t.TempDir()

	// Create a large file that should be excluded
	largeContent := make([]byte, 2*1024*1024) // 2MB
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	largeFile := filepath.Join(testDir, "large.go")
	err := os.WriteFile(largeFile, largeContent, 0644)
	if err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	// Create a small file that should be included
	smallFile := filepath.Join(testDir, "small.go")
	err = os.WriteFile(smallFile, []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("Failed to write small file: %v", err)
	}

	flattener := NewDefaultCodebaseFlattener()
	codebase, err := flattener.FlattenWorkspace(testDir)
	if err != nil {
		t.Fatalf("FlattenWorkspace failed: %v", err)
	}

	// Should only find the small file
	if codebase.TotalFiles != 1 {
		t.Errorf("Expected 1 file, got %d", codebase.TotalFiles)
	}

	if len(codebase.Files) > 0 && codebase.Files[0].RelativePath != "small.go" {
		t.Errorf("Expected small.go, got %s", codebase.Files[0].RelativePath)
	}
}

func TestDefaultCodebaseFlattener_ProjectDetection(t *testing.T) {
	testCases := []struct {
		name              string
		files             map[string]string
		expectedType      string
		expectedStructure map[string]string
	}{
		{
			name: "Go project",
			files: map[string]string{
				"go.mod":           "module test",
				"main.go":          "package main",
				"pkg/lib/code.go":  "package lib",
				"cmd/cli/main.go":  "package main",
				"internal/util.go": "package internal",
			},
			expectedType: "go",
			expectedStructure: map[string]string{
				"pkg":      "packages/libraries",
				"cmd":      "command line tools",
				"internal": "internal packages",
			},
		},
		{
			name: "Node.js project",
			files: map[string]string{
				"package.json":     `{"name": "test"}`,
				"src/index.js":     "console.log('hello');",
				"test/app.test.js": "// tests",
			},
			expectedType: "node",
			expectedStructure: map[string]string{
				"src":  "source code",
				"test": "tests",
			},
		},
		{
			name: "Python project",
			files: map[string]string{
				"setup.py":           "from setuptools import setup",
				"src/main.py":        "print('hello')",
				"tests/test_main.py": "import unittest",
			},
			expectedType: "python",
			expectedStructure: map[string]string{
				"src":   "source code",
				"tests": "tests",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()

			// Create files
			for filePath, content := range tc.files {
				fullPath := filepath.Join(testDir, filePath)
				dir := filepath.Dir(fullPath)

				err := os.MkdirAll(dir, 0755)
				if err != nil {
					t.Fatalf("Failed to create directory %s: %v", dir, err)
				}

				err = os.WriteFile(fullPath, []byte(content), 0644)
				if err != nil {
					t.Fatalf("Failed to write file %s: %v", fullPath, err)
				}
			}

			flattener := NewDefaultCodebaseFlattener()
			codebase, err := flattener.FlattenWorkspace(testDir)
			if err != nil {
				t.Fatalf("FlattenWorkspace failed: %v", err)
			}

			// Test project type
			if codebase.ProjectInfo.Type != tc.expectedType {
				t.Errorf("Expected project type '%s', got '%s'", tc.expectedType, codebase.ProjectInfo.Type)
				t.Logf("Found files:")
				for _, file := range codebase.Files {
					t.Logf("  %s (language: %s)", file.RelativePath, file.Language)
				}
				t.Logf("Config files: %v", codebase.ProjectInfo.ConfigFiles)
				t.Logf("Main files: %v", codebase.ProjectInfo.MainFiles)
			}

			// Test structure detection
			for expectedDir, expectedPurpose := range tc.expectedStructure {
				if actualPurpose, exists := codebase.ProjectInfo.Structure[expectedDir]; !exists {
					t.Errorf("Expected directory '%s' to be detected", expectedDir)
				} else if actualPurpose != expectedPurpose {
					t.Errorf("Expected directory '%s' purpose '%s', got '%s'", expectedDir, expectedPurpose, actualPurpose)
				}
			}
		})
	}
}
