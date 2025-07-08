package analyze

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/logze/v2"
)

// DependencyMapper maps semantic relationships between code entities
type DependencyMapper struct {
	provider interfaces.CodeProvider
	log      logze.Logger
}

// NewDependencyMapper creates a new dependency mapper
func NewDependencyMapper(provider interfaces.CodeProvider) *DependencyMapper {
	return &DependencyMapper{
		provider: provider,
		log:      logze.With("component", "dependency-mapper"),
	}
}

// DependencyGraph represents the relationship graph of code entities
type DependencyGraph struct {
	Entities     map[string]*CodeEntity    `json:"entities"`      // entity_id -> entity
	Dependencies map[string][]Relationship `json:"dependencies"`  // entity_id -> list of things it depends on
	Dependents   map[string][]Relationship `json:"dependents"`    // entity_id -> list of things that depend on it
	CallGraph    map[string][]FunctionCall `json:"call_graph"`    // function_id -> list of functions it calls
	TypeUsage    map[string][]TypeUsage    `json:"type_usage"`    // type_id -> list of places where it's used
	ImportGraph  map[string][]ImportUsage  `json:"import_graph"`  // package_path -> list of import usages
	PackageScope map[string][]string       `json:"package_scope"` // package_path -> list of entities in package
}

// CodeEntity represents a code entity with its semantic information
type CodeEntity struct {
	ID            string     `json:"id"`             // unique identifier
	Name          string     `json:"name"`           // entity name
	Type          EntityType `json:"type"`           // entity type
	Package       string     `json:"package"`        // package name
	FilePath      string     `json:"file_path"`      // file path
	StartLine     int        `json:"start_line"`     // start line in file
	EndLine       int        `json:"end_line"`       // end line in file
	IsExported    bool       `json:"is_exported"`    // whether it's exported
	Signature     string     `json:"signature"`      // function/method signature
	DocComment    string     `json:"doc_comment"`    // documentation comment
	CodeSnippet   string     `json:"code_snippet"`   // actual code
	Complexity    int        `json:"complexity"`     // cyclomatic complexity
	BusinessArea  string     `json:"business_area"`  // inferred business area
	SecurityLevel string     `json:"security_level"` // security sensitivity level
}

// Relationship represents a semantic relationship between entities
type Relationship struct {
	Target      string           `json:"target"`       // target entity ID
	Type        RelationshipType `json:"type"`         // type of relationship
	Context     string           `json:"context"`      // usage context
	FilePath    string           `json:"file_path"`    // where the relationship occurs
	LineNumber  int              `json:"line_number"`  // line number of the relationship
	CodeSnippet string           `json:"code_snippet"` // relevant code snippet
	Strength    float64          `json:"strength"`     // relationship strength (0.0-1.0)
	IsExternal  bool             `json:"is_external"`  // whether target is external to project
}

// RelationshipType represents different types of semantic relationships
type RelationshipType string

const (
	RelationshipFunctionCall   RelationshipType = "function_call"  // calls a function
	RelationshipMethodCall     RelationshipType = "method_call"    // calls a method
	RelationshipTypeUsage      RelationshipType = "type_usage"     // uses a type
	RelationshipFieldAccess    RelationshipType = "field_access"   // accesses a field
	RelationshipInterfaceImpl  RelationshipType = "interface_impl" // implements an interface
	RelationshipComposition    RelationshipType = "composition"    // embeds/composes another type
	RelationshipInheritance    RelationshipType = "inheritance"    // inherits from another type
	RelationshipParameterType  RelationshipType = "parameter_type" // uses type as parameter
	RelationshipReturnType     RelationshipType = "return_type"    // uses type as return value
	RelationshipVariableType   RelationshipType = "variable_type"  // uses type for variable
	RelationshipImport         RelationshipType = "import"         // imports a package
	RelationshipAssignment     RelationshipType = "assignment"     // assigns to a variable
	RelationshipInitialization RelationshipType = "initialization" // initializes a value
)

