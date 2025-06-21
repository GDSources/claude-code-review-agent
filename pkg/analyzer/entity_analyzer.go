package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EntityType represents the type of code entity
type EntityType string

const (
	EntityFunction   EntityType = "function"
	EntityMethod     EntityType = "method"
	EntityClass      EntityType = "class"
	EntityStruct     EntityType = "struct"
	EntityInterface  EntityType = "interface"
	EntityTypeAlias  EntityType = "type"
	EntityConstant   EntityType = "constant"
	EntityVariable   EntityType = "variable"
	EntityEnum       EntityType = "enum"
	EntityNamespace  EntityType = "namespace"
	EntityImport     EntityType = "import"
)

// ReferenceType represents how an entity is referenced
type ReferenceType string

const (
	ReferenceCall         ReferenceType = "call"
	ReferenceInstantiation ReferenceType = "instantiation"
	ReferenceTypeUsage    ReferenceType = "type_usage"
	ReferenceInheritance  ReferenceType = "inheritance"
	ReferenceImport       ReferenceType = "import"
	ReferenceAssignment   ReferenceType = "assignment"
)

// CodeEntity represents a code entity that can be deleted
type CodeEntity struct {
	Type       EntityType        `json:"type"`
	Name       string            `json:"name"`
	Language   string            `json:"language"`
	File       string            `json:"file"`
	LineNumber int               `json:"line_number"`
	Signature  string            `json:"signature"`
	Scope      string            `json:"scope"`
	Visibility string            `json:"visibility"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// OrphanedReference represents a reference to a deleted entity
type OrphanedReference struct {
	EntityName    string        `json:"entity_name"`
	EntityType    EntityType    `json:"entity_type"`
	File          string        `json:"file"`
	LineNumber    int           `json:"line_number"`
	ReferenceType ReferenceType `json:"reference_type"`
	Context       string        `json:"context"`
	DeletedFrom   string        `json:"deleted_from"`
}

// EntityAnalyzer analyzes code entities and their references
type EntityAnalyzer interface {
	// ExtractDeletedEntities extracts entities that were deleted in a diff
	ExtractDeletedEntities(diff *ParsedDiff) ([]CodeEntity, error)
	
	// FindOrphanedReferences finds references to deleted entities in the codebase
	FindOrphanedReferences(workspacePath string, deletedEntities []CodeEntity) ([]OrphanedReference, error)
	
	// ScanFileForReferences scans a single file for references to specific entities
	ScanFileForReferences(filePath string, entities []CodeEntity) ([]OrphanedReference, error)
}

// LanguageParser parses entities and references for a specific language
type LanguageParser interface {
	// GetLanguage returns the language this parser handles
	GetLanguage() string
	
	// ParseEntities extracts entities from file content
	ParseEntities(content string, filename string) ([]CodeEntity, error)
	
	// FindReferences finds references to entities in file content
	FindReferences(content string, filename string, entities []CodeEntity) ([]OrphanedReference, error)
}

// DefaultEntityAnalyzer implements EntityAnalyzer
type DefaultEntityAnalyzer struct {
	parsers map[string]LanguageParser
}

// NewDefaultEntityAnalyzer creates a new entity analyzer with language parsers
func NewDefaultEntityAnalyzer() *DefaultEntityAnalyzer {
	analyzer := &DefaultEntityAnalyzer{
		parsers: make(map[string]LanguageParser),
	}
	
	// Register language parsers
	analyzer.parsers["go"] = NewGoParser()
	analyzer.parsers["javascript"] = NewJavaScriptParser()
	analyzer.parsers["typescript"] = NewTypeScriptParser()
	
	return analyzer
}

// ExtractDeletedEntities extracts entities that were deleted in a diff
func (ea *DefaultEntityAnalyzer) ExtractDeletedEntities(diff *ParsedDiff) ([]CodeEntity, error) {
	var deletedEntities []CodeEntity
	
	for _, file := range diff.Files {
		parser, exists := ea.parsers[file.Language]
		if !exists {
			continue // Skip unsupported languages
		}
		
		// Extract entities from removed lines
		entities, err := ea.extractEntitiesFromRemovedLines(file, parser)
		if err != nil {
			return nil, fmt.Errorf("failed to extract entities from file %s: %w", file.Filename, err)
		}
		
		deletedEntities = append(deletedEntities, entities...)
	}
	
	return deletedEntities, nil
}

// FindOrphanedReferences finds references to deleted entities in the codebase
func (ea *DefaultEntityAnalyzer) FindOrphanedReferences(workspacePath string, deletedEntities []CodeEntity) ([]OrphanedReference, error) {
	var allReferences []OrphanedReference
	
	// Group entities by language for efficiency
	entitiesByLang := make(map[string][]CodeEntity)
	for _, entity := range deletedEntities {
		entitiesByLang[entity.Language] = append(entitiesByLang[entity.Language], entity)
	}
	
	// Walk through the workspace and scan files
	err := filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			return nil
		}
		
		// Determine file language
		language := detectLanguage(info.Name())
		entities, exists := entitiesByLang[language]
		if !exists || len(entities) == 0 {
			return nil // No entities to check for this language
		}
		
		// Scan the file for references
		references, scanErr := ea.ScanFileForReferences(path, entities)
		if scanErr != nil {
			// Log error but continue processing other files
			fmt.Printf("Warning: failed to scan file %s: %v\n", path, scanErr)
			return nil
		}
		
		allReferences = append(allReferences, references...)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to walk workspace: %w", err)
	}
	
	return allReferences, nil
}

// ScanFileForReferences scans a single file for references to specific entities
func (ea *DefaultEntityAnalyzer) ScanFileForReferences(filePath string, entities []CodeEntity) ([]OrphanedReference, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	
	// Determine the language of the file
	language := detectLanguage(filepath.Base(filePath))
	parser, exists := ea.parsers[language]
	if !exists {
		return nil, nil // Skip unsupported languages
	}
	
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	
	// Find references using the appropriate parser
	references, err := parser.FindReferences(string(content), filePath, entities)
	if err != nil {
		return nil, fmt.Errorf("failed to find references in file %s: %w", filePath, err)
	}
	
	return references, nil
}

// extractEntitiesFromRemovedLines extracts entities from removed lines in a diff
func (ea *DefaultEntityAnalyzer) extractEntitiesFromRemovedLines(file FileDiff, parser LanguageParser) ([]CodeEntity, error) {
	var removedContent strings.Builder
	var lineMapping []int // Maps line numbers in removedContent to original line numbers
	
	// Collect all removed lines
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if line.Type == "removed" {
				removedContent.WriteString(line.Content + "\n")
				lineMapping = append(lineMapping, line.OldLineNo)
			}
		}
	}
	
	if removedContent.Len() == 0 {
		return nil, nil // No removed content
	}
	
	// Parse entities from the removed content
	entities, err := parser.ParseEntities(removedContent.String(), file.Filename)
	if err != nil {
		return nil, err
	}
	
	// Adjust line numbers to match original file
	for i := range entities {
		if entities[i].LineNumber > 0 && entities[i].LineNumber <= len(lineMapping) {
			entities[i].LineNumber = lineMapping[entities[i].LineNumber-1]
		}
		entities[i].File = file.Filename
		entities[i].Language = file.Language
	}
	
	return entities, nil
}

// GoParser implements LanguageParser for Go
type GoParser struct{}

// NewGoParser creates a new Go parser
func NewGoParser() *GoParser {
	return &GoParser{}
}

// GetLanguage returns "go"
func (p *GoParser) GetLanguage() string {
	return "go"
}

// ParseEntities extracts Go entities from content
func (p *GoParser) ParseEntities(content string, filename string) ([]CodeEntity, error) {
	var entities []CodeEntity
	
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		
		// Parse functions
		if entity := p.parseFunctionDeclaration(line, lineNum+1); entity != nil {
			entities = append(entities, *entity)
		}
		
		// Parse types (struct, interface, type alias)
		if entity := p.parseTypeDeclaration(line, lineNum+1); entity != nil {
			entities = append(entities, *entity)
		}
		
		// Parse constants and variables
		if entity := p.parseConstantOrVariable(line, lineNum+1); entity != nil {
			entities = append(entities, *entity)
		}
	}
	
	return entities, nil
}

// FindReferences finds references to Go entities
func (p *GoParser) FindReferences(content string, filename string, entities []CodeEntity) ([]OrphanedReference, error) {
	var references []OrphanedReference
	
	// Create a map for quick entity lookup
	entityMap := make(map[string]CodeEntity)
	for _, entity := range entities {
		entityMap[entity.Name] = entity
	}
	
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		
		// Find function calls, type usage, etc.
		refs := p.findReferencesInLine(line, lineNum+1, filename, entityMap)
		references = append(references, refs...)
	}
	
	return references, nil
}

// Helper methods for GoParser will be implemented next...

// parseFunctionDeclaration parses Go function declarations
func (p *GoParser) parseFunctionDeclaration(line string, lineNum int) *CodeEntity {
	// Skip comments
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return nil
	}
	
	// Match function declarations: func name(...) or func (receiver) name(...)
	// Updated regex to handle generics: func name[T any](...) or func (receiver) name[T any](...)
	funcRegex := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?(\w+)(?:\[[^\]]*\])?\s*\(`)
	matches := funcRegex.FindStringSubmatch(line)
	
	if len(matches) >= 2 {
		funcName := matches[1]
		return &CodeEntity{
			Type:       EntityFunction,
			Name:       funcName,
			LineNumber: lineNum,
			Signature:  strings.TrimSpace(line),
		}
	}
	
	return nil
}

