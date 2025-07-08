package analyze

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/logze/v2"
)

// SupportedLanguage represents different programming languages
type SupportedLanguage string

const (
	LanguageGo         SupportedLanguage = "go"
	LanguageJavaScript SupportedLanguage = "javascript"
	LanguageTypeScript SupportedLanguage = "typescript"
	LanguagePython     SupportedLanguage = "python"
	LanguageJava       SupportedLanguage = "java"
	LanguageRust       SupportedLanguage = "rust"
	LanguageCpp        SupportedLanguage = "cpp"
	LanguageC          SupportedLanguage = "c"
	LanguageUnknown    SupportedLanguage = "unknown"
)

// SemanticAnalyzer provides deep semantic analysis of code changes
type SemanticAnalyzer struct {
	provider interfaces.CodeProvider
	log      logze.Logger
}

// NewSemanticAnalyzer creates a new semantic analyzer
func NewSemanticAnalyzer(provider interfaces.CodeProvider) *SemanticAnalyzer {
	return &SemanticAnalyzer{
		provider: provider,
		log:      logze.With("component", "semantic-analyzer"),
	}
}

// ChangedEntity represents a specific code entity that was changed
type ChangedEntity struct {
	Type         EntityType   `json:"type"`         // function, method, type, struct, interface, const, var
	Name         string       `json:"name"`         // entity name
	FullName     string       `json:"full_name"`    // package.Type.Method or package.Function
	Package      string       `json:"package"`      // package name
	IsExported   bool         `json:"is_exported"`  // whether it's exported
	StartLine    int          `json:"start_line"`   // start line in file
	EndLine      int          `json:"end_line"`     // end line in file
	ChangeType   ChangeType   `json:"change_type"`  // added, modified, deleted
	BeforeCode   string       `json:"before_code"`  // code before change
	AfterCode    string       `json:"after_code"`   // code after change
	Signature    string       `json:"signature"`    // function/method signature
	DocComment   string       `json:"doc_comment"`  // documentation comment
	Dependencies []Dependency `json:"dependencies"` // what this entity depends on
	Dependents   []Dependent  `json:"dependents"`   // what depends on this entity
}

// EntityType represents the type of code entity
type EntityType string

const (
	EntityTypeFunction  EntityType = "function"
	EntityTypeMethod    EntityType = "method"
	EntityTypeType      EntityType = "type"
	EntityTypeStruct    EntityType = "struct"
	EntityTypeInterface EntityType = "interface"
	EntityTypeConst     EntityType = "const"
	EntityTypeVar       EntityType = "var"
	EntityTypeImport    EntityType = "import"
)

// ChangeType represents how an entity was changed
type ChangeType string

const (
	ChangeTypeAdded    ChangeType = "added"
	ChangeTypeModified ChangeType = "modified"
	ChangeTypeDeleted  ChangeType = "deleted"
)

// Dependency represents something this entity depends on
type Dependency struct {
	Name         string     `json:"name"`          // name of the dependency
	Type         EntityType `json:"type"`          // type of dependency
	Package      string     `json:"package"`       // package of dependency
	IsExternal   bool       `json:"is_external"`   // whether it's external to the project
	UsageContext string     `json:"usage_context"` // how it's used (parameter, return, field, etc.)
	CodeSnippet  string     `json:"code_snippet"`  // relevant code snippet
}

// Dependent represents something that depends on this entity
type Dependent struct {
	Name         string     `json:"name"`          // name of the dependent
	Type         EntityType `json:"type"`          // type of dependent
	Package      string     `json:"package"`       // package of dependent
	FilePath     string     `json:"file_path"`     // file path of dependent
	UsageContext string     `json:"usage_context"` // how it uses this entity
	CodeSnippet  string     `json:"code_snippet"`  // relevant code snippet
}

// SemanticAnalysisResult contains the results of semantic analysis
type SemanticAnalysisResult struct {
	ChangedEntities    []ChangedEntity    `json:"changed_entities"`
	ImpactAnalysis     ImpactAnalysis     `json:"impact_analysis"`
	BusinessContext    BusinessContext    `json:"business_context"`
	ArchitecturalScope ArchitecturalScope `json:"architectural_scope"`
	ProjectPatterns    ProjectPatterns    `json:"project_patterns"`
}

