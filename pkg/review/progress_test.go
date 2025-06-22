package review

import (
	"strings"
	"testing"
	"time"
)

// NEW FAILING TESTS FOR PROGRESS COMMENT GENERATOR (TDD APPROACH)

func TestGenerateInitialProgressComment_Success(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "initializing",
		Message:     "Starting code review analysis...",
		StartTime:   time.Now(),
		LastUpdated: time.Now(),
	}

	comment := GenerateProgressComment(progress)

	// Debug: print the actual comment
	t.Logf("Generated comment: %s", comment)

	if !strings.Contains(comment, "🔍") {
		t.Error("expected initial comment to contain search emoji")
	}

	if !strings.Contains(comment, "Starting code review analysis") {
		t.Error("expected comment to contain progress message")
	}

	if !strings.Contains(comment, "review-agent:progress-comment") {
		t.Error("expected comment to contain progress marker")
	}

	if !strings.Contains(comment, "**Stage:** initializing") {
		t.Error("expected comment to contain current stage")
	}
}

func TestGenerateProgressComment_Analyzing(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "analyzing",
		Message:     "Analyzing code changes...",
		StartTime:   time.Now().Add(-30 * time.Second),
		LastUpdated: time.Now(),
	}

	comment := GenerateProgressComment(progress)

	if !strings.Contains(comment, "📊") {
		t.Error("expected analyzing comment to contain chart emoji")
	}

	if !strings.Contains(comment, "Analyzing code changes") {
		t.Error("expected comment to contain progress message")
	}

	if !strings.Contains(comment, "**Stage:** analyzing") {
		t.Error("expected comment to contain current stage")
	}

	if !strings.Contains(comment, "**Elapsed:**") {
		t.Error("expected comment to contain elapsed time")
	}
}

func TestGenerateProgressComment_Reviewing(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "reviewing",
		Message:     "Generating review comments...",
		StartTime:   time.Now().Add(-60 * time.Second),
		LastUpdated: time.Now(),
	}

	comment := GenerateProgressComment(progress)

	if !strings.Contains(comment, "💬") {
		t.Error("expected reviewing comment to contain speech emoji")
	}

	if !strings.Contains(comment, "Generating review comments") {
		t.Error("expected comment to contain progress message")
	}

	if !strings.Contains(comment, "**Stage:** reviewing") {
		t.Error("expected comment to contain current stage")
	}
}

func TestGenerateProgressComment_Completed(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "completed",
		Message:     "Review completed successfully",
		StartTime:   time.Now().Add(-90 * time.Second),
		LastUpdated: time.Now(),
		Summary:     "Posted 5 review comments",
	}

	comment := GenerateProgressComment(progress)

	if !strings.Contains(comment, "✅") {
		t.Error("expected completed comment to contain check mark emoji")
	}

	if !strings.Contains(comment, "Review completed successfully") {
		t.Error("expected comment to contain progress message")
	}

	if !strings.Contains(comment, "Posted 5 review comments") {
		t.Error("expected comment to contain summary")
	}

	if !strings.Contains(comment, "**Stage:** completed") {
		t.Error("expected comment to contain current stage")
	}
}

func TestGenerateProgressComment_Failed(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "failed",
		Message:     "Review failed due to API error",
		StartTime:   time.Now().Add(-45 * time.Second),
		LastUpdated: time.Now(),
		Summary:     "Error: Unable to fetch diff",
	}

	comment := GenerateProgressComment(progress)

	if !strings.Contains(comment, "❌") {
		t.Error("expected failed comment to contain X emoji")
	}

	if !strings.Contains(comment, "Review failed due to API error") {
		t.Error("expected comment to contain progress message")
	}

	if !strings.Contains(comment, "Error: Unable to fetch diff") {
		t.Error("expected comment to contain error summary")
	}

	if !strings.Contains(comment, "**Stage:** failed") {
		t.Error("expected comment to contain current stage")
	}
}

func TestCreateProgressFromReviewData_Initial(t *testing.T) {
	reviewData := &ReviewData{
		Event: &PullRequestEvent{
			Number: 123,
		},
	}

	progress := CreateInitialProgress(reviewData)

	if progress.Stage != "initializing" {
		t.Errorf("expected stage 'initializing', got '%s'", progress.Stage)
	}

	if progress.Message == "" {
		t.Error("expected progress message to be set")
	}

	if progress.StartTime.IsZero() {
		t.Error("expected start time to be set")
	}

	if progress.LastUpdated.IsZero() {
		t.Error("expected last updated to be set")
	}
}

func TestUpdateProgressStage_Success(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "initializing",
		Message:     "Starting...",
		StartTime:   time.Now().Add(-30 * time.Second),
		LastUpdated: time.Now().Add(-30 * time.Second),
	}

	originalTime := progress.LastUpdated

	UpdateProgressStage(progress, "analyzing", "Analyzing code changes...")

	if progress.Stage != "analyzing" {
		t.Errorf("expected stage 'analyzing', got '%s'", progress.Stage)
	}

	if progress.Message != "Analyzing code changes..." {
		t.Errorf("expected message 'Analyzing code changes...', got '%s'", progress.Message)
	}

	if !progress.LastUpdated.After(originalTime) {
		t.Error("expected last updated time to be updated")
	}

	// Start time should remain unchanged
	if progress.StartTime.After(originalTime) {
		t.Error("expected start time to remain unchanged")
	}
}

func TestFormatElapsedTime_Success(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		expected string
	}{
		{
			name:     "Less than minute",
			elapsed:  30 * time.Second,
			expected: "30s",
		},
		{
			name:     "One minute",
			elapsed:  60 * time.Second,
			expected: "1m0s",
		},
		{
			name:     "Multiple minutes",
			elapsed:  150 * time.Second,
			expected: "2m30s",
		},
		{
			name:     "Hours and minutes",
			elapsed:  3661 * time.Second,
			expected: "1h1m1s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatElapsedTime(tt.elapsed)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestGenerateProgressCommentWithMarkdown_Success(t *testing.T) {
	progress := &ReviewProgress{
		Stage:       "completed",
		Message:     "Review completed",
		StartTime:   time.Now().Add(-120 * time.Second),
		LastUpdated: time.Now(),
		Summary:     "Posted 3 comments, found 2 issues",
	}

	comment := GenerateProgressComment(progress)

	// Should contain markdown formatting
	if !strings.Contains(comment, "**") {
		t.Error("expected comment to contain markdown bold formatting")
	}

	// Should contain structured sections
	if !strings.Contains(comment, "**Review Progress**") {
		t.Error("expected comment to contain progress header")
	}

	// Should contain summary section for completed reviews
	if !strings.Contains(comment, "## Summary") {
		t.Error("expected completed comment to contain summary section")
	}

	// Should contain the progress marker
	if !strings.Contains(comment, "<!-- review-agent:progress-comment -->") {
		t.Error("expected comment to contain HTML comment marker")
	}
}