// parseTypeDeclaration parses Go type declarations
func (p *GoParser) parseTypeDeclaration(line string, lineNum int) *CodeEntity {
	// Skip comments
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return nil
	}
	
	// Match type declarations: type Name struct, type Name interface, type Name = OtherType
	typeRegex := regexp.MustCompile(`type\s+(\w+)(?:\[.*\])?\s+(struct|interface|=)`)
	matches := typeRegex.FindStringSubmatch(line)
	
	if len(matches) >= 3 {
		typeName := matches[1]
		typeKind := matches[2]
		
		var entityType EntityType
		switch typeKind {
		case "struct":
			entityType = EntityStruct
		case "interface":
			entityType = EntityInterface
		case "=":
			entityType = EntityTypeAlias
		default:
			entityType = EntityTypeAlias
		}
		
		return &CodeEntity{
			Type:       entityType,
			Name:       typeName,
			LineNumber: lineNum,
			Signature:  strings.TrimSpace(line),
		}
	}
	
	return nil
}

// parseConstantOrVariable parses Go constant and variable declarations
func (p *GoParser) parseConstantOrVariable(line string, lineNum int) *CodeEntity {
	// Skip comments
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return nil
	}
	
	// Match const and var declarations
	constRegex := regexp.MustCompile(`(?:const|var)\s+(\w+)`)
	matches := constRegex.FindStringSubmatch(line)
	
	if len(matches) >= 2 {
		name := matches[1]
		entityType := EntityConstant
		if strings.HasPrefix(strings.TrimSpace(line), "var") {
			entityType = EntityVariable
		}
		
		return &CodeEntity{
			Type:       entityType,
			Name:       name,
			LineNumber: lineNum,
			Signature:  strings.TrimSpace(line),
		}
	}
	
	return nil
}

