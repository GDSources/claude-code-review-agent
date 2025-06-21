package analyzer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DiffAnalyzer processes GitHub diffs and extracts structured information for LLM analysis
type DiffAnalyzer interface {
	ParseDiff(rawDiff string) (*ParsedDiff, error)
	ExtractContext(parsedDiff *ParsedDiff, contextLines int) (*ContextualDiff, error)
}

// FileDiff represents changes to a single file
type FileDiff struct {
	Filename    string     `json:"filename"`
	Status      string     `json:"status"`       // "added", "modified", "deleted", "renamed"
	OldFilename string     `json:"old_filename"` // For renamed files
	Hunks       []DiffHunk `json:"hunks"`
	Additions   int        `json:"additions"`
	Deletions   int        `json:"deletions"`
	Language    string     `json:"language"` // Detected programming language
}

// DiffHunk represents a contiguous section of changes in a file
type DiffHunk struct {
	OldStart int        `json:"old_start"`
	OldCount int        `json:"old_count"`
	NewStart int        `json:"new_start"`
	NewCount int        `json:"new_count"`
	Lines    []DiffLine `json:"lines"`
	Header   string     `json:"header"`
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	Type      string `json:"type"` // "context", "added", "removed"
	Content   string `json:"content"`
	OldLineNo int    `json:"old_line_no"`
	NewLineNo int    `json:"new_line_no"`
}

// ParsedDiff represents the complete parsed diff
type ParsedDiff struct {
	Files        []FileDiff `json:"files"`
	TotalFiles   int        `json:"total_files"`
	TotalAdded   int        `json:"total_added"`
	TotalRemoved int        `json:"total_removed"`
}

// ContextualDiff includes surrounding code context for LLM analysis
type ContextualDiff struct {
	*ParsedDiff
	FilesWithContext []FileWithContext `json:"files_with_context"`
}

// FileWithContext includes surrounding code context around changes
type FileWithContext struct {
	FileDiff
	ContextBlocks []ContextBlock `json:"context_blocks"`
}

// ContextBlock represents a block of code with surrounding context
type ContextBlock struct {
	StartLine   int        `json:"start_line"`
	EndLine     int        `json:"end_line"`
	Lines       []DiffLine `json:"lines"`
	ChangeType  string     `json:"change_type"` // "addition", "deletion", "modification"
	Description string     `json:"description"` // Human-readable description
}

// DefaultDiffAnalyzer implements the DiffAnalyzer interface
type DefaultDiffAnalyzer struct{}

// NewDefaultDiffAnalyzer creates a new default diff analyzer
func NewDefaultDiffAnalyzer() *DefaultDiffAnalyzer {
	return &DefaultDiffAnalyzer{}
}

// ParseDiff parses a unified diff string into structured data
func (d *DefaultDiffAnalyzer) ParseDiff(rawDiff string) (*ParsedDiff, error) {
	if rawDiff == "" {
		return &ParsedDiff{Files: []FileDiff{}, TotalFiles: 0}, nil
	}

	lines := strings.Split(rawDiff, "\n")
	var files []FileDiff
	var currentFile *FileDiff
	var currentHunk *DiffHunk
	var oldLineNo, newLineNo int

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			// Start of a new file
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				}
				files = append(files, *currentFile)
			}

			filename, err := extractFilename(line)
			if err != nil {
				return nil, fmt.Errorf("failed to extract filename from line %d: %w", i+1, err)
			}

			currentFile = &FileDiff{
				Filename: filename,
				Status:   "modified",
				Hunks:    []DiffHunk{},
				Language: detectLanguage(filename),
			}
			currentHunk = nil

		case strings.HasPrefix(line, "new file mode"):
			if currentFile != nil {
				currentFile.Status = "added"
			}

		case strings.HasPrefix(line, "deleted file mode"):
			if currentFile != nil {
				currentFile.Status = "deleted"
			}

		case strings.HasPrefix(line, "rename from"):
			if currentFile != nil {
				currentFile.Status = "renamed"
				currentFile.OldFilename = strings.TrimPrefix(line, "rename from ")
			}

		case strings.HasPrefix(line, "@@"):
			// Start of a new hunk
			if currentFile != nil && currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("failed to parse hunk header at line %d: %w", i+1, err)
			}

			currentHunk = hunk
			oldLineNo = hunk.OldStart
			newLineNo = hunk.NewStart

		case len(line) > 0 && (line[0] == ' ' || line[0] == '+' || line[0] == '-'):
			// Diff content line
			if currentHunk != nil {
				diffLine := DiffLine{
					Content: line[1:], // Remove the prefix character
				}

				switch line[0] {
				case ' ':
					diffLine.Type = "context"
					diffLine.OldLineNo = oldLineNo
					diffLine.NewLineNo = newLineNo
					oldLineNo++
					newLineNo++
				case '+':
					diffLine.Type = "added"
					diffLine.NewLineNo = newLineNo
					newLineNo++
					if currentFile != nil {
						currentFile.Additions++
					}
				case '-':
					diffLine.Type = "removed"
					diffLine.OldLineNo = oldLineNo
					oldLineNo++
					if currentFile != nil {
						currentFile.Deletions++
					}
				}

				currentHunk.Lines = append(currentHunk.Lines, diffLine)
			}
		}
	}

	// Add the last file and hunk
	if currentFile != nil {
		if currentHunk != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		}
		files = append(files, *currentFile)
	}

	// Calculate totals
	totalAdded, totalRemoved := 0, 0
	for _, file := range files {
		totalAdded += file.Additions
		totalRemoved += file.Deletions
	}

	return &ParsedDiff{
		Files:        files,
		TotalFiles:   len(files),
		TotalAdded:   totalAdded,
		TotalRemoved: totalRemoved,
	}, nil
}

