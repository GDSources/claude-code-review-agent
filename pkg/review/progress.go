package review

import (
	"fmt"
	"strings"
	"time"
)

// ReviewProgress represents the current state of a code review process
type ReviewProgress struct {
	Stage       string    `json:"stage"`        // "initializing", "analyzing", "reviewing", "completed", "failed"
	Message     string    `json:"message"`      // Current status message
	StartTime   time.Time `json:"start_time"`   // When the review started
	LastUpdated time.Time `json:"last_updated"` // When this progress was last updated
	Summary     string    `json:"summary"`      // Final summary for completed/failed reviews
}

// GenerateProgressComment generates a markdown comment showing the current review progress
func GenerateProgressComment(progress *ReviewProgress) string {
	var builder strings.Builder

	// Header with emoji based on stage
	emoji := getStageEmoji(progress.Stage)
	builder.WriteString(fmt.Sprintf("%s **Review Progress**\n\n", emoji))

	// Current stage and message
	builder.WriteString(fmt.Sprintf("**Stage:** %s\n", progress.Stage))
	builder.WriteString(fmt.Sprintf("**Status:** %s\n\n", progress.Message))

	// Elapsed time
	elapsed := progress.LastUpdated.Sub(progress.StartTime)
	builder.WriteString(fmt.Sprintf("**Elapsed:** %s\n", FormatElapsedTime(elapsed)))
	builder.WriteString(fmt.Sprintf("**Last Updated:** %s\n\n", progress.LastUpdated.Format("15:04:05")))

	// Summary section for completed or failed reviews
	if progress.Summary != "" && (progress.Stage == "completed" || progress.Stage == "failed") {
		builder.WriteString("## Summary\n\n")
		builder.WriteString(fmt.Sprintf("%s\n\n", progress.Summary))
	}

	// Progress marker (hidden HTML comment for identification)
	builder.WriteString("<!-- review-agent:progress-comment -->")

	return builder.String()
}

// getStageEmoji returns the appropriate emoji for each review stage
func getStageEmoji(stage string) string {
	switch stage {
	case "initializing":
		return "🔍"
	case "analyzing":
		return "📊"
	case "reviewing":
		return "💬"
	case "completed":
		return "✅"
	case "failed":
		return "❌"
	default:
		return "⏳"
	}
}

// CreateInitialProgress creates a new ReviewProgress for a starting review
func CreateInitialProgress(reviewData *ReviewData) *ReviewProgress {
	now := time.Now()
	message := fmt.Sprintf("Starting code review analysis for PR #%d...", reviewData.Event.Number)

	return &ReviewProgress{
		Stage:       "initializing",
		Message:     message,
		StartTime:   now,
		LastUpdated: now,
		Summary:     "",
	}
}

// UpdateProgressStage updates the progress to a new stage with a new message
func UpdateProgressStage(progress *ReviewProgress, stage, message string) {
	progress.Stage = stage
	progress.Message = message
	progress.LastUpdated = time.Now()
}

// FormatElapsedTime formats a duration into a human-readable string
func FormatElapsedTime(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%.0fs", duration.Seconds())
	}

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}

	return fmt.Sprintf("%dm%ds", minutes, seconds)
}
