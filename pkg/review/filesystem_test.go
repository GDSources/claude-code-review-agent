package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultFileSystemManager_CreateTempDir(t *testing.T) {
	fsManager := NewDefaultFileSystemManager()

	tempDir, err := fsManager.CreateTempDir("test-prefix-")
	if err != nil {
		t.Fatalf("unexpected error creating temp dir: %v", err)
	}

	defer os.RemoveAll(tempDir)

	// Verify directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("expected temp directory to exist")
	}

	// Verify prefix is used
	dirName := filepath.Base(tempDir)
	if !strings.HasPrefix(dirName, "test-prefix-") {
		t.Errorf("expected directory name to have prefix 'test-prefix-', got '%s'", dirName)
	}
}

func TestDefaultFileSystemManager_RemoveAll(t *testing.T) {
	fsManager := NewDefaultFileSystemManager()

	// Create a temporary directory to remove
	tempDir, err := fsManager.CreateTempDir("remove-test-")
	if err != nil {
		t.Fatalf("unexpected error creating temp dir: %v", err)
	}

	// Create a file inside to verify recursive removal
	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("unexpected error creating test file: %v", err)
	}

	// Remove the directory
	err = fsManager.RemoveAll(tempDir)
	if err != nil {
		t.Errorf("unexpected error removing directory: %v", err)
	}

	// Verify directory no longer exists
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("expected temp directory to be removed")
	}
}

func TestDefaultFileSystemManager_Exists(t *testing.T) {
	fsManager := NewDefaultFileSystemManager()

	// Test with existing directory
	tempDir, err := fsManager.CreateTempDir("exists-test-")
	if err != nil {
		t.Fatalf("unexpected error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if !fsManager.Exists(tempDir) {
		t.Error("expected Exists to return true for existing directory")
	}

	// Test with non-existing path
	nonExistentPath := "/path/that/does/not/exist"
	if fsManager.Exists(nonExistentPath) {
		t.Error("expected Exists to return false for non-existent path")
	}
}

func TestDefaultFileSystemManager_Integration(t *testing.T) {
	fsManager := NewDefaultFileSystemManager()

	// Create temp dir
	tempDir, err := fsManager.CreateTempDir("integration-test-")
	if err != nil {
		t.Fatalf("unexpected error creating temp dir: %v", err)
	}

	// Verify it exists
	if !fsManager.Exists(tempDir) {
		t.Error("expected created directory to exist")
	}

	// Create subdirectory and file structure
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("unexpected error creating subdirectory: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("unexpected error creating test file: %v", err)
	}

	// Verify file exists
	if !fsManager.Exists(testFile) {
		t.Error("expected test file to exist")
	}

	// Remove all
	err = fsManager.RemoveAll(tempDir)
	if err != nil {
		t.Errorf("unexpected error removing directory: %v", err)
	}

	// Verify everything is gone
	if fsManager.Exists(tempDir) {
		t.Error("expected directory to be removed")
	}
	if fsManager.Exists(testFile) {
		t.Error("expected test file to be removed")
	}
}