// ExtractContext adds surrounding code context to the parsed diff
func (d *DefaultDiffAnalyzer) ExtractContext(parsedDiff *ParsedDiff, contextLines int) (*ContextualDiff, error) {
	if contextLines < 0 {
		contextLines = 5 // Default to 5 lines of context as mentioned in CLAUDE.md
	}

	filesWithContext := make([]FileWithContext, len(parsedDiff.Files))

	for i, file := range parsedDiff.Files {
		fileWithContext := FileWithContext{
			FileDiff:      file,
			ContextBlocks: []ContextBlock{},
		}

		// Extract context blocks from each hunk
		for _, hunk := range file.Hunks {
			blocks := extractContextBlocks(hunk, contextLines)
			fileWithContext.ContextBlocks = append(fileWithContext.ContextBlocks, blocks...)
		}

		filesWithContext[i] = fileWithContext
	}

	return &ContextualDiff{
		ParsedDiff:       parsedDiff,
		FilesWithContext: filesWithContext,
	}, nil
}

// Helper functions

// extractFilename extracts the filename from a "diff --git" line
func extractFilename(line string) (string, error) {
	// Format: "diff --git a/path/to/file b/path/to/file"
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid diff header format")
	}

	// Extract filename from "a/path/to/file"
	aPath := parts[2]
	if strings.HasPrefix(aPath, "a/") {
		return aPath[2:], nil
	}

	return aPath, nil
}

// parseHunkHeader parses a hunk header line like "@@ -1,4 +1,6 @@"
func parseHunkHeader(line string) (*DiffHunk, error) {
	// Regular expression to parse hunk header
	re := regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)
	matches := re.FindStringSubmatch(line)

	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid hunk header format: %s", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid old start line number: %s", matches[1])
	}

	oldCount := 1
	if matches[2] != "" {
		oldCount, err = strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("invalid old line count: %s", matches[2])
		}
	}

	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("invalid new start line number: %s", matches[3])
	}

	newCount := 1
	if matches[4] != "" {
		newCount, err = strconv.Atoi(matches[4])
		if err != nil {
			return nil, fmt.Errorf("invalid new line count: %s", matches[4])
		}
	}

	header := line
	if len(matches) > 5 {
		header += matches[5] // Include any additional context after @@
	}

	return &DiffHunk{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
		Lines:    []DiffLine{},
		Header:   header,
	}, nil
}