// FunctionCall represents a function call relationship
type FunctionCall struct {
	Caller        string   `json:"caller"`         // calling function ID
	Callee        string   `json:"callee"`         // called function name
	CalleeID      string   `json:"callee_id"`      // called function ID (if known)
	LineNumber    int      `json:"line_number"`    // line number of call
	Arguments     []string `json:"arguments"`      // argument types/values
	IsMethod      bool     `json:"is_method"`      // whether it's a method call
	Receiver      string   `json:"receiver"`       // receiver type for method calls
	CodeSnippet   string   `json:"code_snippet"`   // code snippet of the call
	Frequency     int      `json:"frequency"`      // how often this call occurs
	IsConditional bool     `json:"is_conditional"` // whether call is conditional
}

// TypeUsage represents how a type is used
type TypeUsage struct {
	TypeName     string       `json:"type_name"`     // name of the type
	TypeID       string       `json:"type_id"`       // type entity ID
	UsageContext UsageContext `json:"usage_context"` // how it's used
	FilePath     string       `json:"file_path"`     // where it's used
	LineNumber   int          `json:"line_number"`   // line number
	CodeSnippet  string       `json:"code_snippet"`  // relevant code
	IsPointer    bool         `json:"is_pointer"`    // whether used as pointer
	IsSlice      bool         `json:"is_slice"`      // whether used as slice
	IsMap        bool         `json:"is_map"`        // whether used as map
}

// UsageContext represents different ways a type can be used
type UsageContext string

const (
	UsageParameter       UsageContext = "parameter"        // function parameter
	UsageReturn          UsageContext = "return"           // return value
	UsageVariable        UsageContext = "variable"         // variable declaration
	UsageStructField     UsageContext = "struct_field"     // struct field
	UsageInterfaceMethod UsageContext = "interface_method" // interface method
	UsageTypeAssertion   UsageContext = "type_assertion"   // type assertion
	UsageTypeSwitch      UsageContext = "type_switch"      // type switch
	UsageComposition     UsageContext = "composition"      // embedded in struct
	UsageInstantiation   UsageContext = "instantiation"    // creating instance
)

// ImportUsage represents how an import is used
type ImportUsage struct {
	ImportPath    string   `json:"import_path"`     // import path
	Alias         string   `json:"alias"`           // import alias
	FilePath      string   `json:"file_path"`       // where imported
	UsageCount    int      `json:"usage_count"`     // how often used
	UsedEntities  []string `json:"used_entities"`   // specific entities used from package
	FirstUseLine  int      `json:"first_use_line"`  // first line where package is used
	IsStandardLib bool     `json:"is_standard_lib"` // whether it's stdlib
	IsThirdParty  bool     `json:"is_third_party"`  // whether it's third party
}

