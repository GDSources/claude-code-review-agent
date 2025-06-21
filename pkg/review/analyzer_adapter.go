package review

import (
	"github.com/your-org/review-agent/pkg/analyzer"
)

// AnalyzerAdapter adapts the analyzer package to implement CodeAnalyzer interface
type AnalyzerAdapter struct {
	analyzer analyzer.DiffAnalyzer
}

// NewAnalyzerAdapter creates a new analyzer adapter
func NewAnalyzerAdapter(diffAnalyzer analyzer.DiffAnalyzer) *AnalyzerAdapter {
	return &AnalyzerAdapter{
		analyzer: diffAnalyzer,
	}
}

// NewDefaultAnalyzerAdapter creates a new analyzer adapter with default implementation
func NewDefaultAnalyzerAdapter() *AnalyzerAdapter {
	return &AnalyzerAdapter{
		analyzer: analyzer.NewDefaultDiffAnalyzer(),
	}
}

// ParseDiff implements CodeAnalyzer interface
func (a *AnalyzerAdapter) ParseDiff(rawDiff string) (*analyzer.ParsedDiff, error) {
	return a.analyzer.ParseDiff(rawDiff)
}

// ExtractContext implements CodeAnalyzer interface
func (a *AnalyzerAdapter) ExtractContext(parsedDiff *analyzer.ParsedDiff, contextLines int) (*analyzer.ContextualDiff, error) {
	return a.analyzer.ExtractContext(parsedDiff, contextLines)
}