// detectLanguage detects programming language from filename
func detectLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	langMap := map[string]string{
		".go":    "go",
		".js":    "javascript",
		".ts":    "typescript",
		".tsx":   "typescript",
		".jsx":   "javascript",
		".py":    "python",
		".java":  "java",
		".cpp":   "cpp",
		".c":     "c",
		".h":     "c",
		".hpp":   "cpp",
		".rs":    "rust",
		".rb":    "ruby",
		".php":   "php",
		".cs":    "csharp",
		".swift": "swift",
		".kt":    "kotlin",
		".scala": "scala",
		".sh":    "bash",
		".sql":   "sql",
		".md":    "markdown",
		".yaml":  "yaml",
		".yml":   "yaml",
		".json":  "json",
		".xml":   "xml",
		".html":  "html",
		".css":   "css",
		".scss":  "scss",
		".sass":  "sass",
	}

	if lang, exists := langMap[ext]; exists {
		return lang
	}

	return "plaintext"
}

// extractContextBlocks creates context blocks from a hunk
func extractContextBlocks(hunk DiffHunk, contextLines int) []ContextBlock {
	var blocks []ContextBlock
	var currentBlock *ContextBlock

	for i, line := range hunk.Lines {
		if line.Type == "added" || line.Type == "removed" {
			if currentBlock == nil {
				// Start a new context block
				startIdx := maxInt(0, i-contextLines)
				currentBlock = &ContextBlock{
					Lines: []DiffLine{},
				}

				// Add preceding context
				for j := startIdx; j < i; j++ {
					currentBlock.Lines = append(currentBlock.Lines, hunk.Lines[j])
				}
			}

			currentBlock.Lines = append(currentBlock.Lines, line)

			// Determine change type
			if line.Type == "added" {
				if currentBlock.ChangeType == "" {
					currentBlock.ChangeType = "addition"
				} else if currentBlock.ChangeType == "deletion" {
					currentBlock.ChangeType = "modification"
				}
			} else if line.Type == "removed" {
				if currentBlock.ChangeType == "" {
					currentBlock.ChangeType = "deletion"
				} else if currentBlock.ChangeType == "addition" {
					currentBlock.ChangeType = "modification"
				}
			}
		} else if currentBlock != nil {
			// Context line after changes
			currentBlock.Lines = append(currentBlock.Lines, line)

			// Check if we've reached the end of context
			nextChangeIdx := findNextChange(hunk.Lines, i+1)
			if nextChangeIdx == -1 || nextChangeIdx > i+contextLines {
				// Finalize current block
				finalizeContextBlock(currentBlock)
				blocks = append(blocks, *currentBlock)
				currentBlock = nil
			}
		}
	}

	// Finalize any remaining block
	if currentBlock != nil {
		finalizeContextBlock(currentBlock)
		blocks = append(blocks, *currentBlock)
	}

	return blocks
}

// Helper functions for context extraction

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func findNextChange(lines []DiffLine, startIdx int) int {
	for i := startIdx; i < len(lines); i++ {
		if lines[i].Type == "added" || lines[i].Type == "removed" {
			return i
		}
	}
	return -1
}

func finalizeContextBlock(block *ContextBlock) {
	if len(block.Lines) == 0 {
		return
	}

	// Set line numbers
	for _, line := range block.Lines {
		if line.OldLineNo > 0 {
			if block.StartLine == 0 || line.OldLineNo < block.StartLine {
				block.StartLine = line.OldLineNo
			}
			if line.OldLineNo > block.EndLine {
				block.EndLine = line.OldLineNo
			}
		}
		if line.NewLineNo > 0 {
			if block.StartLine == 0 || line.NewLineNo < block.StartLine {
				block.StartLine = line.NewLineNo
			}
			if line.NewLineNo > block.EndLine {
				block.EndLine = line.NewLineNo
			}
		}
	}

	// Generate description
	switch block.ChangeType {
	case "addition":
		block.Description = fmt.Sprintf("Added %d line(s)", countLinesOfType(block.Lines, "added"))
	case "deletion":
		block.Description = fmt.Sprintf("Removed %d line(s)", countLinesOfType(block.Lines, "removed"))
	case "modification":
		added := countLinesOfType(block.Lines, "added")
		removed := countLinesOfType(block.Lines, "removed")
		block.Description = fmt.Sprintf("Modified code: +%d -%d lines", added, removed)
	default:
		block.Description = "Code change"
	}
}

func countLinesOfType(lines []DiffLine, lineType string) int {
	count := 0
	for _, line := range lines {
		if line.Type == lineType {
			count++
		}
	}
	return count
}
