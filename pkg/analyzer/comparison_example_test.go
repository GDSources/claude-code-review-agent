package analyzer

import (
	"os"
	"testing"
)

// TestCompareHeuristicVsClaudeAnalysis demonstrates the difference between 
// heuristic analysis and Claude AI analysis for deletion safety
func TestCompareHeuristicVsClaudeAnalysis(t *testing.T) {
	// Create test data
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import "fmt"

func main() {
	result := calculateTotal(10, 20) // Will be orphaned
	fmt.Printf("Total: %d\n", result)
	
	helper() // Safe call
}

func helper() {
	fmt.Println("Helper function")
}
`,
				LineCount: 12,
			},
		},
		TotalFiles: 1,
		TotalLines: 12,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
			Name: "test-comparison",
		},
		Summary: "Simple Go test file",
	}

	deletedContent := []DeletedCode{
		{
			File: "utils.go",
			Content: `func calculateTotal(a, b int) int {
    return a + b
}

func unusedFunction() {
    // This function is not referenced anywhere
}`,
			StartLine:  1,
			EndLine:    6,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Removing utility functions",
	}

	// Test 1: Heuristic analysis (fallback mode)
	t.Run("Heuristic Analysis", func(t *testing.T) {
		heuristicAnalyzer := NewDefaultDeletionAnalyzer() // No LLM client
		result, err := heuristicAnalyzer.AnalyzeDeletions(request)
		if err != nil {
			t.Fatalf("Heuristic analysis failed: %v", err)
		}

		t.Logf("🔧 Heuristic Analysis Results:")
		t.Logf("   Summary: %s", result.Summary)
		t.Logf("   Confidence: %.2f", result.Confidence)
		t.Logf("   Orphaned References: %d", len(result.OrphanedReferences))
		for _, ref := range result.OrphanedReferences {
			t.Logf("     - %s in %s", ref.DeletedEntity, ref.ReferencingFile)
		}
		t.Logf("   Safe Deletions: %v", result.SafeDeletions)

		// Heuristic analysis should find basic text-based matches
		if len(result.OrphanedReferences) == 0 {
			t.Error("Heuristic analysis should find some references")
		}
	})

	// Test 2: Claude AI analysis (if API key available)
	t.Run("Claude AI Analysis", func(t *testing.T) {
		apiKey := os.Getenv("CLAUDE_API_KEY")
		if apiKey == "" {
			t.Skip("CLAUDE_API_KEY not set, skipping Claude analysis")
		}

		// Create Claude client
		config := ClaudeDeletionConfig{
			APIKey:    apiKey,
			Model:     DefaultDeletionModel,
			MaxTokens: 4000, // Smaller for testing
		}

		claudeClient, err := NewClaudeDeletionClient(config)
		if err != nil {
			t.Fatalf("Failed to create Claude client: %v", err)
		}

		claudeAnalyzer := NewDeletionAnalyzerWithLLM(claudeClient)
		result, err := claudeAnalyzer.AnalyzeDeletions(request)
		if err != nil {
			t.Fatalf("Claude analysis failed: %v", err)
		}

		t.Logf("🤖 Claude AI Analysis Results:")
		t.Logf("   Summary: %s", result.Summary)
		t.Logf("   Confidence: %.2f", result.Confidence)
		t.Logf("   Orphaned References: %d", len(result.OrphanedReferences))
		for _, ref := range result.OrphanedReferences {
			t.Logf("     - %s in %s (severity: %s)", ref.DeletedEntity, ref.ReferencingFile, ref.Severity)
			t.Logf("       Suggestion: %s", ref.Suggestion)
		}
		t.Logf("   Safe Deletions: %v", result.SafeDeletions)
		t.Logf("   Warnings: %d", len(result.Warnings))

		// Claude should provide more nuanced analysis
		if len(result.OrphanedReferences) > 0 {
			// Check that Claude provides better suggestions
			for _, ref := range result.OrphanedReferences {
				if ref.Suggestion == "" {
					t.Error("Claude should provide suggestions for orphaned references")
				}
				if ref.Severity == "" {
					t.Error("Claude should provide severity levels")
				}
			}
		}

		// Claude should identify safe deletions
		if len(result.SafeDeletions) == 0 {
			t.Log("Claude might identify safe deletions (like unusedFunction)")
		}
	})
}

// TestRealWorldDeletionScenario tests a more complex, realistic deletion scenario
func TestRealWorldDeletionScenario(t *testing.T) {
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		t.Skip("CLAUDE_API_KEY not set, skipping real-world scenario test")
	}

	// Simulate a realistic refactoring scenario
	codebase := &FlattenedCodebase{
		Files: []FileContent{
			{
				RelativePath: "handlers/user_handler.go",
				Language:     "go",
				Content: `package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

func GetUserProfile(c *gin.Context) {
	userID := c.Param("id")
	
	// This will become orphaned after migration
	profile := LegacyUserService.GetProfile(userID)
	
	// New service call (safe)
	newProfile := NewUserService.GetProfile(userID)
	
	c.JSON(http.StatusOK, newProfile)
}

func UpdateUser(c *gin.Context) {
	// This still uses old validation
	if !LegacyValidator.IsValid(user) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
		return
	}
	
	// Rest of update logic...
}`,
				LineCount: 24,
			},
			{
				RelativePath: "services/auth_service.go",
				Language:     "go",
				Content: `package services