// findReferencesInLine finds references to entities in a single line
func (p *GoParser) findReferencesInLine(line string, lineNum int, filename string, entityMap map[string]CodeEntity) []OrphanedReference {
	var references []OrphanedReference
	
	// Skip comments
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return references
	}
	
	for entityName, entity := range entityMap {
		// Look for function calls: entityName(
		if matched, _ := regexp.MatchString(`\b`+regexp.QuoteMeta(entityName)+`\s*\(`, line); matched {
			references = append(references, OrphanedReference{
				EntityName:    entityName,
				EntityType:    entity.Type,
				File:          filename,
				LineNumber:    lineNum,
				ReferenceType: ReferenceCall,
				Context:       strings.TrimSpace(line),
				DeletedFrom:   entity.File,
			})
		}
		
		// Look for type usage: var x EntityName
		if matched, _ := regexp.MatchString(`\b`+regexp.QuoteMeta(entityName)+`\b`, line); matched {
			// Avoid duplicate detection for function calls
			if isCall, _ := regexp.MatchString(`\b`+regexp.QuoteMeta(entityName)+`\s*\(`, line); !isCall {
				references = append(references, OrphanedReference{
					EntityName:    entityName,
					EntityType:    entity.Type,
					File:          filename,
					LineNumber:    lineNum,
					ReferenceType: ReferenceTypeUsage,
					Context:       strings.TrimSpace(line),
					DeletedFrom:   entity.File,
				})
			}
		}
	}
	
	return references
}

// Placeholder implementations for JavaScript and TypeScript parsers
type JavaScriptParser struct{}

func NewJavaScriptParser() *JavaScriptParser {
	return &JavaScriptParser{}
}

func (p *JavaScriptParser) GetLanguage() string {
	return "javascript"
}

func (p *JavaScriptParser) ParseEntities(content string, filename string) ([]CodeEntity, error) {
	// TODO: Implement JavaScript entity parsing
	return nil, nil
}

func (p *JavaScriptParser) FindReferences(content string, filename string, entities []CodeEntity) ([]OrphanedReference, error) {
	// TODO: Implement JavaScript reference finding
	return nil, nil
}

type TypeScriptParser struct{}

func NewTypeScriptParser() *TypeScriptParser {
	return &TypeScriptParser{}
}

func (p *TypeScriptParser) GetLanguage() string {
	return "typescript"
}

func (p *TypeScriptParser) ParseEntities(content string, filename string) ([]CodeEntity, error) {
	// TODO: Implement TypeScript entity parsing
	return nil, nil
}

func (p *TypeScriptParser) FindReferences(content string, filename string, entities []CodeEntity) ([]OrphanedReference, error) {
	// TODO: Implement TypeScript reference finding
	return nil, nil
}