// MapDependencies creates a comprehensive dependency graph for changed entities
func (dm *DependencyMapper) MapDependencies(ctx context.Context, request model.ReviewRequest, changedEntities []ChangedEntity, filePath string) (*DependencyGraph, error) {
	log := dm.log.WithFields("file", filePath, "entities", len(changedEntities))
	log.Debug("starting dependency mapping")

	graph := &DependencyGraph{
		Entities:     make(map[string]*CodeEntity),
		Dependencies: make(map[string][]Relationship),
		Dependents:   make(map[string][]Relationship),
		CallGraph:    make(map[string][]FunctionCall),
		TypeUsage:    make(map[string][]TypeUsage),
		ImportGraph:  make(map[string][]ImportUsage),
		PackageScope: make(map[string][]string),
	}

	// Convert changed entities to code entities
	for _, entity := range changedEntities {
		codeEntity := dm.convertToCodeEntity(entity, filePath)
		graph.Entities[codeEntity.ID] = codeEntity
	}

	// Map direct dependencies for each changed entity
	for _, entity := range changedEntities {
		entityID := dm.generateEntityID(entity.Name, entity.Type, filePath)

		// Find function calls
		if entity.Type == EntityTypeFunction || entity.Type == EntityTypeMethod {
			calls, err := dm.findFunctionCalls(ctx, request, entity, filePath)
			if err != nil {
				log.Warn("failed to find function calls", "entity", entity.Name, "error", err)
			} else {
				graph.CallGraph[entityID] = calls
			}
		}

		// Find type usages
		typeUsages, err := dm.findTypeUsages(ctx, request, entity, filePath)
		if err != nil {
			log.Warn("failed to find type usages", "entity", entity.Name, "error", err)
		} else {
			for _, usage := range typeUsages {
				if usage.TypeID != "" {
					graph.TypeUsage[usage.TypeID] = append(graph.TypeUsage[usage.TypeID], usage)
				}
			}
		}

		// Find direct dependencies
		dependencies, err := dm.findDirectDependencies(ctx, request, entity, filePath)
		if err != nil {
			log.Warn("failed to find dependencies", "entity", entity.Name, "error", err)
		} else {
			graph.Dependencies[entityID] = dependencies
		}

		// Find dependents (things that depend on this entity)
		dependents, err := dm.findDependents(ctx, request, entity, filePath)
		if err != nil {
			log.Warn("failed to find dependents", "entity", entity.Name, "error", err)
		} else {
			graph.Dependents[entityID] = dependents
		}
	}

	// Analyze import relationships
	importUsages, err := dm.analyzeImports(ctx, request, filePath)
	if err != nil {
		log.Warn("failed to analyze imports", "error", err)
	} else {
		for importPath, usages := range importUsages {
			graph.ImportGraph[importPath] = usages
		}
	}

	// Build package scope map
	err = dm.buildPackageScope(ctx, request, graph, filePath)
	if err != nil {
		log.Warn("failed to build package scope", "error", err)
	}

	log.Debug("dependency mapping completed",
		"entities", len(graph.Entities),
		"dependencies", len(graph.Dependencies),
		"dependents", len(graph.Dependents))

	return graph, nil
}

// convertToCodeEntity converts a ChangedEntity to a CodeEntity
func (dm *DependencyMapper) convertToCodeEntity(entity ChangedEntity, filePath string) *CodeEntity {
	return &CodeEntity{
		ID:            dm.generateEntityID(entity.Name, entity.Type, filePath),
		Name:          entity.Name,
		Type:          entity.Type,
		Package:       dm.extractPackageFromPath(filePath),
		FilePath:      filePath,
		StartLine:     entity.StartLine,
		EndLine:       entity.EndLine,
		IsExported:    entity.IsExported,
		Signature:     entity.Signature,
		CodeSnippet:   entity.AfterCode,
		BusinessArea:  dm.inferBusinessArea(filePath, entity.Name),
		SecurityLevel: dm.inferSecurityLevel(filePath, entity.Name, entity.AfterCode),
	}
}

// generateEntityID generates a unique ID for an entity
func (dm *DependencyMapper) generateEntityID(name string, entityType EntityType, filePath string) string {
	pkg := dm.extractPackageFromPath(filePath)
	return fmt.Sprintf("%s.%s.%s", pkg, string(entityType), name)
}

// extractPackageFromPath extracts package name from file path
func (dm *DependencyMapper) extractPackageFromPath(filePath string) string {
	dir := filepath.Dir(filePath)
	return filepath.Base(dir)
}

