package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Test data structures for expected results
type ExpectedResults struct {
	DeletedEntities     []CodeEntity        `json:"deleted_entities"`
	OrphanedReferences  []OrphanedReference `json:"orphaned_references"`
}

func TestGoParser_ParseEntities(t *testing.T) {
	parser := NewGoParser()
	
	// Read test file content
	testDataPath := filepath.Join("testdata", "go", "sample_deletions.go")
	content, err := os.ReadFile(testDataPath)
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}
	
	entities, err := parser.ParseEntities(string(content), "sample_deletions.go")
	if err != nil {
		t.Fatalf("ParseEntities failed: %v", err)
	}
	
	// Load expected results
	expectedPath := filepath.Join("testdata", "go", "expected_results.json")
	expectedData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read expected results: %v", err)
	}
	
	var expected ExpectedResults
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("Failed to unmarshal expected results: %v", err)
	}
	
	// Test that we found the expected number of entities
	if len(entities) == 0 {
		t.Error("Expected to find some entities, got none")
	}
	
	// Test specific entities
	entityMap := make(map[string]CodeEntity)
	for _, entity := range entities {
		entityMap[entity.Name] = entity
	}
	
	// Test for DeletedFunction
	if entity, exists := entityMap["DeletedFunction"]; exists {
		if entity.Type != EntityFunction {
			t.Errorf("Expected DeletedFunction to be EntityFunction, got %v", entity.Type)
		}
		if entity.Name != "DeletedFunction" {
			t.Errorf("Expected entity name 'DeletedFunction', got '%s'", entity.Name)
		}
	} else {
		t.Error("Expected to find DeletedFunction entity")
	}
	
	// Test for DeletedStruct
	if entity, exists := entityMap["DeletedStruct"]; exists {
		if entity.Type != EntityStruct {
			t.Errorf("Expected DeletedStruct to be EntityStruct, got %v", entity.Type)
		}
	} else {
		t.Error("Expected to find DeletedStruct entity")
	}
	
	// Test for DeletedInterface
	if entity, exists := entityMap["DeletedInterface"]; exists {
		if entity.Type != EntityInterface {
			t.Errorf("Expected DeletedInterface to be EntityInterface, got %v", entity.Type)
		}
	} else {
		t.Error("Expected to find DeletedInterface entity")
	}
	
	// Test for constants
	if entity, exists := entityMap["DeletedConstant"]; exists {
		if entity.Type != EntityConstant {
			t.Errorf("Expected DeletedConstant to be EntityConstant, got %v", entity.Type)
		}
	} else {
		t.Error("Expected to find DeletedConstant entity")
	}
	
	t.Logf("Found %d entities", len(entities))
	for _, entity := range entities {
		t.Logf("Entity: %s (%s) at line %d", entity.Name, entity.Type, entity.LineNumber)
	}
}

