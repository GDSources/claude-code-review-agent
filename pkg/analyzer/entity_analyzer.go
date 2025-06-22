package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FlattenedCodebase represents the entire codebase in a format suitable for AI analysis
type FlattenedCodebase struct {
	Files       []FileContent `json:"files"`
	Summary     string        `json:"summary"`
	TotalFiles  int           `json:"total_files"`
	TotalLines  int           `json:"total_lines"`
	Languages   []string      `json:"languages"`
	ProjectInfo ProjectInfo   `json:"project_info"`
}

// FileContent represents a single file's content and metadata
type FileContent struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Language     string `json:"language"`
	Content      string `json:"content"`
	LineCount    int    `json:"line_count"`
	Size         int    `json:"size"`
}

// ProjectInfo contains metadata about the project
type ProjectInfo struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "go", "node", "python", etc.
	MainFiles   []string          `json:"main_files"`
	ConfigFiles []string          `json:"config_files"`
	Structure   map[string]string `json:"structure"` // directory -> purpose
}

// DeletionAnalysisRequest represents a request to analyze code deletions
type DeletionAnalysisRequest struct {
	Codebase       *FlattenedCodebase `json:"codebase"`
	DeletedContent []DeletedCode      `json:"deleted_content"`
	Context        string             `json:"context"`
}

// DeletedCode represents code that was deleted
type DeletedCode struct {
	File       string `json:"file"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Language   string `json:"language"`
	ChangeType string `json:"change_type"` // "deleted", "renamed", "moved"
}

// DeletionAnalysisResult represents the AI's analysis of code deletions
type DeletionAnalysisResult struct {
	OrphanedReferences []OrphanedReference `json:"orphaned_references"`
	SafeDeletions      []string            `json:"safe_deletions"`
	Warnings           []Warning           `json:"warnings"`
	Summary            string              `json:"summary"`
	Confidence         float64             `json:"confidence"`
}

// OrphanedReference represents a reference to deleted code that may cause issues
type OrphanedReference struct {
	DeletedEntity    string `json:"deleted_entity"`
	ReferencingFile  string `json:"referencing_file"`
	ReferencingLines []int  `json:"referencing_lines"`
	ReferenceType    string `json:"reference_type"` // "function_call", "import", "type_usage", etc.
	Context          string `json:"context"`
	Severity         string `json:"severity"` // "error", "warning", "info"
	Suggestion       string `json:"suggestion"`
}

// Warning represents a potential issue found during analysis
type Warning struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	File       string `json:"file,omitempty"`
	LineNumber int    `json:"line_number,omitempty"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion,omitempty"`
}

// CodebaseFlattener flattens a codebase for AI analysis
type CodebaseFlattener interface {
	// FlattenWorkspace flattens all relevant repository files
	FlattenWorkspace(workspacePath string) (*FlattenedCodebase, error)

	// FlattenDiff flattens only files affected by a diff
	FlattenDiff(workspacePath string, diff *ParsedDiff) (*FlattenedCodebase, error)
}

// DeletionAnalyzer analyzes code deletions using AI
type DeletionAnalyzer interface {
	// AnalyzeDeletions analyzes deleted code and finds potential issues
	AnalyzeDeletions(request *DeletionAnalysisRequest) (*DeletionAnalysisResult, error)
}

// AIContextBuilder formats codebase and deletion data for LLM consumption
type AIContextBuilder interface {
	// BuildContext creates a formatted context for AI analysis
	BuildContext(request *DeletionAnalysisRequest) (*AIAnalysisContext, error)
}

// AIAnalysisContext represents the formatted context for AI analysis
type AIAnalysisContext struct {
	SystemPrompt    string                 `json:"system_prompt"`
	UserPrompt      string                 `json:"user_prompt"`
	CodebaseContext string                 `json:"codebase_context"`
	DeletionContext string                 `json:"deletion_context"`
	Instructions    string                 `json:"instructions"`
	ExpectedFormat  map[string]interface{} `json:"expected_format"`
}