// findFunctionCalls finds all function calls made by an entity
func (dm *DependencyMapper) findFunctionCalls(ctx context.Context, request model.ReviewRequest, entity ChangedEntity, filePath string) ([]FunctionCall, error) {
	var calls []FunctionCall

	// Parse the entity's code to find function calls
	code := entity.AfterCode
	if code == "" {
		return calls, nil
	}

	// Look for function call patterns
	functionCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\s*\(`)
	methodCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\.\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

	lines := strings.Split(code, "\n")
	for lineNum, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		// Skip comment lines completely
		if dm.isCommentLine(line) {
			continue
		}

		// Remove inline comments from the line
		line = dm.removeInlineComments(line)
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Find method calls
		methodMatches := methodCallRegex.FindAllStringSubmatch(line, -1)
		for _, match := range methodMatches {
			if len(match) >= 3 && dm.isValidFunctionCall(match[2]) {
				calls = append(calls, FunctionCall{
					Caller:      entity.Name,
					Callee:      match[2],
					LineNumber:  entity.StartLine + lineNum,
					IsMethod:    true,
					Receiver:    match[1],
					CodeSnippet: strings.TrimSpace(originalLine),
					Frequency:   1,
				})
			}
		}

		// Find function calls
		funcMatches := functionCallRegex.FindAllStringSubmatch(line, -1)
		for _, match := range funcMatches {
			if len(match) >= 2 && dm.isValidFunctionCall(match[1]) {
				// Skip if it's already captured as a method call
				if !strings.Contains(match[1], ".") || dm.isPackageQualifiedCall(match[1]) {
					calls = append(calls, FunctionCall{
						Caller:      entity.Name,
						Callee:      match[1],
						LineNumber:  entity.StartLine + lineNum,
						IsMethod:    false,
						CodeSnippet: strings.TrimSpace(originalLine),
						Frequency:   1,
					})
				}
			}
		}
	}

	return calls, nil
}

// isCommentLine checks if a line is a comment (handles Go, JS, Python, etc.)
func (dm *DependencyMapper) isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") || // Go, JS, C++
		strings.HasPrefix(trimmed, "#") || // Python, shell
		strings.HasPrefix(trimmed, "/*") || // Multi-line comments
		strings.HasPrefix(trimmed, "*") || // Inside multi-line comments
		strings.HasPrefix(trimmed, "<!--") || // HTML comments
		strings.HasPrefix(trimmed, "--") // SQL comments
}

// removeInlineComments removes inline comments from a line
func (dm *DependencyMapper) removeInlineComments(line string) string {
	// Handle different comment styles
	commentMarkers := []string{"//", "#"}

	for _, marker := range commentMarkers {
		if idx := strings.Index(line, marker); idx != -1 {
			// Make sure it's not inside a string literal
			if !dm.isInsideStringLiteral(line, idx) {
				line = line[:idx]
			}
		}
	}

	return line
}

// isInsideStringLiteral checks if the given index is inside a string literal
func (dm *DependencyMapper) isInsideStringLiteral(line string, index int) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for i, char := range line {
		if i >= index {
			break
		}

		switch char {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
		case '\\':
			// Skip escaped characters
			if i+1 < len(line) {
				i++
			}
		}
	}

	return inSingleQuote || inDoubleQuote || inBacktick
}

// isValidFunctionCall validates that the matched text is actually a function call
func (dm *DependencyMapper) isValidFunctionCall(name string) bool {
	// Skip if it looks like a comment artifact
	if strings.Contains(strings.ToLower(name), "test") && len(name) < 4 {
		return false
	}

	// Skip single characters or very short names that are likely false positives
	if len(name) < 2 {
		return false
	}

	// Skip common false positives
	falsePositives := []string{
		"if", "for", "while", "switch", "case", "default", "return",
		"var", "const", "let", "import", "export", "class", "interface",
		"type", "struct", "func", "def", "async", "await",
	}

	nameLower := strings.ToLower(name)
	for _, fp := range falsePositives {
		if nameLower == fp {
			return false
		}
	}

	// Must start with a letter or underscore
	if len(name) > 0 {
		firstChar := name[0]
		return (firstChar >= 'a' && firstChar <= 'z') ||
			(firstChar >= 'A' && firstChar <= 'Z') ||
			firstChar == '_'
	}

	return false
}

// isPackageQualifiedCall checks if a call is package-qualified (e.g., fmt.Println)
func (dm *DependencyMapper) isPackageQualifiedCall(call string) bool {
	parts := strings.Split(call, ".")
	if len(parts) == 2 {
		// Check if first part looks like a package name (lowercase)
		pkg := parts[0]
		return len(pkg) > 0 && pkg[0] >= 'a' && pkg[0] <= 'z'
	}
	return false
}

// findTypeUsages finds how types are used by an entity
func (dm *DependencyMapper) findTypeUsages(ctx context.Context, request model.ReviewRequest, entity ChangedEntity, filePath string) ([]TypeUsage, error) {
	var usages []TypeUsage

	code := entity.AfterCode
	if code == "" {
		return usages, nil
	}

	// Look for type usage patterns
	variableDeclRegex := regexp.MustCompile(`(?:var\s+\w+\s+|:\s*=\s*(?:\*)?|\w+\s+)([A-Z][a-zA-Z0-9_]*(?:\[[^\]]*\])?(?:\*)?)\s*(?:[{(\n,]|$)`)
	parameterRegex := regexp.MustCompile(`\(\s*\w+\s+(?:\*)?([A-Z][a-zA-Z0-9_]*(?:\[[^\]]*\])?)`)
	returnRegex := regexp.MustCompile(`\)\s+(?:\*)?([A-Z][a-zA-Z0-9_]*(?:\[[^\]]*\])?)`)

	lines := strings.Split(code, "\n")
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Find type usages in variable declarations
		varMatches := variableDeclRegex.FindAllStringSubmatch(line, -1)
		for _, match := range varMatches {
			if len(match) >= 2 {
				typeName := dm.cleanTypeName(match[1])
				usages = append(usages, TypeUsage{
					TypeName:     typeName,
					TypeID:       dm.generateEntityID(typeName, EntityTypeType, ""),
					UsageContext: UsageVariable,
					FilePath:     filePath,
					LineNumber:   entity.StartLine + lineNum,
					CodeSnippet:  line,
					IsPointer:    strings.Contains(match[1], "*"),
					IsSlice:      strings.Contains(match[1], "[]"),
				})
			}
		}

		// Find type usages in parameters
		paramMatches := parameterRegex.FindAllStringSubmatch(line, -1)
		for _, match := range paramMatches {
			if len(match) >= 2 {
				typeName := dm.cleanTypeName(match[1])
				usages = append(usages, TypeUsage{
					TypeName:     typeName,
					TypeID:       dm.generateEntityID(typeName, EntityTypeType, ""),
					UsageContext: UsageParameter,
					FilePath:     filePath,
					LineNumber:   entity.StartLine + lineNum,
					CodeSnippet:  line,
					IsPointer:    strings.Contains(match[1], "*"),
					IsSlice:      strings.Contains(match[1], "[]"),
				})
			}
		}

		// Find type usages in return values
		returnMatches := returnRegex.FindAllStringSubmatch(line, -1)
		for _, match := range returnMatches {
			if len(match) >= 2 {
				typeName := dm.cleanTypeName(match[1])
				usages = append(usages, TypeUsage{
					TypeName:     typeName,
					TypeID:       dm.generateEntityID(typeName, EntityTypeType, ""),
					UsageContext: UsageReturn,
					FilePath:     filePath,
					LineNumber:   entity.StartLine + lineNum,
					CodeSnippet:  line,
					IsPointer:    strings.Contains(match[1], "*"),
					IsSlice:      strings.Contains(match[1], "[]"),
				})
			}
		}
	}

	return usages, nil
}

// cleanTypeName cleans up type name by removing decorators
func (dm *DependencyMapper) cleanTypeName(typeName string) string {
	// Remove pointers, slices, arrays
	typeName = strings.ReplaceAll(typeName, "*", "")
	typeName = strings.ReplaceAll(typeName, "[]", "")

	// Remove array bounds
	if idx := strings.Index(typeName, "["); idx != -1 {
		if endIdx := strings.Index(typeName[idx:], "]"); endIdx != -1 {
			typeName = typeName[:idx] + typeName[idx+endIdx+1:]
		}
	}

	return strings.TrimSpace(typeName)
}

// findDirectDependencies finds direct dependencies of an entity
func (dm *DependencyMapper) findDirectDependencies(ctx context.Context, request model.ReviewRequest, entity ChangedEntity, filePath string) ([]Relationship, error) {
	var dependencies []Relationship

	// Convert function calls to dependencies
	calls, err := dm.findFunctionCalls(ctx, request, entity, filePath)
	if err == nil {
		for _, call := range calls {
			relType := RelationshipFunctionCall
			if call.IsMethod {
				relType = RelationshipMethodCall
			}

			dependencies = append(dependencies, Relationship{
				Target:      call.Callee,
				Type:        relType,
				Context:     "function_call",
				FilePath:    filePath,
				LineNumber:  call.LineNumber,
				CodeSnippet: call.CodeSnippet,
				Strength:    0.8, // High strength for direct calls
				IsExternal:  dm.isExternalEntity(call.Callee),
			})
		}
	}

	// Convert type usages to dependencies
	typeUsages, err := dm.findTypeUsages(ctx, request, entity, filePath)
	if err == nil {
		for _, usage := range typeUsages {
			dependencies = append(dependencies, Relationship{
				Target:      usage.TypeName,
				Type:        RelationshipTypeUsage,
				Context:     string(usage.UsageContext),
				FilePath:    filePath,
				LineNumber:  usage.LineNumber,
				CodeSnippet: usage.CodeSnippet,
				Strength:    0.6, // Medium strength for type usage
				IsExternal:  dm.isExternalEntity(usage.TypeName),
			})
		}
	}

	return dependencies, nil
}

// findDependents finds entities that depend on the given entity
func (dm *DependencyMapper) findDependents(ctx context.Context, request model.ReviewRequest, entity ChangedEntity, filePath string) ([]Relationship, error) {
	var dependents []Relationship

	// This would require searching the entire codebase for references
	// For now, implement a basic version that searches in the same package
	packageDir := filepath.Dir(filePath)

	// Search for usages in package files (simplified implementation)
	commonFiles := []string{
		"config.go", "types.go", "constants.go", "errors.go", "utils.go",
		"helpers.go", "models.go", "handlers.go", "service.go", "repository.go",
	}

	for _, filename := range commonFiles {
		fullPath := filepath.Join(packageDir, filename)
		if fullPath == filePath {
			continue // Skip the same file
		}

		content, err := dm.provider.GetFileContent(ctx, request.ProjectID, fullPath, request.MergeRequest.TargetBranch)
		if err != nil {
			continue // File doesn't exist or can't be read
		}

		// Search for references to the entity
		usages := dm.findEntityUsages(entity.Name, content, fullPath)
		dependents = append(dependents, usages...)
	}

	return dependents, nil
}

// findEntityUsages finds usages of an entity in code content
func (dm *DependencyMapper) findEntityUsages(entityName, content, filePath string) []Relationship {
	var usages []Relationship

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		if strings.Contains(line, entityName) {
			// Create a basic usage relationship
			usages = append(usages, Relationship{
				Target:      entityName,
				Type:        RelationshipFunctionCall, // Simplified - could be more specific
				Context:     "usage",
				FilePath:    filePath,
				LineNumber:  lineNum + 1,
				CodeSnippet: strings.TrimSpace(line),
				Strength:    0.5,
				IsExternal:  false,
			})
		}
	}

	return usages
}

// analyzeImports analyzes import relationships
func (dm *DependencyMapper) analyzeImports(ctx context.Context, request model.ReviewRequest, filePath string) (map[string][]ImportUsage, error) {
	importUsages := make(map[string][]ImportUsage)

	// Get file content
	content, err := dm.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.SHA)
	if err != nil {
		return importUsages, fmt.Errorf("failed to get file content: %w", err)
	}

	// Parse imports
	importRegex := regexp.MustCompile(`import\s+(?:([a-zA-Z_][a-zA-Z0-9_]*)\s+)?"([^"]+)"`)
	matches := importRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			importPath := match[2]
			alias := match[1]

			usage := ImportUsage{
				ImportPath:    importPath,
				Alias:         alias,
				FilePath:      filePath,
				UsageCount:    dm.countImportUsage(importPath, alias, content),
				IsStandardLib: dm.isStandardLibrary(importPath),
				IsThirdParty:  dm.isThirdPartyLibrary(importPath),
			}

			importUsages[importPath] = append(importUsages[importPath], usage)
		}
	}

	return importUsages, nil
}