// ImpactAnalysis analyzes the potential impact of changes
type ImpactAnalysis struct {
	Scope           string           `json:"scope"`            // local, package, project, external
	RiskLevel       string           `json:"risk_level"`       // low, medium, high, critical
	AffectedAreas   []string         `json:"affected_areas"`   // business areas affected
	BreakingChanges []BreakingChange `json:"breaking_changes"` // potential breaking changes
	TestImpact      TestImpact       `json:"test_impact"`      // testing implications
}

// BusinessContext provides business-level context for changes
type BusinessContext struct {
	Domain          string   `json:"domain"`           // business domain (auth, payment, etc.)
	Criticality     string   `json:"criticality"`      // business criticality
	UserImpact      string   `json:"user_impact"`      // impact on users
	DataSensitivity string   `json:"data_sensitivity"` // data sensitivity level
	ComplianceAreas []string `json:"compliance_areas"` // compliance considerations
}

// ArchitecturalScope defines the architectural scope of changes
type ArchitecturalScope struct {
	Layer        string   `json:"layer"`        // presentation, business, data, etc.
	Components   []string `json:"components"`   // affected architectural components
	Boundaries   []string `json:"boundaries"`   // crossed architectural boundaries
	Patterns     []string `json:"patterns"`     // architectural patterns involved
	Dependencies []string `json:"dependencies"` // external dependencies affected
}

// ProjectPatterns contains project-specific patterns and conventions
type ProjectPatterns struct {
	CodingStyle        CodingStyleInfo   `json:"coding_style"`
	ErrorHandling      ErrorHandlingInfo `json:"error_handling"`
	TestingPatterns    TestingInfo       `json:"testing_patterns"`
	ArchitecturalStyle ArchitecturalInfo `json:"architectural_style"`
	SecurityPatterns   SecurityInfo      `json:"security_patterns"`
}

// Supporting types for project patterns
type CodingStyleInfo struct {
	NamingConventions  []string `json:"naming_conventions"`
	StructurePatterns  []string `json:"structure_patterns"`
	CommentingStyle    string   `json:"commenting_style"`
	ImportOrganization string   `json:"import_organization"`
}

type ErrorHandlingInfo struct {
	Pattern    string   `json:"pattern"`     // error handling pattern used
	WrapStyle  string   `json:"wrap_style"`  // how errors are wrapped
	LoggingLib string   `json:"logging_lib"` // logging library used
	Examples   []string `json:"examples"`    // example error handling code
}

type TestingInfo struct {
	Framework     string   `json:"framework"`      // testing framework
	Patterns      []string `json:"patterns"`       // testing patterns
	CoverageStyle string   `json:"coverage_style"` // coverage expectations
	MockingStyle  string   `json:"mocking_style"`  // mocking approach
}

type ArchitecturalInfo struct {
	LayerPattern   string   `json:"layer_pattern"`   // architectural pattern
	DIPattern      string   `json:"di_pattern"`      // dependency injection
	ConfigPattern  string   `json:"config_pattern"`  // configuration pattern
	DesignPatterns []string `json:"design_patterns"` // common design patterns
}

type SecurityInfo struct {
	AuthPattern     string   `json:"auth_pattern"`     // authentication pattern
	InputValidation string   `json:"input_validation"` // input validation approach
	CryptoUsage     []string `json:"crypto_usage"`     // cryptographic patterns
	DataProtection  string   `json:"data_protection"`  // data protection approach
}

type BreakingChange struct {
	Type        string `json:"type"`        // signature, behavior, removal
	Entity      string `json:"entity"`      // affected entity
	Description string `json:"description"` // description of the breaking change
	Mitigation  string `json:"mitigation"`  // suggested mitigation
}

type TestImpact struct {
	RequiresNewTests      bool     `json:"requires_new_tests"`
	AffectedTestFiles     []string `json:"affected_test_files"`
	TestingStrategy       string   `json:"testing_strategy"`
	MockingConsiderations string   `json:"mocking_considerations"`
}

// AnalyzeChanges performs deep semantic analysis of code changes
func (sa *SemanticAnalyzer) AnalyzeChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "project", request.ProjectID)
	log.Debug("starting semantic analysis")

	result := &SemanticAnalysisResult{}

	// Detect language from file path
	language := sa.detectLanguage(fileDiff.NewPath)
	log.Debug("detected language", "language", language)

	// Use language-specific analysis strategy
	switch language {
	case LanguageGo:
		return sa.analyzeGoChanges(ctx, request, fileDiff, result)
	case LanguageJavaScript, LanguageTypeScript:
		return sa.analyzeJSChanges(ctx, request, fileDiff, result)
	case LanguagePython:
		return sa.analyzePythonChanges(ctx, request, fileDiff, result)
	case LanguageJava:
		return sa.analyzeJavaChanges(ctx, request, fileDiff, result)
	case LanguageRust:
		return sa.analyzeRustChanges(ctx, request, fileDiff, result)
	case LanguageC, LanguageCpp:
		return sa.analyzeCChanges(ctx, request, fileDiff, result)
	default:
		return sa.analyzeGenericChanges(ctx, request, fileDiff, result)
	}
}