// LLMClient interface for sending deletion analysis requests to AI services
type LLMClient interface {
	AnalyzeDeletions(ctx context.Context, aiContext *AIAnalysisContext) (*DeletionAnalysisResult, error)
}

// DefaultCodebaseFlattener implements CodebaseFlattener
type DefaultCodebaseFlattener struct {
	includedExtensions []string
	excludedPaths      []string
	maxFileSize        int64
}

// NewDefaultCodebaseFlattener creates a new codebase flattener
func NewDefaultCodebaseFlattener() *DefaultCodebaseFlattener {
	return &DefaultCodebaseFlattener{
		includedExtensions: []string{
			".go", ".js", ".ts", ".tsx", ".jsx", ".py", ".java", ".cpp", ".c", ".h", ".hpp",
			".rs", ".rb", ".php", ".cs", ".swift", ".kt", ".scala", ".sh", ".sql",
			".json", ".yaml", ".yml", ".toml", ".md", ".txt",
		},
		excludedPaths: []string{
			"node_modules", "vendor", ".git", "dist", "build", "target", ".next",
			".nuxt", "coverage", ".nyc_output", "tmp", "temp", "logs", ".cache",
			"__pycache__", ".pytest_cache", ".venv", "venv", "env",
		},
		maxFileSize: 1024 * 1024, // 1MB max file size
	}
}