// countImportUsage counts how often an import is used
func (dm *DependencyMapper) countImportUsage(importPath, alias, content string) int {
	packageName := alias
	if packageName == "" {
		parts := strings.Split(importPath, "/")
		packageName = parts[len(parts)-1]
	}

	// Count occurrences of package usage
	count := strings.Count(content, packageName+".")
	return count
}

// isStandardLibrary checks if an import is from the standard library
func (dm *DependencyMapper) isStandardLibrary(importPath string) bool {
	// Simplified check - stdlib packages don't usually have dots in the first segment
	firstSegment := strings.Split(importPath, "/")[0]
	return !strings.Contains(firstSegment, ".")
}

// isThirdPartyLibrary checks if an import is from a third-party library
func (dm *DependencyMapper) isThirdPartyLibrary(importPath string) bool {
	return !dm.isStandardLibrary(importPath) && !strings.HasPrefix(importPath, "github.com/maxbolgarin/codry")
}

// buildPackageScope builds a map of packages to their entities
func (dm *DependencyMapper) buildPackageScope(ctx context.Context, request model.ReviewRequest, graph *DependencyGraph, filePath string) error {
	packageName := dm.extractPackageFromPath(filePath)

	var entityIDs []string
	for entityID, entity := range graph.Entities {
		if entity.Package == packageName {
			entityIDs = append(entityIDs, entityID)
		}
	}

	graph.PackageScope[packageName] = entityIDs
	return nil
}