// analyzeGoChanges performs Go-specific semantic analysis
func (sa *SemanticAnalyzer) analyzeGoChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "go")

	// Parse the file to understand its structure using Go AST
	beforeAST, afterAST, err := sa.parseFileVersions(ctx, request, fileDiff)
	if err != nil {
		log.Warn("failed to parse Go AST, falling back to diff analysis", "error", err)
		return sa.analyzeGenericChanges(ctx, request, fileDiff, result)
	}

	// Identify changed entities using AST comparison for Go
	result.ChangedEntities, err = sa.identifyChangedEntities(beforeAST, afterAST, fileDiff)
	if err != nil {
		log.Warn("failed to identify changed entities", "error", err)
	}

	// Go-specific dependency analysis
	err = sa.analyzeDependencies(ctx, request, result.ChangedEntities, fileDiff.NewPath)
	if err != nil {
		log.Warn("failed to analyze dependencies", "error", err)
	}

	// Go-specific dependent analysis
	err = sa.analyzeDependents(ctx, request, result.ChangedEntities, fileDiff.NewPath)
	if err != nil {
		log.Warn("failed to analyze dependents", "error", err)
	}

	// Perform impact analysis
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)

	// Determine business context
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)

	// Determine architectural scope
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)

	// Analyze Go-specific project patterns
	result.ProjectPatterns, err = sa.analyzeGoProjectPatterns(ctx, request, fileDiff.NewPath)
	if err != nil {
		log.Warn("failed to analyze project patterns", "error", err)
	}

	log.Debug("Go semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzeJSChanges performs JavaScript/TypeScript-specific semantic analysis
func (sa *SemanticAnalyzer) analyzeJSChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "javascript")

	// Extract entities from diff using JS-specific patterns
	result.ChangedEntities = sa.extractJSEntitiesFromDiff(fileDiff)

	// Perform basic impact analysis
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)

	// JS-specific project patterns
	result.ProjectPatterns = sa.analyzeJSProjectPatterns()

	log.Debug("JavaScript semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzePythonChanges performs Python-specific semantic analysis
func (sa *SemanticAnalyzer) analyzePythonChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "python")

	// Extract entities from diff using Python-specific patterns
	result.ChangedEntities = sa.extractPythonEntitiesFromDiff(fileDiff)

	// Perform basic analysis
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)
	result.ProjectPatterns = sa.analyzePythonProjectPatterns()

	log.Debug("Python semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzeJavaChanges performs Java-specific semantic analysis
func (sa *SemanticAnalyzer) analyzeJavaChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "java")

	result.ChangedEntities = sa.extractJavaEntitiesFromDiff(fileDiff)
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)
	result.ProjectPatterns = sa.analyzeJavaProjectPatterns()

	log.Debug("Java semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzeRustChanges performs Rust-specific semantic analysis
func (sa *SemanticAnalyzer) analyzeRustChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "rust")

	result.ChangedEntities = sa.extractRustEntitiesFromDiff(fileDiff)
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)
	result.ProjectPatterns = sa.analyzeRustProjectPatterns()

	log.Debug("Rust semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzeCChanges performs C/C++-specific semantic analysis
func (sa *SemanticAnalyzer) analyzeCChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "c")

	result.ChangedEntities = sa.extractCEntitiesFromDiff(fileDiff)
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)
	result.ProjectPatterns = sa.analyzeCProjectPatterns()

	log.Debug("C/C++ semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// analyzeGenericChanges performs generic analysis for unknown languages
func (sa *SemanticAnalyzer) analyzeGenericChanges(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, result *SemanticAnalysisResult) (*SemanticAnalysisResult, error) {
	log := sa.log.WithFields("file", fileDiff.NewPath, "language", "generic")

	// Use basic diff analysis for unknown languages
	result.ChangedEntities = sa.extractEntitiesFromDiff(fileDiff)
	result.ImpactAnalysis = sa.analyzeImpact(result.ChangedEntities)
	result.BusinessContext = sa.analyzeBusinessContext(fileDiff.NewPath, result.ChangedEntities)
	result.ArchitecturalScope = sa.analyzeArchitecturalScope(fileDiff.NewPath, result.ChangedEntities)
	result.ProjectPatterns = sa.analyzeGenericProjectPatterns()

	log.Debug("Generic semantic analysis completed", "changed_entities", len(result.ChangedEntities))
	return result, nil
}

// parseFileVersions parses both the before and after versions of a file
func (sa *SemanticAnalyzer) parseFileVersions(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*ast.File, *ast.File, error) {
	var beforeAST, afterAST *ast.File
	var err error

	fset := token.NewFileSet()

	// Parse before version (if file is not new)
	if !fileDiff.IsNew {
		beforeContent, contentErr := sa.getFileContent(ctx, request, fileDiff.OldPath, request.MergeRequest.TargetBranch)
		if contentErr == nil {
			beforeAST, err = parser.ParseFile(fset, fileDiff.OldPath, beforeContent, parser.ParseComments)
			if err != nil {
				sa.log.Warn("failed to parse before version", "error", err)
			}
		}
	}

	// Parse after version (if file is not deleted)
	if !fileDiff.IsDeleted {
		afterContent, contentErr := sa.getFileContent(ctx, request, fileDiff.NewPath, request.MergeRequest.SHA)
		if contentErr == nil {
			afterAST, err = parser.ParseFile(fset, fileDiff.NewPath, afterContent, parser.ParseComments)
			if err != nil {
				sa.log.Warn("failed to parse after version", "error", err)
			}
		}
	}

	return beforeAST, afterAST, nil
}

// getFileContent retrieves file content with fallback strategies
func (sa *SemanticAnalyzer) getFileContent(ctx context.Context, request model.ReviewRequest, filePath, ref string) (string, error) {
	content, err := sa.provider.GetFileContent(ctx, request.ProjectID, filePath, ref)
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}
	return content, nil
}