func TestGoParser_FindReferences(t *testing.T) {
	parser := NewGoParser()
	
	// First, parse entities from the deletions file
	deletionsPath := filepath.Join("testdata", "go", "sample_deletions.go")
	deletionsContent, err := os.ReadFile(deletionsPath)
	if err != nil {
		t.Fatalf("Failed to read deletions file: %v", err)
	}
	
	entities, err := parser.ParseEntities(string(deletionsContent), "sample_deletions.go")
	if err != nil {
		t.Fatalf("Failed to parse entities: %v", err)
	}
	
	if len(entities) == 0 {
		t.Fatal("No entities parsed from deletions file")
	}
	
	// Now find references in the references file
	referencesPath := filepath.Join("testdata", "go", "sample_references.go")
	referencesContent, err := os.ReadFile(referencesPath)
	if err != nil {
		t.Fatalf("Failed to read references file: %v", err)
	}
	
	references, err := parser.FindReferences(string(referencesContent), "sample_references.go", entities)
	if err != nil {
		t.Fatalf("FindReferences failed: %v", err)
	}
	
	// Test that we found some references
	if len(references) == 0 {
		t.Error("Expected to find some references, got none")
	}
	
	// Test for specific references
	referenceMap := make(map[string][]OrphanedReference)
	for _, ref := range references {
		referenceMap[ref.EntityName] = append(referenceMap[ref.EntityName], ref)
	}
	
	// Test DeletedFunction references
	if refs, exists := referenceMap["DeletedFunction"]; exists {
		found := false
		for _, ref := range refs {
			if ref.ReferenceType == ReferenceCall {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find function call reference to DeletedFunction")
		}
	} else {
		t.Error("Expected to find references to DeletedFunction")
	}
	
	// Test DeletedStruct references
	if refs, exists := referenceMap["DeletedStruct"]; exists {
		found := false
		for _, ref := range refs {
			if ref.ReferenceType == ReferenceTypeUsage {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find type usage reference to DeletedStruct")
		}
	} else {
		t.Error("Expected to find references to DeletedStruct")
	}
	
	t.Logf("Found %d references", len(references))
	for _, ref := range references {
		t.Logf("Reference: %s (%s) in %s at line %d", ref.EntityName, ref.ReferenceType, ref.File, ref.LineNumber)
	}
}

func TestDefaultEntityAnalyzer_ExtractDeletedEntities(t *testing.T) {
	analyzer := NewDefaultEntityAnalyzer()
	
	// Create a mock diff with deleted Go code
	diff := &ParsedDiff{
		Files: []FileDiff{
			{
				Filename: "sample.go",
				Language: "go",
				Status:   "modified",
				Hunks: []DiffHunk{
					{
						Lines: []DiffLine{
							{Type: "context", Content: "package main"},
							{Type: "removed", Content: "func DeletedFunction() {", OldLineNo: 5},
							{Type: "removed", Content: "    fmt.Println(\"deleted\")", OldLineNo: 6},
							{Type: "removed", Content: "}", OldLineNo: 7},
							{Type: "context", Content: ""},
							{Type: "removed", Content: "type DeletedStruct struct {", OldLineNo: 9},
							{Type: "removed", Content: "    Name string", OldLineNo: 10},
							{Type: "removed", Content: "}", OldLineNo: 11},
						},
					},
				},
			},
		},
	}
	
	entities, err := analyzer.ExtractDeletedEntities(diff)
	if err != nil {
		t.Fatalf("ExtractDeletedEntities failed: %v", err)
	}
	
	// Test that we found entities
	if len(entities) == 0 {
		t.Error("Expected to find deleted entities, got none")
	}
	
	// Create entity map for easier testing
	entityMap := make(map[string]CodeEntity)
	for _, entity := range entities {
		entityMap[entity.Name] = entity
	}
	
	// Test for specific entities
	if entity, exists := entityMap["DeletedFunction"]; exists {
		if entity.Type != EntityFunction {
			t.Errorf("Expected DeletedFunction to be EntityFunction, got %v", entity.Type)
		}
		if entity.File != "sample.go" {
			t.Errorf("Expected file 'sample.go', got '%s'", entity.File)
		}
		if entity.Language != "go" {
			t.Errorf("Expected language 'go', got '%s'", entity.Language)
		}
	} else {
		t.Error("Expected to find DeletedFunction in deleted entities")
	}
	
	if entity, exists := entityMap["DeletedStruct"]; exists {
		if entity.Type != EntityStruct {
			t.Errorf("Expected DeletedStruct to be EntityStruct, got %v", entity.Type)
		}
	} else {
		t.Error("Expected to find DeletedStruct in deleted entities")
	}
	
	t.Logf("Found %d deleted entities", len(entities))
}

func TestDefaultEntityAnalyzer_ScanFileForReferences(t *testing.T) {
	analyzer := NewDefaultEntityAnalyzer()
	
	// Create some test entities
	entities := []CodeEntity{
		{
			Type:     EntityFunction,
			Name:     "DeletedFunction",
			Language: "go",
			File:     "deleted.go",
		},
		{
			Type:     EntityStruct,
			Name:     "DeletedStruct", 
			Language: "go",
			File:     "deleted.go",
		},
	}
	
	// Test scanning the references file
	referencesPath := filepath.Join("testdata", "go", "sample_references.go")
	references, err := analyzer.ScanFileForReferences(referencesPath, entities)
	if err != nil {
		t.Fatalf("ScanFileForReferences failed: %v", err)
	}
	
	// Test that we found references
	if len(references) == 0 {
		t.Error("Expected to find references, got none")
	}
	
	// Test that references have correct structure
	for _, ref := range references {
		if ref.EntityName == "" {
			t.Error("Reference has empty EntityName")
		}
		if ref.File == "" {
			t.Error("Reference has empty File")
		}
		if ref.LineNumber <= 0 {
			t.Error("Reference has invalid LineNumber")
		}
		if ref.Context == "" {
			t.Error("Reference has empty Context")
		}
	}
	
	t.Logf("Found %d references in file", len(references))
}

func TestGoParser_parseFunctionDeclaration(t *testing.T) {
	parser := NewGoParser()
	
	testCases := []struct {
		line     string
		expected string
		shouldFind bool
	}{
		{"func TestFunction() {", "TestFunction", true},
		{"func (r *Receiver) Method() {", "Method", true},
		{"func TestWithParams(a int, b string) error {", "TestWithParams", true},
		{"func TestGeneric[T any](param T) T {", "TestGeneric", true},
		{"func TestMultiReturn() (int, error) {", "TestMultiReturn", true},
		{"// func CommentedFunction() {", "", false},
		{"var notAFunction = func() {", "", false},
		{"type NotFunction struct {", "", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			entity := parser.parseFunctionDeclaration(tc.line, 1)
			
			if tc.shouldFind {
				if entity == nil {
					t.Errorf("Expected to find function entity, got nil")
					return
				}
				if entity.Name != tc.expected {
					t.Errorf("Expected function name '%s', got '%s'", tc.expected, entity.Name)
				}
				if entity.Type != EntityFunction {
					t.Errorf("Expected EntityFunction, got %v", entity.Type)
				}
			} else {
				if entity != nil {
					t.Errorf("Expected no function entity, got %v", entity)
				}
			}
		})
	}
}

func TestGoParser_parseTypeDeclaration(t *testing.T) {
	parser := NewGoParser()
	
	testCases := []struct {
		line         string
		expectedName string
		expectedType EntityType
		shouldFind   bool
	}{
		{"type MyStruct struct {", "MyStruct", EntityStruct, true},
		{"type MyInterface interface {", "MyInterface", EntityInterface, true},
		{"type MyType = string", "MyType", EntityTypeAlias, true},
		{"type MyGeneric[T any] struct {", "MyGeneric", EntityStruct, true},
		{"// type CommentedType struct {", "", EntityTypeAlias, false},
		{"func NotAType() {", "", EntityTypeAlias, false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			entity := parser.parseTypeDeclaration(tc.line, 1)
			
			if tc.shouldFind {
				if entity == nil {
					t.Errorf("Expected to find type entity, got nil")
					return
				}
				if entity.Name != tc.expectedName {
					t.Errorf("Expected type name '%s', got '%s'", tc.expectedName, entity.Name)
				}
				if entity.Type != tc.expectedType {
					t.Errorf("Expected %v, got %v", tc.expectedType, entity.Type)
				}
			} else {
				if entity != nil {
					t.Errorf("Expected no type entity, got %v", entity)
				}
			}
		})
	}
}

