package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/your-org/review-agent/pkg/analyzer"
)

func main() {
	// Get Claude API key from environment
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		log.Fatal("CLAUDE_API_KEY environment variable is required")
	}

	// Create Claude client for deletion analysis
	claudeConfig := analyzer.ClaudeDeletionConfig{
		APIKey:      apiKey,
		Model:       analyzer.DefaultDeletionModel,
		MaxTokens:   analyzer.DefaultDeletionMaxTokens,
		Temperature: analyzer.DefaultDeletionTemperature,
	}

	claudeClient, err := analyzer.NewClaudeDeletionClient(claudeConfig)
	if err != nil {
		log.Fatalf("Failed to create Claude client: %v", err)
	}

	// Create deletion analyzer with Claude LLM integration
	analyzer := analyzer.NewDeletionAnalyzerWithLLM(claudeClient)

	// Example: Analyze a code deletion scenario
	codebase := &analyzer.FlattenedCodebase{
		Files: []analyzer.FileContent{
			{
				RelativePath: "main.go",
				Language:     "go",
				Content: `package main

import (
	"fmt"
	"log"
)

func main() {
	// This function call will become orphaned
	result := CalculateUserScore(user, criteria)
	log.Printf("User score: %d", result)
	
	// This call references a deleted utility
	data := ProcessUserData(user.ID)
	fmt.Println(data)
	
	// This should be safe
	DisplayWelcomeMessage()
}

func DisplayWelcomeMessage() {
	fmt.Println("Welcome to our application!")
}
`,
				LineCount: 21,
			},
			{
				RelativePath: "user_service.go",
				Language:     "go",
				Content: `package main

import "database/sql"

func GetUserProfile(userID string) (*User, error) {
	// This will also have orphaned references
	score := CalculateUserScore(user, defaultCriteria)
	
	// Safe database operation
	return fetchUserFromDB(userID)
}

func fetchUserFromDB(userID string) (*User, error) {
	// Safe database access
	return nil, nil
}
`,
				LineCount: 13,
			},
		},
		TotalFiles: 2,
		TotalLines: 34,
		Languages:  []string{"go"},
		ProjectInfo: analyzer.ProjectInfo{
			Type: "go",
			Name: "user-management-service",
			Structure: map[string]string{
				"main": "application entry point",
				"user": "user management functionality",
			},
		},
		Summary: "Go user management service with 2 files",
	}

	// Define what was deleted
	deletedContent := []analyzer.DeletedCode{
		{
			File: "scoring.go",
			Content: `// User scoring functions that were removed during refactoring

func CalculateUserScore(user *User, criteria ScoringCriteria) int {
	score := 0
	
	// Complex scoring logic
	for _, criterion := range criteria.Rules {
		score += evaluateCriterion(user, criterion)
	}
	
	return score
}

func evaluateCriterion(user *User, criterion Rule) int {
	// Criterion evaluation logic
	return criterion.Weight * getUserValue(user, criterion.Field)
}`,
			StartLine:  1,
			EndLine:    15,
			Language:   "go",
			ChangeType: "deleted",
		},
		{
			File: "utils.go",
			Content: `// Utility functions removed during cleanup

func ProcessUserData(userID string) map[string]interface{} {
	// Data processing logic
	return map[string]interface{}{
		"processed": true,
		"userID":    userID,
	}
}

const defaultCriteria = ScoringCriteria{
	Rules: []Rule{
		{Field: "activity", Weight: 10},
		{Field: "engagement", Weight: 15},
	},
}`,
			StartLine:  1,
			EndLine:    16,
			Language:   "go",
			ChangeType: "deleted",
		},
	}

	// Create deletion analysis request
	request := &analyzer.DeletionAnalysisRequest{
		Codebase:       codebase,
		DeletedContent: deletedContent,
		Context:        "Refactoring user management service - removed old scoring system and simplified data processing utilities",
	}

	fmt.Println("🔍 Analyzing code deletions with Claude AI...")
	fmt.Println("📁 Codebase: 2 Go files, 34 lines")
	fmt.Println("🗑️  Deleted: 2 files with scoring and utility functions")
	fmt.Println()

	// Run AI-powered deletion analysis
	ctx := context.Background()
	result, err := analyzer.AnalyzeDeletionsWithContext(ctx, request)
	if err != nil {
		log.Fatalf("Deletion analysis failed: %v", err)
	}

	// Display results
	fmt.Println("🤖 Claude AI Analysis Results:")
	fmt.Println("=====================================")
	fmt.Printf("📊 Summary: %s\n", result.Summary)
	fmt.Printf("🎯 Confidence: %.1f%%\n", result.Confidence*100)
	fmt.Println()

	if len(result.OrphanedReferences) > 0 {
		fmt.Printf("⚠️  Found %d Orphaned References:\n", len(result.OrphanedReferences))
		for i, ref := range result.OrphanedReferences {
			fmt.Printf("  %d. %s\n", i+1, ref.DeletedEntity)
			fmt.Printf("     📄 File: %s (lines %v)\n", ref.ReferencingFile, ref.ReferencingLines)
			fmt.Printf("     🔥 Severity: %s\n", ref.Severity)
			fmt.Printf("     💡 Suggestion: %s\n", ref.Suggestion)
			fmt.Println()
		}
	} else {
		fmt.Println("✅ No orphaned references found - deletion appears safe!")
	}

	if len(result.SafeDeletions) > 0 {
		fmt.Printf("✅ Safe Deletions (%d):\n", len(result.SafeDeletions))
		for _, entity := range result.SafeDeletions {
			fmt.Printf("  • %s\n", entity)
		}
		fmt.Println()
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("⚠️  Warnings (%d):\n", len(result.Warnings))
		for _, warning := range result.Warnings {
			fmt.Printf("  • %s: %s\n", warning.Type, warning.Message)
			if warning.Suggestion != "" {
				fmt.Printf("    💡 %s\n", warning.Suggestion)
			}
		}
		fmt.Println()
	}

	fmt.Println("Analysis complete! 🎉")
}