// identifyChangedEntities compares ASTs to identify what entities have changed
func (sa *SemanticAnalyzer) identifyChangedEntities(beforeAST, afterAST *ast.File, fileDiff *model.FileDiff) ([]ChangedEntity, error) {
	var entities []ChangedEntity

	// For now, implement a basic version that analyzes the diff
	// In a more sophisticated version, we would compare the ASTs directly
	entities = sa.extractEntitiesFromDiff(fileDiff)

	return entities, nil
}

// extractEntitiesFromDiff extracts entities from diff analysis (basic implementation)
func (sa *SemanticAnalyzer) extractEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// Parse diff lines to identify function/type changes
	lines := strings.Split(fileDiff.Diff, "\n")
	currentEntity := &ChangedEntity{}
	inEntity := false

	for i, line := range lines {
		// Look for function definitions
		if strings.HasPrefix(line, "+func ") || strings.HasPrefix(line, "-func ") {
			if inEntity && currentEntity.Name != "" {
				entities = append(entities, *currentEntity)
			}

			currentEntity = &ChangedEntity{
				Type:       EntityTypeFunction,
				ChangeType: sa.getChangeTypeFromLine(line),
				StartLine:  i + 1,
			}

			// Extract function name and signature
			funcInfo := sa.parseFunctionSignature(line)
			currentEntity.Name = funcInfo.Name
			currentEntity.Signature = funcInfo.Signature
			currentEntity.IsExported = sa.isExported(funcInfo.Name)

			if strings.HasPrefix(line, "+") {
				currentEntity.AfterCode = strings.TrimPrefix(line, "+")
			} else {
				currentEntity.BeforeCode = strings.TrimPrefix(line, "-")
			}

			inEntity = true
		} else if strings.HasPrefix(line, "+type ") || strings.HasPrefix(line, "-type ") {
			if inEntity && currentEntity.Name != "" {
				entities = append(entities, *currentEntity)
			}

			currentEntity = &ChangedEntity{
				Type:       EntityTypeType,
				ChangeType: sa.getChangeTypeFromLine(line),
				StartLine:  i + 1,
			}

			// Extract type name
			typeInfo := sa.parseTypeDefinition(line)
			currentEntity.Name = typeInfo.Name
			currentEntity.IsExported = sa.isExported(typeInfo.Name)

			if strings.HasPrefix(line, "+") {
				currentEntity.AfterCode = strings.TrimPrefix(line, "+")
			} else {
				currentEntity.BeforeCode = strings.TrimPrefix(line, "-")
			}

			inEntity = true
		} else if inEntity {
			// Continue collecting entity code
			if strings.HasPrefix(line, "+") {
				if currentEntity.AfterCode != "" {
					currentEntity.AfterCode += "\n"
				}
				currentEntity.AfterCode += strings.TrimPrefix(line, "+")
			} else if strings.HasPrefix(line, "-") {
				if currentEntity.BeforeCode != "" {
					currentEntity.BeforeCode += "\n"
				}
				currentEntity.BeforeCode += strings.TrimPrefix(line, "-")
			}

			// Check if we've reached the end of the entity
			if strings.TrimSpace(line) == "}" ||
				(strings.HasPrefix(line, "+func ") || strings.HasPrefix(line, "-func ")) ||
				(strings.HasPrefix(line, "+type ") || strings.HasPrefix(line, "-type ")) {
				currentEntity.EndLine = i + 1
			}
		}
	}

	// Add the last entity if we were processing one
	if inEntity && currentEntity.Name != "" {
		entities = append(entities, *currentEntity)
	}

	return entities
}