func TestGoParser_parseConstantOrVariable(t *testing.T) {
	parser := NewGoParser()
	
	testCases := []struct {
		line         string
		expectedName string
		expectedType EntityType
		shouldFind   bool
	}{
		{"const MyConstant = \"value\"", "MyConstant", EntityConstant, true},
		{"var MyVariable = \"value\"", "MyVariable", EntityVariable, true},
		{"const MyConst int = 42", "MyConst", EntityConstant, true},
		{"var MyVar int = 42", "MyVar", EntityVariable, true},
		{"// const CommentedConst = 1", "", EntityConstant, false},
		{"func NotAConstant() {", "", EntityConstant, false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			entity := parser.parseConstantOrVariable(tc.line, 1)
			
			if tc.shouldFind {
				if entity == nil {
					t.Errorf("Expected to find entity, got nil")
					return
				}
				if entity.Name != tc.expectedName {
					t.Errorf("Expected name '%s', got '%s'", tc.expectedName, entity.Name)
				}
				if entity.Type != tc.expectedType {
					t.Errorf("Expected %v, got %v", tc.expectedType, entity.Type)
				}
			} else {
				if entity != nil {
					t.Errorf("Expected no entity, got %v", entity)
				}
			}
		})
	}
}

func TestGoParser_findReferencesInLine(t *testing.T) {
	parser := NewGoParser()
	
	entities := map[string]CodeEntity{
		"TestFunction": {
			Type: EntityFunction,
			Name: "TestFunction",
			File: "test.go",
		},
		"TestStruct": {
			Type: EntityStruct,
			Name: "TestStruct",
			File: "test.go",
		},
	}
	
	testCases := []struct {
		line              string
		expectedRefs      int
		expectedRefTypes  []ReferenceType
		expectedEntities  []string
	}{
		{"TestFunction()", 1, []ReferenceType{ReferenceCall}, []string{"TestFunction"}},
		{"var x TestStruct", 1, []ReferenceType{ReferenceTypeUsage}, []string{"TestStruct"}},
		{"TestFunction(); TestStruct{}", 2, []ReferenceType{ReferenceCall, ReferenceTypeUsage}, []string{"TestFunction", "TestStruct"}},
		{"// TestFunction() in comment", 0, []ReferenceType{}, []string{}},
		{"someOtherFunction()", 0, []ReferenceType{}, []string{}},
	}
	
	for _, tc := range testCases {
		t.Run(tc.line, func(t *testing.T) {
			refs := parser.findReferencesInLine(tc.line, 1, "test.go", entities)
			
			if len(refs) != tc.expectedRefs {
				t.Errorf("Expected %d references, got %d", tc.expectedRefs, len(refs))
			}
			
			// Check reference types and entities
			for i, ref := range refs {
				if i < len(tc.expectedRefTypes) {
					if ref.ReferenceType != tc.expectedRefTypes[i] {
						t.Errorf("Expected reference type %v, got %v", tc.expectedRefTypes[i], ref.ReferenceType)
					}
				}
				if i < len(tc.expectedEntities) {
					if ref.EntityName != tc.expectedEntities[i] {
						t.Errorf("Expected entity name %s, got %s", tc.expectedEntities[i], ref.EntityName)
					}
				}
			}
		})
	}
}