// isExternalEntity checks if an entity is external to the project
func (dm *DependencyMapper) isExternalEntity(entityName string) bool {
	// Simple heuristic - if it contains a dot and starts with lowercase, it's likely external
	if strings.Contains(entityName, ".") {
		parts := strings.Split(entityName, ".")
		if len(parts) > 0 && len(parts[0]) > 0 {
			return parts[0][0] >= 'a' && parts[0][0] <= 'z'
		}
	}
	return false
}

// inferBusinessArea infers business area from file path and entity name
func (dm *DependencyMapper) inferBusinessArea(filePath, entityName string) string {
	pathLower := strings.ToLower(filePath)
	nameLower := strings.ToLower(entityName)

	if strings.Contains(pathLower, "auth") || strings.Contains(nameLower, "auth") {
		return "authentication"
	}
	if strings.Contains(pathLower, "payment") || strings.Contains(nameLower, "payment") {
		return "payment"
	}
	if strings.Contains(pathLower, "user") || strings.Contains(nameLower, "user") {
		return "user_management"
	}
	if strings.Contains(pathLower, "api") || strings.Contains(pathLower, "handler") {
		return "api"
	}
	if strings.Contains(pathLower, "config") || strings.Contains(nameLower, "config") {
		return "configuration"
	}

	return "general"
}

// inferSecurityLevel infers security sensitivity level
func (dm *DependencyMapper) inferSecurityLevel(filePath, entityName, code string) string {
	combined := strings.ToLower(filePath + " " + entityName + " " + code)

	if strings.Contains(combined, "password") || strings.Contains(combined, "secret") ||
		strings.Contains(combined, "token") || strings.Contains(combined, "key") ||
		strings.Contains(combined, "auth") || strings.Contains(combined, "crypto") {
		return "high"
	}

	if strings.Contains(combined, "user") || strings.Contains(combined, "session") ||
		strings.Contains(combined, "login") || strings.Contains(combined, "validate") {
		return "medium"
	}

	return "low"
}