// Helper functions for parsing

type FunctionInfo struct {
	Name      string
	Signature string
	Receiver  string
}

type TypeInfo struct {
	Name string
	Kind string // struct, interface, etc.
}

func (sa *SemanticAnalyzer) parseFunctionSignature(line string) FunctionInfo {
	// Remove diff prefix
	cleanLine := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+"), "-"))

	// Basic function parsing (can be improved with proper AST parsing)
	funcRegex := regexp.MustCompile(`func\s*(?:\([^)]*\))?\s*([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)(?:\s*[^{]*)?`)
	matches := funcRegex.FindStringSubmatch(cleanLine)

	if len(matches) >= 2 {
		return FunctionInfo{
			Name:      matches[1],
			Signature: cleanLine,
		}
	}

	return FunctionInfo{}
}

func (sa *SemanticAnalyzer) parseTypeDefinition(line string) TypeInfo {
	// Remove diff prefix
	cleanLine := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+"), "-"))

	// Basic type parsing
	typeRegex := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface|\S+)`)
	matches := typeRegex.FindStringSubmatch(cleanLine)

	if len(matches) >= 3 {
		return TypeInfo{
			Name: matches[1],
			Kind: matches[2],
		}
	}

	return TypeInfo{}
}

func (sa *SemanticAnalyzer) getChangeTypeFromLine(line string) ChangeType {
	if strings.HasPrefix(line, "+") {
		return ChangeTypeAdded
	} else if strings.HasPrefix(line, "-") {
		return ChangeTypeDeleted
	}
	return ChangeTypeModified
}

func (sa *SemanticAnalyzer) isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// analyzeDependencies finds what each changed entity depends on
func (sa *SemanticAnalyzer) analyzeDependencies(ctx context.Context, request model.ReviewRequest, entities []ChangedEntity, filePath string) error {
	// For each entity, analyze its dependencies
	for i := range entities {
		entity := &entities[i]

		// Analyze code to find dependencies
		dependencies := sa.extractDependenciesFromCode(entity.AfterCode)
		entity.Dependencies = dependencies
	}

	return nil
}

// analyzeDependents finds what depends on each changed entity
func (sa *SemanticAnalyzer) analyzeDependents(ctx context.Context, request model.ReviewRequest, entities []ChangedEntity, filePath string) error {
	// This would require searching the entire codebase for references
	// For now, implement a basic version
	for i := range entities {
		entity := &entities[i]

		// Find dependents by searching for usage patterns
		dependents, err := sa.findDependents(ctx, request, entity.Name, filepath.Dir(filePath))
		if err != nil {
			sa.log.Warn("failed to find dependents", "entity", entity.Name, "error", err)
			continue
		}

		entity.Dependents = dependents
	}

	return nil
}

// extractDependenciesFromCode analyzes code to find its dependencies
func (sa *SemanticAnalyzer) extractDependenciesFromCode(code string) []Dependency {
	var dependencies []Dependency

	// Look for function calls, type usage, etc.
	// This is a simplified implementation
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for function calls (basic pattern)
		callRegex := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
		matches := callRegex.FindAllStringSubmatch(line, -1)

		for _, match := range matches {
			if len(match) >= 2 {
				dependencies = append(dependencies, Dependency{
					Name:         match[1],
					Type:         EntityTypeFunction,
					UsageContext: "function_call",
					CodeSnippet:  line,
				})
			}
		}
	}

	return dependencies
}

// findDependents searches for entities that depend on the given entity
func (sa *SemanticAnalyzer) findDependents(ctx context.Context, request model.ReviewRequest, entityName, packagePath string) ([]Dependent, error) {
	var dependents []Dependent

	// This is a simplified implementation - in practice, we'd want to:
	// 1. Search through Go files in the package
	// 2. Look for references to the entity
	// 3. Analyze imports and usage patterns

	return dependents, nil
}

// analyzeImpact determines the impact scope and risk level of changes
func (sa *SemanticAnalyzer) analyzeImpact(entities []ChangedEntity) ImpactAnalysis {
	impact := ImpactAnalysis{
		Scope:     "local",
		RiskLevel: "low",
	}

	// Analyze entities to determine scope and risk
	hasExportedChanges := false
	hasBreakingChanges := false

	for _, entity := range entities {
		if entity.IsExported {
			hasExportedChanges = true
			if entity.ChangeType == ChangeTypeDeleted || sa.isSignatureChange(entity) {
				hasBreakingChanges = true
				impact.BreakingChanges = append(impact.BreakingChanges, BreakingChange{
					Type:        "signature",
					Entity:      entity.Name,
					Description: fmt.Sprintf("%s %s has breaking changes", entity.Type, entity.Name),
				})
			}
		}
	}

	// Determine scope
	if hasExportedChanges {
		impact.Scope = "package"
		if len(entities) > 3 {
			impact.Scope = "project"
		}
	}

	// Determine risk level
	if hasBreakingChanges {
		impact.RiskLevel = "high"
	} else if hasExportedChanges {
		impact.RiskLevel = "medium"
	}

	return impact
}

func (sa *SemanticAnalyzer) isSignatureChange(entity ChangedEntity) bool {
	// Simple check for signature changes - this could be more sophisticated
	return entity.BeforeCode != "" && entity.AfterCode != "" &&
		entity.BeforeCode != entity.AfterCode
}

// analyzeBusinessContext determines business context from file path and entities
func (sa *SemanticAnalyzer) analyzeBusinessContext(filePath string, entities []ChangedEntity) BusinessContext {
	context := BusinessContext{
		Criticality: "medium",
		UserImpact:  "low",
	}

	// Infer domain from file path
	pathLower := strings.ToLower(filePath)
	if strings.Contains(pathLower, "auth") {
		context.Domain = "authentication"
		context.Criticality = "high"
		context.DataSensitivity = "high"
		context.ComplianceAreas = []string{"security", "privacy"}
	} else if strings.Contains(pathLower, "payment") {
		context.Domain = "payment"
		context.Criticality = "critical"
		context.DataSensitivity = "critical"
		context.ComplianceAreas = []string{"pci", "financial"}
	} else if strings.Contains(pathLower, "user") {
		context.Domain = "user_management"
		context.Criticality = "high"
		context.DataSensitivity = "high"
		context.ComplianceAreas = []string{"privacy", "gdpr"}
	} else if strings.Contains(pathLower, "api") || strings.Contains(pathLower, "handler") {
		context.Domain = "api"
		context.UserImpact = "medium"
	}

	return context
}

// analyzeArchitecturalScope determines architectural scope of changes
func (sa *SemanticAnalyzer) analyzeArchitecturalScope(filePath string, entities []ChangedEntity) ArchitecturalScope {
	scope := ArchitecturalScope{}

	// Infer layer from file path
	pathLower := strings.ToLower(filePath)
	if strings.Contains(pathLower, "handler") || strings.Contains(pathLower, "controller") {
		scope.Layer = "presentation"
	} else if strings.Contains(pathLower, "service") || strings.Contains(pathLower, "business") {
		scope.Layer = "business"
	} else if strings.Contains(pathLower, "repository") || strings.Contains(pathLower, "dao") {
		scope.Layer = "data"
	} else if strings.Contains(pathLower, "model") || strings.Contains(pathLower, "entity") {
		scope.Layer = "domain"
	}

	// Analyze components and patterns
	for _, entity := range entities {
		entityName := strings.ToLower(entity.Name)
		if strings.Contains(entityName, "config") {
			scope.Components = append(scope.Components, "configuration")
		}
		if strings.Contains(entityName, "cache") {
			scope.Components = append(scope.Components, "caching")
		}
		if strings.Contains(entityName, "log") {
			scope.Components = append(scope.Components, "logging")
		}
	}

	return scope
}

// analyzeProjectPatterns analyzes project-specific patterns and conventions
func (sa *SemanticAnalyzer) analyzeProjectPatterns(ctx context.Context, request model.ReviewRequest, filePath string) (ProjectPatterns, error) {
	patterns := ProjectPatterns{}

	// This will be expanded to analyze:
	// 1. Linter configuration
	// 2. Go.mod dependencies
	// 3. Neighboring files in the same package
	// 4. Common patterns used throughout the project

	// For now, return basic patterns
	patterns.CodingStyle = CodingStyleInfo{
		NamingConventions: []string{"camelCase", "PascalCase"},
		CommentingStyle:   "go_doc",
	}

	patterns.ErrorHandling = ErrorHandlingInfo{
		Pattern:    "wrapped_errors",
		LoggingLib: "logze",
	}

	return patterns, nil
}

// detectLanguage detects programming language from file path
func (sa *SemanticAnalyzer) detectLanguage(filePath string) SupportedLanguage {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return LanguageGo
	case ".js", ".jsx":
		return LanguageJavaScript
	case ".ts", ".tsx":
		return LanguageTypeScript
	case ".py", ".pyw":
		return LanguagePython
	case ".java":
		return LanguageJava
	case ".rs":
		return LanguageRust
	case ".cpp", ".cxx", ".cc":
		return LanguageCpp
	case ".c", ".h":
		return LanguageC
	default:
		return LanguageUnknown
	}
}

// Language-specific entity extraction methods

// extractJSEntitiesFromDiff extracts JavaScript/TypeScript entities from diff
func (sa *SemanticAnalyzer) extractJSEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// JS function patterns: function name(), const name = () => {}, name() {}
	functionPatterns := []string{
		`function\s+(\w+)\s*\(`,
		`const\s+(\w+)\s*=\s*\([^)]*\)\s*=>\s*{`,
		`(\w+)\s*:\s*function\s*\(`,
		`(\w+)\s*\([^)]*\)\s*{`,
	}

	// JS class/interface patterns
	classPatterns := []string{
		`class\s+(\w+)`,
		`interface\s+(\w+)`,
		`type\s+(\w+)\s*=`,
	}

	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, functionPatterns, EntityTypeFunction)...)
	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, classPatterns, EntityTypeType)...)

	return entities
}

// extractPythonEntitiesFromDiff extracts Python entities from diff
func (sa *SemanticAnalyzer) extractPythonEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// Python function patterns: def name():, async def name():
	functionPatterns := []string{
		`def\s+(\w+)\s*\(`,
		`async\s+def\s+(\w+)\s*\(`,
	}

	// Python class patterns
	classPatterns := []string{
		`class\s+(\w+)`,
	}

	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, functionPatterns, EntityTypeFunction)...)
	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, classPatterns, EntityTypeType)...)

	return entities
}

// extractJavaEntitiesFromDiff extracts Java entities from diff
func (sa *SemanticAnalyzer) extractJavaEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// Java method patterns: public/private/protected void name(), static type name()
	functionPatterns := []string{
		`(?:public|private|protected)?\s*(?:static\s+)?[\w<>\[\]]+\s+(\w+)\s*\(`,
	}

	// Java class/interface patterns
	classPatterns := []string{
		`(?:public\s+)?(?:abstract\s+)?class\s+(\w+)`,
		`(?:public\s+)?interface\s+(\w+)`,
		`(?:public\s+)?enum\s+(\w+)`,
	}

	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, functionPatterns, EntityTypeFunction)...)
	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, classPatterns, EntityTypeType)...)

	return entities
}

// extractRustEntitiesFromDiff extracts Rust entities from diff
func (sa *SemanticAnalyzer) extractRustEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// Rust function patterns: fn name(), pub fn name()
	functionPatterns := []string{
		`(?:pub\s+)?fn\s+(\w+)\s*\(`,
	}

	// Rust struct/enum/trait patterns
	typePatterns := []string{
		`(?:pub\s+)?struct\s+(\w+)`,
		`(?:pub\s+)?enum\s+(\w+)`,
		`(?:pub\s+)?trait\s+(\w+)`,
		`type\s+(\w+)\s*=`,
	}

	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, functionPatterns, EntityTypeFunction)...)
	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, typePatterns, EntityTypeType)...)

	return entities
}

// extractCEntitiesFromDiff extracts C/C++ entities from diff
func (sa *SemanticAnalyzer) extractCEntitiesFromDiff(fileDiff *model.FileDiff) []ChangedEntity {
	var entities []ChangedEntity

	// C/C++ function patterns
	functionPatterns := []string{
		`(?:static\s+)?[\w\*]+\s+(\w+)\s*\([^)]*\)\s*{`,
		`(?:inline\s+)?(?:virtual\s+)?[\w\*]+\s+(\w+)\s*\([^)]*\)\s*{`,
	}

	// C/C++ struct/class patterns
	typePatterns := []string{
		`struct\s+(\w+)`,
		`class\s+(\w+)`,
		`typedef\s+[\w\s]+\s+(\w+)`,
		`enum\s+(\w+)`,
	}

	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, functionPatterns, EntityTypeFunction)...)
	entities = append(entities, sa.extractEntitiesWithPatterns(fileDiff, typePatterns, EntityTypeType)...)

	return entities
}

// extractEntitiesWithPatterns is a helper method to extract entities using regex patterns
func (sa *SemanticAnalyzer) extractEntitiesWithPatterns(fileDiff *model.FileDiff, patterns []string, entityType EntityType) []ChangedEntity {
	var entities []ChangedEntity

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		matches := regex.FindAllStringSubmatch(fileDiff.Diff, -1)

		for _, match := range matches {
			if len(match) > 1 {
				entity := ChangedEntity{
					Type:       entityType,
					Name:       match[1],
					ChangeType: ChangeTypeModified, // Default assumption
					IsExported: sa.isExported(match[1]),
				}
				entities = append(entities, entity)
			}
		}
	}

	return entities
}

// Language-specific project pattern analysis methods

// analyzeGoProjectPatterns analyzes Go-specific project patterns
func (sa *SemanticAnalyzer) analyzeGoProjectPatterns(ctx context.Context, request model.ReviewRequest, filePath string) (ProjectPatterns, error) {
	// This would look for go.mod, .golangci.yml, etc.
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"camelCase for unexported", "PascalCase for exported"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "explicit error returns",
		},
		TestingPatterns: TestingInfo{
			Framework: "testing",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "clean architecture",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "JWT/OAuth",
		},
	}, nil
}

// analyzeJSProjectPatterns analyzes JavaScript/TypeScript project patterns
func (sa *SemanticAnalyzer) analyzeJSProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"camelCase", "kebab-case for files"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "try-catch or promises",
		},
		TestingPatterns: TestingInfo{
			Framework: "jest",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "component-based",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "session/JWT",
		},
	}
}

// analyzePythonProjectPatterns analyzes Python project patterns
func (sa *SemanticAnalyzer) analyzePythonProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"snake_case", "PascalCase for classes"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "exceptions",
		},
		TestingPatterns: TestingInfo{
			Framework: "pytest",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "MVC/Django patterns",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "Django auth/Flask-Login",
		},
	}
}

// analyzeJavaProjectPatterns analyzes Java project patterns
func (sa *SemanticAnalyzer) analyzeJavaProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"camelCase", "PascalCase for classes"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "checked exceptions",
		},
		TestingPatterns: TestingInfo{
			Framework: "junit",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "Spring/layered",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "Spring Security",
		},
	}
}

// analyzeRustProjectPatterns analyzes Rust project patterns
func (sa *SemanticAnalyzer) analyzeRustProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"snake_case", "PascalCase for types"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "Result<T, E> types",
		},
		TestingPatterns: TestingInfo{
			Framework: "cargo test",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "ownership-based",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "memory-safe by design",
		},
	}
}

// analyzeCProjectPatterns analyzes C/C++ project patterns
func (sa *SemanticAnalyzer) analyzeCProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"snake_case", "camelCase"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "error codes/errno",
		},
		TestingPatterns: TestingInfo{
			Framework: "gtest",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "procedural/OOP",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "manual validation",
		},
	}
}

// analyzeGenericProjectPatterns provides basic patterns for unknown languages
func (sa *SemanticAnalyzer) analyzeGenericProjectPatterns() ProjectPatterns {
	return ProjectPatterns{
		CodingStyle: CodingStyleInfo{
			NamingConventions: []string{"follow file conventions"},
		},
		ErrorHandling: ErrorHandlingInfo{
			Pattern: "language-specific",
		},
		TestingPatterns: TestingInfo{
			Framework: "unknown",
		},
		ArchitecturalStyle: ArchitecturalInfo{
			LayerPattern: "unknown",
		},
		SecurityPatterns: SecurityInfo{
			AuthPattern: "unknown",
		},
	}
}