// FlattenWorkspace flattens all relevant repository files
func (cf *DefaultCodebaseFlattener) FlattenWorkspace(workspacePath string) (*FlattenedCodebase, error) {
	var files []FileContent
	var totalLines int
	languageSet := make(map[string]bool)

	err := filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Check if this directory should be excluded
			dirName := filepath.Base(path)
			if slices.Contains(cf.excludedPaths, dirName) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file size
		if info.Size() > cf.maxFileSize {
			return nil // Skip large files
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(cf.includedExtensions, ext) {
			return nil
		}

		// Check if path contains excluded directories
		relPath, err := filepath.Rel(workspacePath, path)
		if err != nil {
			return err
		}
		for _, excluded := range cf.excludedPaths {
			if strings.Contains(relPath, excluded) {
				return nil
			}
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		language := detectLanguage(info.Name())
		languageSet[language] = true

		lineCount := strings.Count(string(content), "\n") + 1
		totalLines += lineCount

		files = append(files, FileContent{
			Path:         path,
			RelativePath: relPath,
			Language:     language,
			Content:      string(content),
			LineCount:    lineCount,
			Size:         int(info.Size()),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk workspace: %w", err)
	}

	// Convert language set to slice
	languages := make([]string, 0, len(languageSet))
	for lang := range languageSet {
		languages = append(languages, lang)
	}

	// Detect project info
	projectInfo := cf.detectProjectInfo(workspacePath, files)

	return &FlattenedCodebase{
		Files:       files,
		TotalFiles:  len(files),
		TotalLines:  totalLines,
		Languages:   languages,
		ProjectInfo: projectInfo,
		Summary:     cf.generateSummary(files, languages),
	}, nil
}

// FlattenDiff flattens only files affected by a diff
func (cf *DefaultCodebaseFlattener) FlattenDiff(workspacePath string, diff *ParsedDiff) (*FlattenedCodebase, error) {
	var files []FileContent
	var totalLines int
	languageSet := make(map[string]bool)

	for _, fileDiff := range diff.Files {
		fullPath := filepath.Join(workspacePath, fileDiff.Filename)

		// Check if file exists (might be deleted)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		// Read file content
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue // Skip files we can't read
		}

		language := detectLanguage(fileDiff.Filename)
		languageSet[language] = true

		lineCount := strings.Count(string(content), "\n") + 1
		totalLines += lineCount

		files = append(files, FileContent{
			Path:         fullPath,
			RelativePath: fileDiff.Filename,
			Language:     language,
			Content:      string(content),
			LineCount:    lineCount,
			Size:         len(content),
		})
	}

	// Convert language set to slice
	languages := make([]string, 0, len(languageSet))
	for lang := range languageSet {
		languages = append(languages, lang)
	}

	// Detect project info
	projectInfo := cf.detectProjectInfo(workspacePath, files)

	return &FlattenedCodebase{
		Files:       files,
		TotalFiles:  len(files),
		TotalLines:  totalLines,
		Languages:   languages,
		ProjectInfo: projectInfo,
		Summary:     cf.generateSummary(files, languages),
	}, nil
}

// detectProjectInfo analyzes the codebase to determine project type and structure
func (cf *DefaultCodebaseFlattener) detectProjectInfo(workspacePath string, files []FileContent) ProjectInfo {
	info := ProjectInfo{
		Name:        filepath.Base(workspacePath),
		Structure:   make(map[string]string),
		MainFiles:   []string{},
		ConfigFiles: []string{},
	}

	// Detect project type based on config files and main files
	for _, file := range files {
		fileName := filepath.Base(file.RelativePath)

		switch fileName {
		case "go.mod", "main.go":
			info.Type = "go"
			info.MainFiles = append(info.MainFiles, file.RelativePath)
		case "package.json":
			if info.Type == "" {
				info.Type = "node"
			}
			info.ConfigFiles = append(info.ConfigFiles, file.RelativePath)
		case "tsconfig.json", "webpack.config.js":
			if info.Type == "" {
				info.Type = "node"
			}
			info.ConfigFiles = append(info.ConfigFiles, file.RelativePath)
		case "requirements.txt", "setup.py", "pyproject.toml":
			if info.Type == "" {
				info.Type = "python"
			}
			info.ConfigFiles = append(info.ConfigFiles, file.RelativePath)
		case "Cargo.toml":
			if info.Type == "" {
				info.Type = "rust"
			}
			info.ConfigFiles = append(info.ConfigFiles, file.RelativePath)
		}

		// Analyze directory structure
		dir := filepath.Dir(file.RelativePath)
		if dir != "." {
			topLevel := strings.Split(dir, string(filepath.Separator))[0]
			switch topLevel {
			case "src", "lib":
				info.Structure[topLevel] = "source code"
			case "test", "tests", "__tests__":
				info.Structure[topLevel] = "tests"
			case "docs", "documentation":
				info.Structure[topLevel] = "documentation"
			case "examples", "example":
				info.Structure[topLevel] = "examples"
			case "cmd":
				info.Structure[topLevel] = "command line tools"
			case "pkg":
				info.Structure[topLevel] = "packages/libraries"
			case "internal":
				info.Structure[topLevel] = "internal packages"
			}
		}
	}

	return info
}

// generateSummary creates a summary of the codebase
func (cf *DefaultCodebaseFlattener) generateSummary(files []FileContent, languages []string) string {
	if len(files) == 0 {
		return "Empty codebase"
	}

	return fmt.Sprintf("Codebase with %d files in %d languages: %s",
		len(files), len(languages), strings.Join(languages, ", "))
}

// extractDeletedContent extracts deleted code from a diff
func extractDeletedContent(diff *ParsedDiff) []DeletedCode {
	var deletedContent []DeletedCode

	for _, file := range diff.Files {
		if file.Status == "deleted" {
			// Entire file was deleted
			var content strings.Builder
			var startLine, endLine int

			for _, hunk := range file.Hunks {
				for _, line := range hunk.Lines {
					if line.Type == "removed" {
						if startLine == 0 {
							startLine = line.OldLineNo
						}
						endLine = line.OldLineNo
						content.WriteString(line.Content + "\n")
					}
				}
			}

			if content.Len() > 0 {
				deletedContent = append(deletedContent, DeletedCode{
					File:       file.Filename,
					Content:    strings.TrimSuffix(content.String(), "\n"),
					StartLine:  startLine,
					EndLine:    endLine,
					Language:   file.Language,
					ChangeType: "deleted",
				})
			}
		} else {
			// File was modified, extract deleted sections
			for _, hunk := range file.Hunks {
				var content strings.Builder
				var startLine, endLine int
				var hasRemovedContent bool

				for _, line := range hunk.Lines {
					if line.Type == "removed" {
						if startLine == 0 {
							startLine = line.OldLineNo
						}
						endLine = line.OldLineNo
						content.WriteString(line.Content + "\n")
						hasRemovedContent = true
					}
				}

				if hasRemovedContent {
					deletedContent = append(deletedContent, DeletedCode{
						File:       file.Filename,
						Content:    strings.TrimSuffix(content.String(), "\n"),
						StartLine:  startLine,
						EndLine:    endLine,
						Language:   file.Language,
						ChangeType: "deleted",
					})
				}
			}
		}
	}

	return deletedContent
}

// DefaultDeletionAnalyzer implements DeletionAnalyzer using AI
type DefaultDeletionAnalyzer struct {
	contextBuilder AIContextBuilder
	llmClient      LLMClient // Interface for LLM communication
}

// NewDefaultDeletionAnalyzer creates a new AI-based deletion analyzer with heuristics
func NewDefaultDeletionAnalyzer() *DefaultDeletionAnalyzer {
	return &DefaultDeletionAnalyzer{
		contextBuilder: NewDefaultAIContextBuilder(),
		llmClient:      nil, // Use heuristics when no LLM client is provided
	}
}

// NewDeletionAnalyzerWithLLM creates a new AI-based deletion analyzer with LLM integration
func NewDeletionAnalyzerWithLLM(llmClient LLMClient) *DefaultDeletionAnalyzer {
	return &DefaultDeletionAnalyzer{
		contextBuilder: NewDefaultAIContextBuilder(),
		llmClient:      llmClient,
	}
}

// AnalyzeDeletions analyzes deleted code and finds potential issues using AI
func (da *DefaultDeletionAnalyzer) AnalyzeDeletions(request *DeletionAnalysisRequest) (*DeletionAnalysisResult, error) {
	return da.AnalyzeDeletionsWithContext(context.Background(), request)
}

// AnalyzeDeletionsWithContext analyzes deleted code with context support
func (da *DefaultDeletionAnalyzer) AnalyzeDeletionsWithContext(ctx context.Context, request *DeletionAnalysisRequest) (*DeletionAnalysisResult, error) {
	// Build AI context for analysis
	aiContext, err := da.contextBuilder.BuildContext(request)
	if err != nil {
		return nil, fmt.Errorf("failed to build AI context: %w", err)
	}

	// Use LLM client if available, otherwise fallback to heuristics
	if da.llmClient != nil {
		return da.llmClient.AnalyzeDeletions(ctx, aiContext)
	}

	// Fallback to enhanced heuristics with AI context awareness
	var orphanedRefs []OrphanedReference
	var warnings []Warning

	// Log AI context for debugging (in real implementation, this would be sent to LLM)
	contextLength := len(aiContext.SystemPrompt) + len(aiContext.UserPrompt) +
		len(aiContext.CodebaseContext) + len(aiContext.DeletionContext)

	// Enhanced analysis using AI context structure
	for _, deleted := range request.DeletedContent {
		// Use enhanced reference finding with context awareness
		refs := da.findPotentialReferencesEnhanced(deleted, request.Codebase, aiContext)
		orphanedRefs = append(orphanedRefs, refs...)
	}

	// Generate warnings for common issues
	if len(orphanedRefs) > 0 {
		warnings = append(warnings, Warning{
			Type:       "orphaned_references",
			Message:    fmt.Sprintf("Found %d potential orphaned references", len(orphanedRefs)),
			Severity:   "warning",
			Suggestion: "Review the identified references and either remove them or provide alternative implementations",
		})
	}

	// Enhanced confidence calculation based on AI context
	confidence := da.calculateConfidenceWithContext(request, orphanedRefs, contextLength)

	summary := fmt.Sprintf("Heuristic analysis of %d deleted code sections across %d files. Found %d potential issues.",
		len(request.DeletedContent), len(request.Codebase.Files), len(orphanedRefs))

	return &DeletionAnalysisResult{
		OrphanedReferences: orphanedRefs,
		SafeDeletions:      da.identifySafeDeletions(request.DeletedContent, orphanedRefs),
		Warnings:           warnings,
		Summary:            summary,
		Confidence:         confidence,
	}, nil
}

// findPotentialReferences is a simple heuristic-based reference finder
// In a real implementation, this would be handled by the AI
func (da *DefaultDeletionAnalyzer) findPotentialReferences(deleted DeletedCode, codebase *FlattenedCodebase) []OrphanedReference {
	var references []OrphanedReference

	// Extract simple identifiers from deleted content (very basic)
	identifiers := da.extractIdentifiers(deleted.Content, deleted.Language)

	// Search for these identifiers in the codebase
	for _, file := range codebase.Files {
		if file.RelativePath == deleted.File {
			continue // Skip the file where content was deleted
		}

		lines := strings.Split(file.Content, "\n")
		for lineNum, line := range lines {
			for _, identifier := range identifiers {
				if strings.Contains(line, identifier) {
					references = append(references, OrphanedReference{
						DeletedEntity:    identifier,
						ReferencingFile:  file.RelativePath,
						ReferencingLines: []int{lineNum + 1},
						ReferenceType:    "potential_usage",
						Context:          strings.TrimSpace(line),
						Severity:         "warning",
						Suggestion:       fmt.Sprintf("Verify if '%s' is still needed after deletion", identifier),
					})
				}
			}
		}
	}

	return references
}

// extractIdentifiers extracts potential identifiers from code content
// This is a very simple implementation - AI would do this much better
func (da *DefaultDeletionAnalyzer) extractIdentifiers(content string, language string) []string {
	var identifiers []string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		// Very basic identifier extraction based on language
		switch language {
		case "go":
			// Look for function declarations: func FunctionName(
			if strings.HasPrefix(line, "func ") && strings.Contains(line, "(") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					funcName := strings.Split(parts[1], "(")[0]
					// Remove receiver if present: func (r *Type) Method
					if strings.HasPrefix(funcName, "(") {
						if len(parts) >= 3 {
							funcName = strings.Split(parts[2], "(")[0]
						}
					}
					if funcName != "" {
						identifiers = append(identifiers, funcName)
					}
				}
			}
			// Look for type declarations: type TypeName
			if strings.HasPrefix(line, "type ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					identifiers = append(identifiers, parts[1])
				}
			}
		case "javascript", "typescript":
			// Look for function declarations
			if strings.Contains(line, "function ") {
				// Extract function name
				start := strings.Index(line, "function ") + 9
				remaining := line[start:]
				if idx := strings.Index(remaining, "("); idx > 0 {
					funcName := strings.TrimSpace(remaining[:idx])
					if funcName != "" {
						identifiers = append(identifiers, funcName)
					}
				}
			}
			// Look for class declarations
			if strings.Contains(line, "class ") {
				start := strings.Index(line, "class ") + 6
				remaining := line[start:]
				if idx := strings.IndexAny(remaining, " {"); idx > 0 {
					className := strings.TrimSpace(remaining[:idx])
					if className != "" {
						identifiers = append(identifiers, className)
					}
				}
			}
		}
	}

	return identifiers
}