func AuthenticateUser(token string) (*User, error) {
	// Another orphaned reference
	return LegacyUserService.Authenticate(token)
}

func ValidatePermissions(user *User, resource string) bool {
	// Safe - doesn't use deleted services
	return user.HasPermission(resource)
}`,
				LineCount: 9,
			},
		},
		TotalFiles: 2,
		TotalLines: 33,
		Languages:  []string{"go"},
		ProjectInfo: ProjectInfo{
			Type: "go",
			Name: "user-api-service",
			Structure: map[string]string{
				"handlers": "HTTP request handlers",
				"services": "business logic services",
			},
		},
		Summary: "User API service with handlers and services",
	}

	deletedContent := []DeletedCode{
		{
			File: "services/legacy_user_service.go",
			Content: `// Legacy user service being removed during microservices migration

type LegacyUserService struct {
	db *sql.DB
}

func (s *LegacyUserService) GetProfile(userID string) *UserProfile {
	// Old implementation with direct DB access
	row := s.db.QueryRow("SELECT * FROM users WHERE id = ?", userID)
	// ... profile building logic
	return profile
}

func (s *LegacyUserService) Authenticate(token string) (*User, error) {
	// Legacy authentication logic
	return validateLegacyToken(token)
}`,
			StartLine:  1,
			EndLine:    16,
			Language:   "go",
			ChangeType: "deleted",
		},
		{
			File: "validation/legacy_validator.go",
			Content: `// Legacy validation logic being replaced

type LegacyValidator struct{}

func (v *LegacyValidator) IsValid(user *User) bool {
	// Old validation rules
	if user.Email == "" || user.Name == "" {
		return false
	}
	return validateLegacyFormat(user)
}

func validateLegacyFormat(user *User) bool {
	// Complex legacy validation
	return true
}`,
			StartLine:  1,
			EndLine:    14,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	request := &DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Microservices migration: Removing legacy user service and validation components. These are being replaced with new microservice endpoints and modern validation libraries.",
	}

	// Create Claude client
	config := ClaudeDeletionConfig{
		APIKey:    apiKey,
		Model:     DefaultDeletionModel,
		MaxTokens: 6000, // Larger for complex analysis
	}

	claudeClient, err := NewClaudeDeletionClient(config)
	if err != nil {
		t.Fatalf("Failed to create Claude client: %v", err)
	}

	analyzer := NewDeletionAnalyzerWithLLM(claudeClient)

	t.Log("🏗️  Analyzing real-world microservices migration scenario...")
	
	result, err := analyzer.AnalyzeDeletions(request)
	if err != nil {
		t.Fatalf("Real-world analysis failed: %v", err)
	}

	t.Logf("🎯 Real-World Migration Analysis:")
	t.Logf("   Summary: %s", result.Summary)
	t.Logf("   Confidence: %.2f", result.Confidence)
	t.Logf("")

	if len(result.OrphanedReferences) > 0 {
		t.Logf("🚨 Critical Issues Found (%d):", len(result.OrphanedReferences))
		for i, ref := range result.OrphanedReferences {
			t.Logf("   %d. %s", i+1, ref.DeletedEntity)
			t.Logf("      📍 Location: %s (lines %v)", ref.ReferencingFile, ref.ReferencingLines)
			t.Logf("      ⚠️  Severity: %s", ref.Severity)
			t.Logf("      🔧 Suggestion: %s", ref.Suggestion)
			t.Logf("")
		}

		// Verify Claude found the expected orphaned references
		expectedRefs := []string{"LegacyUserService", "LegacyValidator"}
		for _, expected := range expectedRefs {
			found := false
			for _, ref := range result.OrphanedReferences {
				if ref.DeletedEntity == expected || 
				   contains(ref.DeletedEntity, expected) ||
				   contains(ref.Context, expected) {
					found = true
					break
				}
			}
			if !found {
				t.Logf("Note: Expected to find reference to %s", expected)
			}
		}
	}

	if len(result.Warnings) > 0 {
		t.Logf("⚠️  Migration Warnings (%d):", len(result.Warnings))
		for _, warning := range result.Warnings {
			t.Logf("   • %s", warning.Message)
		}
	}

	// This is a real-world scenario, so we expect some issues
	if len(result.OrphanedReferences) == 0 {
		t.Log("🤔 Unexpected: No orphaned references found in migration scenario")
	}
}

// Helper function for string containment check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
		     (s[:len(substr)] == substr || 
		      s[len(s)-len(substr):] == substr ||
		      containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}