// findPotentialReferencesEnhanced uses AI context to improve reference detection
func (da *DefaultDeletionAnalyzer) findPotentialReferencesEnhanced(deleted DeletedCode, codebase *FlattenedCodebase, aiContext *AIAnalysisContext) []OrphanedReference {
	// This enhanced version could use the AI context to improve detection
	// For now, we'll use the existing logic but with enhanced metadata
	references := da.findPotentialReferences(deleted, codebase)

	// Enhance references with better suggestions based on context
	for i := range references {
		references[i].Suggestion = da.generateEnhancedSuggestion(references[i], deleted, aiContext)
	}

	return references
}

// calculateConfidenceWithContext calculates confidence based on AI context
func (da *DefaultDeletionAnalyzer) calculateConfidenceWithContext(request *DeletionAnalysisRequest, orphanedRefs []OrphanedReference, contextLength int) float64 {
	confidence := 0.6 // Base confidence for AI-enhanced analysis

	// Higher confidence with more comprehensive context
	if contextLength > 10000 {
		confidence += 0.1
	}

	// Higher confidence with larger codebase (more data points)
	if len(request.Codebase.Files) > 10 {
		confidence += 0.1
	}

	// Higher confidence if project type is well-supported
	if request.Codebase.ProjectInfo.Type == "go" || request.Codebase.ProjectInfo.Type == "javascript" || request.Codebase.ProjectInfo.Type == "typescript" {
		confidence += 0.1
	}

	// Adjust based on findings
	if len(orphanedRefs) == 0 {
		confidence += 0.1 // More confident when no issues found
	} else if len(orphanedRefs) > 10 {
		confidence -= 0.1 // Less confident with many findings (might be false positives)
	}

	// Ensure confidence stays within bounds
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// identifySafeDeletions identifies deleted entities that appear safe to remove
func (da *DefaultDeletionAnalyzer) identifySafeDeletions(deletedContent []DeletedCode, orphanedRefs []OrphanedReference) []string {
	var safeDeletions []string

	// Create a map of entities that have orphaned references
	orphanedEntities := make(map[string]bool)
	for _, ref := range orphanedRefs {
		orphanedEntities[ref.DeletedEntity] = true
	}

	// Extract entities from deleted content and check if they're referenced
	for _, deleted := range deletedContent {
		entities := da.extractIdentifiers(deleted.Content, deleted.Language)
		for _, entity := range entities {
			if !orphanedEntities[entity] {
				safeDeletions = append(safeDeletions, entity)
			}
		}
	}

	return safeDeletions
}

// generateEnhancedSuggestion creates better suggestions using AI context
func (da *DefaultDeletionAnalyzer) generateEnhancedSuggestion(ref OrphanedReference, deleted DeletedCode, aiContext *AIAnalysisContext) string {
	baseSuggestion := ref.Suggestion

	// Enhance suggestion based on reference type and context
	switch ref.ReferenceType {
	case "potential_usage":
		if strings.Contains(deleted.Content, "func ") {
			return fmt.Sprintf("Remove the call to '%s' or replace with an alternative function. Consider refactoring the code to handle the missing functionality.", ref.DeletedEntity)
		}
		if strings.Contains(deleted.Content, "type ") || strings.Contains(deleted.Content, "class ") {
			return fmt.Sprintf("Remove references to type '%s' or replace with an alternative type. Update variable declarations and type annotations.", ref.DeletedEntity)
		}
	}

	// Add context-specific suggestions
	if strings.Contains(aiContext.DeletionContext, "refactor") {
		return baseSuggestion + " This appears to be part of a refactoring effort."
	}

	return baseSuggestion
}

// DefaultAIContextBuilder implements AIContextBuilder
type DefaultAIContextBuilder struct {
	maxCodebaseLength int
	maxDeletionLength int
}

// NewDefaultAIContextBuilder creates a new AI context builder
func NewDefaultAIContextBuilder() *DefaultAIContextBuilder {
	return &DefaultAIContextBuilder{
		maxCodebaseLength: 50000, // 50K characters max
		maxDeletionLength: 10000, // 10K characters max
	}
}

// BuildContext creates a formatted context for AI analysis
func (cb *DefaultAIContextBuilder) BuildContext(request *DeletionAnalysisRequest) (*AIAnalysisContext, error) {
	systemPrompt := cb.createSystemPrompt()
	userPrompt := cb.createUserPrompt(request.Context)
	codebaseContext := cb.formatCodebaseContext(request.Codebase)
	deletionContext := cb.formatDeletionContext(request.DeletedContent, request.Context)
	instructions := cb.createInstructions()
	expectedFormat := cb.createExpectedFormat()

	return &AIAnalysisContext{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		CodebaseContext: codebaseContext,
		DeletionContext: deletionContext,
		Instructions:    instructions,
		ExpectedFormat:  expectedFormat,
	}, nil
}

// createSystemPrompt creates the system prompt for AI analysis
func (cb *DefaultAIContextBuilder) createSystemPrompt() string {
	return `You are an expert code analyst specializing in detecting orphaned references and analyzing code deletion safety.

Your task is to analyze a codebase and identify any references to deleted code that may cause compilation errors, runtime issues, or broken functionality.

You have expertise in multiple programming languages and can identify:
- Function calls to deleted functions
- Imports of deleted modules/packages
- Usage of deleted types, classes, interfaces
- References to deleted constants, variables
- Inheritance from deleted base classes
- Implementation of deleted interfaces
- Usage of deleted decorators, annotations
- References in comments that may indicate dependencies

You should consider the context of the deletion and provide actionable suggestions for resolving any issues found.`
}

// createUserPrompt creates the user prompt based on the change context
func (cb *DefaultAIContextBuilder) createUserPrompt(context string) string {
	prompt := "Please analyze the provided codebase and deleted code to identify any orphaned references that may cause issues."

	if context != "" {
		prompt += fmt.Sprintf("\n\nContext of the changes: %s", context)
	}

	prompt += "\n\nProvide your analysis in the specified JSON format, focusing on actionable issues and suggestions for resolution."

	return prompt
}

// formatCodebaseContext formats the codebase for AI consumption
func (cb *DefaultAIContextBuilder) formatCodebaseContext(codebase *FlattenedCodebase) string {
	var builder strings.Builder

	// Project overview
	builder.WriteString("# Codebase Analysis\n\n")
	builder.WriteString(fmt.Sprintf("## Project Overview\n"))
	builder.WriteString(fmt.Sprintf("- **Type**: %s\n", codebase.ProjectInfo.Type))
	builder.WriteString(fmt.Sprintf("- **Name**: %s\n", codebase.ProjectInfo.Name))
	builder.WriteString(fmt.Sprintf("- **Total Files**: %d\n", codebase.TotalFiles))
	builder.WriteString(fmt.Sprintf("- **Total Lines**: %d\n", codebase.TotalLines))
	builder.WriteString(fmt.Sprintf("- **Languages**: %s\n", strings.Join(codebase.Languages, ", ")))
	builder.WriteString(fmt.Sprintf("- **Summary**: %s\n\n", codebase.Summary))

	// Directory structure
	if len(codebase.ProjectInfo.Structure) > 0 {
		builder.WriteString("## Directory Structure\n")
		for dir, purpose := range codebase.ProjectInfo.Structure {
			builder.WriteString(fmt.Sprintf("- **%s/**: %s\n", dir, purpose))
		}
		builder.WriteString("\n")
	}

	// Files content
	builder.WriteString("## Files\n\n")

	currentLength := builder.Len()
	for _, file := range codebase.Files {
		fileSection := fmt.Sprintf("### %s (%s)\n```%s\n%s\n```\n\n",
			file.RelativePath, file.Language, file.Language, file.Content)

		// Check if adding this file would exceed limit
		if currentLength+len(fileSection) > cb.maxCodebaseLength {
			builder.WriteString(fmt.Sprintf("### %s (%s)\n[File content truncated - too large for analysis]\n\n",
				file.RelativePath, file.Language))
		} else {
			builder.WriteString(fileSection)
			currentLength += len(fileSection)
		}
	}

	return builder.String()
}

// formatDeletionContext formats the deleted code for AI consumption
func (cb *DefaultAIContextBuilder) formatDeletionContext(deletedContent []DeletedCode, context string) string {
	var builder strings.Builder

	builder.WriteString("# Deleted Code Analysis\n\n")

	if context != "" {
		builder.WriteString(fmt.Sprintf("## Context\n%s\n\n", context))
	}

	builder.WriteString("## Deleted Code Sections\n\n")

	currentLength := builder.Len()
	for i, deleted := range deletedContent {
		var rangeInfo string
		if deleted.StartLine == deleted.EndLine {
			rangeInfo = fmt.Sprintf("line %d", deleted.StartLine)
		} else {
			rangeInfo = fmt.Sprintf("lines %d-%d", deleted.StartLine, deleted.EndLine)
		}

		deleteSection := fmt.Sprintf("### %d. %s (%s, %s)\n**Type**: %s\n**Location**: %s\n\n```%s\n%s\n```\n\n",
			i+1, deleted.File, deleted.Language, rangeInfo, deleted.ChangeType, rangeInfo, deleted.Language, deleted.Content)

		// Check if adding this section would exceed limit
		if currentLength+len(deleteSection) > cb.maxDeletionLength {
			builder.WriteString(fmt.Sprintf("### %d. %s (%s, %s)\n[Content truncated - too large for analysis]\n\n",
				i+1, deleted.File, deleted.Language, rangeInfo))
		} else {
			builder.WriteString(deleteSection)
			currentLength += len(deleteSection)
		}
	}

	return builder.String()
}

// createInstructions creates detailed instructions for the AI
func (cb *DefaultAIContextBuilder) createInstructions() string {
	return `## Analysis Instructions

1. **Scan the codebase** for any references to the deleted entities (functions, classes, types, variables, etc.)

2. **Identify orphaned references** that would cause issues:
   - Function calls to deleted functions
   - Imports/requires of deleted modules
   - Type references to deleted types/classes/interfaces
   - Variable/constant references
   - Inheritance or interface implementation
   - Generic type parameters
   - Decorator/annotation usage

3. **Assess severity**:
   - **error**: Will cause compilation/runtime errors
   - **warning**: May cause issues or indicate needed cleanup
   - **info**: Low-impact references that may need attention

4. **Provide suggestions** for each issue:
   - Remove the reference if no longer needed
   - Replace with alternative implementation
   - Update import statements
   - Refactor code to handle missing dependency

5. **Calculate confidence** (0.0-1.0) based on:
   - Completeness of analysis
   - Clarity of references found
   - Ambiguity in identifier matching

6. **Respond in valid JSON** format matching the expected structure exactly.`
}

// createExpectedFormat creates the expected JSON response format
func (cb *DefaultAIContextBuilder) createExpectedFormat() map[string]interface{} {
	return map[string]interface{}{
		"orphaned_references": []map[string]interface{}{
			{
				"deleted_entity":    "string - name of the deleted function/class/type",
				"referencing_file":  "string - file containing the reference",
				"referencing_lines": []int{0}, // "array of line numbers where references occur"
				"reference_type":    "string - type of reference (function_call, import, type_usage, etc.)",
				"context":           "string - surrounding code context",
				"severity":          "string - error|warning|info",
				"suggestion":        "string - recommended action to resolve the issue",
			},
		},
		"safe_deletions": []string{
			"string - entities that appear safe to delete",
		},
		"warnings": []map[string]interface{}{
			{
				"type":        "string - warning type",
				"message":     "string - warning message",
				"file":        "string - file path (optional)",
				"line_number": 0, // "number - line number (optional)"
				"severity":    "string - warning|error|info",
				"suggestion":  "string - recommended action (optional)",
			},
		},
		"summary":    "string - brief summary of the analysis",
		"confidence": 0.0, // "number - confidence score between 0.0 and 1.0"
	}
}
