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
	"gopkg.in/yaml.v3"
)

// ProjectStyleAnalyzer analyzes project-specific patterns and conventions
type ProjectStyleAnalyzer struct {
	provider interfaces.CodeProvider
	log      logze.Logger
}

// NewProjectStyleAnalyzer creates a new project style analyzer
func NewProjectStyleAnalyzer(provider interfaces.CodeProvider) *ProjectStyleAnalyzer {
	return &ProjectStyleAnalyzer{
		provider: provider,
		log:      logze.With("component", "project-style-analyzer"),
	}
}

// ProjectStyleInfo contains comprehensive project style information
type ProjectStyleInfo struct {
	LinterConfig       LinterConfig       `json:"linter_config"`
	Dependencies       DependencyInfo     `json:"dependencies"`
	CodingConventions  CodingConventions  `json:"coding_conventions"`
	ArchitecturalStyle ArchitecturalStyle `json:"architectural_style"`
	TestingConventions TestingConventions `json:"testing_conventions"`
	ErrorHandling      ErrorHandlingStyle `json:"error_handling"`
	SecurityPatterns   SecurityPatterns   `json:"security_patterns"`
	PerformanceHints   PerformanceHints   `json:"performance_hints"`
}

// LinterConfig represents linter configuration and rules
type LinterConfig struct {
	Tool            string                `json:"tool"`             // golangci-lint, staticcheck, etc.
	EnabledLinters  []string              `json:"enabled_linters"`  // list of enabled linters
	DisabledLinters []string              `json:"disabled_linters"` // list of disabled linters
	Rules           map[string]LinterRule `json:"rules"`            // specific rule configurations
	Complexity      ComplexityLimits      `json:"complexity"`       // complexity limits
}

type LinterRule struct {
	Enabled  bool                   `json:"enabled"`
	Severity string                 `json:"severity"`
	Config   map[string]interface{} `json:"config"`
}

type ComplexityLimits struct {
	Cyclomatic int `json:"cyclomatic"`  // cyclomatic complexity limit
	Cognitive  int `json:"cognitive"`   // cognitive complexity limit
	FuncLength int `json:"func_length"` // function length limit
	FuncParams int `json:"func_params"` // function parameter limit
}

// DependencyInfo represents project dependencies and their implications
type DependencyInfo struct {
	GoVersion       string                 `json:"go_version"`
	Dependencies    []Dependency           `json:"dependencies"`
	TestDeps        []Dependency           `json:"test_dependencies"`
	CommonLibraries map[string]LibraryInfo `json:"common_libraries"`
}

type LibraryInfo struct {
	Name     string   `json:"name"`
	Purpose  string   `json:"purpose"`  // logging, testing, http, etc.
	Patterns []string `json:"patterns"` // common usage patterns
}

// CodingConventions represents coding style conventions
type CodingConventions struct {
	NamingStyle     NamingStyle     `json:"naming_style"`
	StructureStyle  StructureStyle  `json:"structure_style"`
	CommentingStyle CommentingStyle `json:"commenting_style"`
	ImportStyle     ImportStyle     `json:"import_style"`
	ErrorStyle      ErrorStyle      `json:"error_style"`
	InterfaceStyle  InterfaceStyle  `json:"interface_style"`
}

type NamingStyle struct {
	FunctionNaming  string   `json:"function_naming"`  // camelCase, PascalCase, etc.
	VariableNaming  string   `json:"variable_naming"`  // camelCase, snake_case, etc.
	ConstantNaming  string   `json:"constant_naming"`  // UPPER_CASE, PascalCase, etc.
	TypeNaming      string   `json:"type_naming"`      // PascalCase, etc.
	InterfaceNaming string   `json:"interface_naming"` // -er suffix, I- prefix, etc.
	Abbreviations   []string `json:"abbreviations"`    // common abbreviations used
	ForbiddenNames  []string `json:"forbidden_names"`  // names to avoid
}

type StructureStyle struct {
	PackageStructure string `json:"package_structure"`  // flat, domain-driven, layered, etc.
	FileNaming       string `json:"file_naming"`        // snake_case, camelCase, etc.
	DirectoryNaming  string `json:"directory_naming"`   // snake_case, camelCase, etc.
	StructFieldOrder string `json:"struct_field_order"` // alphabetical, by type, by importance
	FunctionGrouping string `json:"function_grouping"`  // by type, alphabetical, by access
	ConstantGrouping string `json:"constant_grouping"`  // by type, by usage, etc.
}

type CommentingStyle struct {
	DocCommentStyle string `json:"doc_comment_style"` // godoc, standard, etc.
	InlineComments  string `json:"inline_comments"`   // encouraged, discouraged, etc.
	TODOStyle       string `json:"todo_style"`        // TODO:, FIXME:, etc.
	CommentLength   int    `json:"comment_length"`    // maximum comment line length
}

type ImportStyle struct {
	GroupingStyle    string   `json:"grouping_style"`    // stdlib/external/internal, etc.
	AliasConventions []string `json:"alias_conventions"` // common import aliases
	ForbiddenImports []string `json:"forbidden_imports"` // imports to avoid
	PreferredImports []string `json:"preferred_imports"` // preferred alternatives
}

type ErrorStyle struct {
	ErrorWrapping string `json:"error_wrapping"` // fmt.Errorf, errors.Wrap, etc.
	ErrorCreation string `json:"error_creation"` // errors.New, fmt.Errorf, etc.
	ErrorChecking string `json:"error_checking"` // explicit, early return, etc.
	ErrorLogging  string `json:"error_logging"`  // structured, simple, etc.
}

type InterfaceStyle struct {
	InterfaceSize  string `json:"interface_size"`  // small, single-method preferred
	MethodNaming   string `json:"method_naming"`   // verb-based, noun-based, etc.
	ParameterStyle string `json:"parameter_style"` // context-first, etc.
	ReturnStyle    string `json:"return_style"`    // named returns, error-last, etc.
}

// TestingConventions represents testing patterns and conventions
type TestingConventions struct {
	TestFramework   string            `json:"test_framework"`   // testify, ginkgo, standard, etc.
	TestFileNaming  string            `json:"test_file_naming"` // _test.go suffix pattern
	TestFuncNaming  string            `json:"test_func_naming"` // Test*, Benchmark*, Example*
	MockingStrategy string            `json:"mocking_strategy"` // interfaces, testify/mock, etc.
	AssertionStyle  string            `json:"assertion_style"`  // testify/assert, require, etc.
	TestStructure   TestStructureInfo `json:"test_structure"`   // how tests are organized
	CoverageTargets CoverageInfo      `json:"coverage_targets"` // coverage expectations
}

type TestStructureInfo struct {
	TestTablePattern string `json:"test_table_pattern"` // table-driven, individual, etc.
	SetupTeardown    string `json:"setup_teardown"`     // function-level, package-level, etc.
	TestHelpers      string `json:"test_helpers"`       // separate file, inline, etc.
	TestData         string `json:"test_data"`          // testdata/, fixtures/, inline
}

type CoverageInfo struct {
	MinCoverage    float64  `json:"min_coverage"`     // minimum coverage percentage
	CoverageByType string   `json:"coverage_by_type"` // line, branch, statement
	ExclusionRules []string `json:"exclusion_rules"`  // patterns to exclude from coverage
}

// SecurityPatterns represents security-related patterns and practices
type SecurityPatterns struct {
	AuthenticationPattern string               `json:"authentication_pattern"` // JWT, session, basic, etc.
	AuthorizationPattern  string               `json:"authorization_pattern"`  // RBAC, ABAC, etc.
	InputValidation       InputValidationInfo  `json:"input_validation"`       // validation strategies
	CryptographyUsage     CryptographyInfo     `json:"cryptography_usage"`     // crypto patterns
	SecretManagement      SecretManagementInfo `json:"secret_management"`      // how secrets are handled
	SecurityHeaders       []string             `json:"security_headers"`       // required security headers
}

type InputValidationInfo struct {
	ValidationLibrary string   `json:"validation_library"` // validator, ozzo-validation, etc.
	ValidationStyle   string   `json:"validation_style"`   // struct tags, function-based, etc.
	SanitizationRules []string `json:"sanitization_rules"` // XSS prevention, SQL injection, etc.
}

type CryptographyInfo struct {
	HashingAlgorithms   []string `json:"hashing_algorithms"`   // bcrypt, argon2, etc.
	EncryptionStandards []string `json:"encryption_standards"` // AES, RSA, etc.
	KeyManagement       string   `json:"key_management"`       // how keys are managed
	RandomnessSource    string   `json:"randomness_source"`    // crypto/rand, etc.
}

type SecretManagementInfo struct {
	SecretStorage     string   `json:"secret_storage"`     // env vars, vault, etc.
	SecretRotation    string   `json:"secret_rotation"`    // manual, automatic, etc.
	SecretValidation  string   `json:"secret_validation"`  // at startup, runtime, etc.
	ForbiddenPatterns []string `json:"forbidden_patterns"` // hardcoded secrets patterns
}

// PerformanceHints represents performance-related patterns and practices
type PerformanceHints struct {
	ConcurrencyPatterns []string           `json:"concurrency_patterns"` // worker pools, pipelines, etc.
	CachingStrategy     CachingInfo        `json:"caching_strategy"`     // caching approach
	DatabasePatterns    DatabaseInfo       `json:"database_patterns"`    // DB access patterns
	MemoryManagement    MemoryManagement   `json:"memory_management"`    // memory usage patterns
	OptimizationHints   []OptimizationHint `json:"optimization_hints"`   // performance tips
}

// ArchitecturalStyle represents architectural patterns and conventions
type ArchitecturalStyle struct {
	LayerPattern     string   `json:"layer_pattern"`     // layered, hexagonal, clean, etc.
	DIPattern        string   `json:"di_pattern"`        // dependency injection approach
	ConfigPattern    string   `json:"config_pattern"`    // configuration management
	DesignPatterns   []string `json:"design_patterns"`   // common design patterns used
	ModularityStyle  string   `json:"modularity_style"`  // how modules are organized
	APIDesignStyle   string   `json:"api_design_style"`  // REST, GraphQL, RPC, etc.
	ErrorPropagation string   `json:"error_propagation"` // how errors are propagated
}

// ErrorHandlingStyle represents error handling patterns and conventions
type ErrorHandlingStyle struct {
	Pattern       string   `json:"pattern"`        // error handling pattern (wrap, bubble, etc.)
	WrapStyle     string   `json:"wrap_style"`     // how errors are wrapped
	LoggingStyle  string   `json:"logging_style"`  // how errors are logged
	RecoveryStyle string   `json:"recovery_style"` // how to recover from errors
	Libraries     []string `json:"libraries"`      // error handling libraries used
	Conventions   []string `json:"conventions"`    // error handling conventions
}

// CachingInfo represents caching patterns and strategies
type CachingInfo struct {
	CachingLibrary    string   `json:"caching_library"`    // redis, memcached, in-memory, etc.
	CachingStrategy   string   `json:"caching_strategy"`   // write-through, write-back, etc.
	TTLStrategy       string   `json:"ttl_strategy"`       // fixed, sliding, etc.
	InvalidationRules []string `json:"invalidation_rules"` // cache invalidation patterns
}

// DatabaseInfo represents database access patterns and strategies
type DatabaseInfo struct {
	ORM               string   `json:"orm"`                // gorm, sqlx, raw SQL, etc.
	ConnectionPool    string   `json:"connection_pool"`    // pgxpool, sql.DB, etc.
	TransactionStyle  string   `json:"transaction_style"`  // explicit, implicit, etc.
	MigrationStrategy string   `json:"migration_strategy"` // migrate, goose, etc.
	QueryPatterns     []string `json:"query_patterns"`     // common query patterns
}

// MemoryManagement represents memory usage patterns and strategies
type MemoryManagement struct {
	PoolingStrategy  string   `json:"pooling_strategy"`   // sync.Pool, custom pools, etc.
	GCOptimizations  []string `json:"gc_optimizations"`   // GOGC settings, etc.
	MemoryLeakChecks []string `json:"memory_leak_checks"` // pprof usage, etc.
}

type OptimizationHint struct {
	Area        string `json:"area"`        // CPU, memory, I/O, etc.
	Pattern     string `json:"pattern"`     // the optimization pattern
	Description string `json:"description"` // when and how to apply
}

// AnalyzeProjectStyle performs comprehensive project style analysis
func (psa *ProjectStyleAnalyzer) AnalyzeProjectStyle(ctx context.Context, request model.ReviewRequest, filePath string) (*ProjectStyleInfo, error) {
	log := psa.log.WithFields("project", request.ProjectID, "file", filePath)
	log.Debug("starting project style analysis")

	style := &ProjectStyleInfo{}

	// Analyze linter configuration
	linterConfig, err := psa.analyzeLinterConfig(ctx, request)
	if err != nil {
		log.Warn("failed to analyze linter config", "error", err)
	} else {
		style.LinterConfig = linterConfig
	}

	// Analyze dependencies
	dependencies, err := psa.analyzeDependencies(ctx, request)
	if err != nil {
		log.Warn("failed to analyze dependencies", "error", err)
	} else {
		style.Dependencies = dependencies
	}

	// Analyze coding conventions from neighboring files
	conventions, err := psa.analyzeCodingConventions(ctx, request, filePath)
	if err != nil {
		log.Warn("failed to analyze coding conventions", "error", err)
	} else {
		style.CodingConventions = conventions
	}

	// Analyze architectural style
	archStyle, err := psa.analyzeArchitecturalStyle(ctx, request, filePath)
	if err != nil {
		log.Warn("failed to analyze architectural style", "error", err)
	} else {
		style.ArchitecturalStyle = archStyle
	}

	// Analyze testing conventions
	testConventions, err := psa.analyzeTestingConventions(ctx, request)
	if err != nil {
		log.Warn("failed to analyze testing conventions", "error", err)
	} else {
		style.TestingConventions = testConventions
	}

	// Analyze error handling patterns
	errorHandling, err := psa.analyzeErrorHandling(ctx, request, filePath)
	if err != nil {
		log.Warn("failed to analyze error handling", "error", err)
	} else {
		style.ErrorHandling = errorHandling
	}

	// Analyze security patterns
	securityPatterns, err := psa.analyzeSecurityPatterns(ctx, request)
	if err != nil {
		log.Warn("failed to analyze security patterns", "error", err)
	} else {
		style.SecurityPatterns = securityPatterns
	}

	// Analyze performance hints
	performanceHints, err := psa.analyzePerformanceHints(ctx, request)
	if err != nil {
		log.Warn("failed to analyze performance hints", "error", err)
	} else {
		style.PerformanceHints = performanceHints
	}

	log.Debug("project style analysis completed")
	return style, nil
}

// analyzeLinterConfig analyzes the project's linter configuration
func (psa *ProjectStyleAnalyzer) analyzeLinterConfig(ctx context.Context, request model.ReviewRequest) (LinterConfig, error) {
	config := LinterConfig{}

	// Try to get .golangci.yml or .golangci.yaml
	content, err := psa.getLinterConfigContent(ctx, request)
	if err != nil {
		return config, fmt.Errorf("failed to get linter config: %w", err)
	}

	// Parse the configuration
	var golangciConfig struct {
		Linters struct {
			Enable  []string `yaml:"enable"`
			Disable []string `yaml:"disable"`
		} `yaml:"linters"`
		LintersSettings struct {
			Cyclop struct {
				MaxComplexity int `yaml:"max-complexity"`
			} `yaml:"cyclop"`
			Funlen struct {
				Lines      int `yaml:"lines"`
				Statements int `yaml:"statements"`
			} `yaml:"funlen"`
			Gocognit struct {
				MinComplexity int `yaml:"min-complexity"`
			} `yaml:"gocognit"`
			Revive struct {
				Rules []struct {
					Name      string `yaml:"name"`
					Arguments []int  `yaml:"arguments"`
				} `yaml:"rules"`
			} `yaml:"revive"`
		} `yaml:"linters-settings"`
	}

	err = yaml.Unmarshal([]byte(content), &golangciConfig)
	if err != nil {
		return config, fmt.Errorf("failed to parse linter config: %w", err)
	}

	config.Tool = "golangci-lint"
	config.EnabledLinters = golangciConfig.Linters.Enable
	config.DisabledLinters = golangciConfig.Linters.Disable

	// Extract complexity limits
	config.Complexity = ComplexityLimits{
		Cyclomatic: golangciConfig.LintersSettings.Cyclop.MaxComplexity,
		Cognitive:  golangciConfig.LintersSettings.Gocognit.MinComplexity,
		FuncLength: golangciConfig.LintersSettings.Funlen.Lines,
	}

	// Extract function parameter limits from revive rules
	for _, rule := range golangciConfig.LintersSettings.Revive.Rules {
		if rule.Name == "argument-limit" && len(rule.Arguments) > 0 {
			config.Complexity.FuncParams = rule.Arguments[0]
		}
	}

	return config, nil
}

// getLinterConfigContent tries to get linter configuration content
func (psa *ProjectStyleAnalyzer) getLinterConfigContent(ctx context.Context, request model.ReviewRequest) (string, error) {
	configFiles := []string{".golangci.yml", ".golangci.yaml", ".golangci-lint.yml", ".golangci-lint.yaml"}

	for _, configFile := range configFiles {
		content, err := psa.provider.GetFileContent(ctx, request.ProjectID, configFile, request.MergeRequest.TargetBranch)
		if err == nil {
			return content, nil
		}
	}

	return "", fmt.Errorf("no linter config found")
}

// analyzeDependencies analyzes go.mod and extracts dependency information
func (psa *ProjectStyleAnalyzer) analyzeDependencies(ctx context.Context, request model.ReviewRequest) (DependencyInfo, error) {
	deps := DependencyInfo{
		CommonLibraries: make(map[string]LibraryInfo),
	}

	// Get go.mod content
	content, err := psa.provider.GetFileContent(ctx, request.ProjectID, "go.mod", request.MergeRequest.TargetBranch)
	if err != nil {
		return deps, fmt.Errorf("failed to get go.mod: %w", err)
	}

	// Parse go.mod for dependencies
	lines := strings.Split(content, "\n")
	inRequireBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Extract Go version
		if strings.HasPrefix(line, "go ") {
			deps.GoVersion = strings.TrimPrefix(line, "go ")
			continue
		}

		// Handle require block
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		// Parse dependency lines
		if strings.HasPrefix(line, "require ") || inRequireBlock {
			dep := psa.parseDependencyLine(line)
			if dep.Name != "" {
				deps.Dependencies = append(deps.Dependencies, dep)

				// Identify common libraries and their purposes
				if libraryInfo := psa.identifyLibraryType(dep.Name); libraryInfo.Purpose != "" {
					deps.CommonLibraries[dep.Name] = libraryInfo
				}
			}
		}
	}

	return deps, nil
}

// parseDependencyLine parses a single dependency line from go.mod
func (psa *ProjectStyleAnalyzer) parseDependencyLine(line string) Dependency {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "require ")

	parts := strings.Fields(line)
	if len(parts) >= 2 {
		return Dependency{
			Name:    parts[0],
			Package: parts[0],
		}
	}

	return Dependency{}
}

// identifyLibraryType identifies the type and purpose of a library
func (psa *ProjectStyleAnalyzer) identifyLibraryType(name string) LibraryInfo {
	nameLower := strings.ToLower(name)

	// Common library patterns
	if strings.Contains(nameLower, "logz") || strings.Contains(nameLower, "logrus") || strings.Contains(nameLower, "zap") {
		return LibraryInfo{
			Name:     name,
			Purpose:  "logging",
			Patterns: []string{"structured_logging", "leveled_logging"},
		}
	}

	if strings.Contains(nameLower, "testify") || strings.Contains(nameLower, "ginkgo") || strings.Contains(nameLower, "gomega") {
		return LibraryInfo{
			Name:     name,
			Purpose:  "testing",
			Patterns: []string{"assertion_based", "behavior_driven"},
		}
	}

	if strings.Contains(nameLower, "gin") || strings.Contains(nameLower, "echo") || strings.Contains(nameLower, "fiber") || strings.Contains(nameLower, "mux") {
		return LibraryInfo{
			Name:     name,
			Purpose:  "http_framework",
			Patterns: []string{"middleware_based", "route_based"},
		}
	}

	if strings.Contains(nameLower, "gorm") || strings.Contains(nameLower, "sqlx") || strings.Contains(nameLower, "ent") {
		return LibraryInfo{
			Name:     name,
			Purpose:  "database_orm",
			Patterns: []string{"active_record", "data_mapper"},
		}
	}

	if strings.Contains(nameLower, "redis") || strings.Contains(nameLower, "memcache") {
		return LibraryInfo{
			Name:     name,
			Purpose:  "caching",
			Patterns: []string{"key_value_store", "ttl_based"},
		}
	}

	return LibraryInfo{}
}

// analyzeCodingConventions analyzes coding conventions from neighboring files
func (psa *ProjectStyleAnalyzer) analyzeCodingConventions(ctx context.Context, request model.ReviewRequest, filePath string) (CodingConventions, error) {
	conventions := CodingConventions{}

	// Get files from the same package
	packageDir := filepath.Dir(filePath)
	packageFiles, err := psa.getPackageFiles(ctx, request, packageDir)
	if err != nil {
		return conventions, fmt.Errorf("failed to get package files: %w", err)
	}

	// Analyze naming conventions
	conventions.NamingStyle = psa.analyzeNamingStyle(packageFiles)

	// Analyze structure conventions
	conventions.StructureStyle = psa.analyzeStructureStyle(packageFiles)

	// Analyze commenting style
	conventions.CommentingStyle = psa.analyzeCommentingStyle(packageFiles)

	// Analyze import style
	conventions.ImportStyle = psa.analyzeImportStyle(packageFiles)

	// Analyze error style
	conventions.ErrorStyle = psa.analyzeErrorStyle(packageFiles)

	// Analyze interface style
	conventions.InterfaceStyle = psa.analyzeInterfaceStyle(packageFiles)

	return conventions, nil
}

// getPackageFiles gets content of files in the same package
func (psa *ProjectStyleAnalyzer) getPackageFiles(ctx context.Context, request model.ReviewRequest, packageDir string) (map[string]string, error) {
	// This is a simplified implementation - in practice, we'd want to:
	// 1. List directory contents
	// 2. Filter for .go files
	// 3. Get content for each file

	files := make(map[string]string)

	// Common Go files that might exist in the package
	commonFiles := []string{
		"config.go", "types.go", "constants.go", "errors.go", "utils.go",
		"helpers.go", "models.go", "handlers.go", "service.go", "repository.go",
		"client.go", "server.go", "main.go", "app.go",
	}

	for _, filename := range commonFiles {
		fullPath := filepath.Join(packageDir, filename)
		content, err := psa.provider.GetFileContent(ctx, request.ProjectID, fullPath, request.MergeRequest.TargetBranch)
		if err == nil {
			files[filename] = content
		}
	}

	return files, nil
}

// analyzeNamingStyle analyzes naming conventions from package files
func (psa *ProjectStyleAnalyzer) analyzeNamingStyle(packageFiles map[string]string) NamingStyle {
	style := NamingStyle{
		FunctionNaming:  "camelCase",
		VariableNaming:  "camelCase",
		ConstantNaming:  "PascalCase",
		TypeNaming:      "PascalCase",
		InterfaceNaming: "er_suffix",
	}

	// Analyze function names
	functionRegex := regexp.MustCompile(`func\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	typeRegex := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s+`)
	constRegex := regexp.MustCompile(`const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`)

	for _, content := range packageFiles {
		// Analyze function naming patterns
		funcMatches := functionRegex.FindAllStringSubmatch(content, -1)
		for _, match := range funcMatches {
			if len(match) >= 2 {
				name := match[1]
				if strings.Contains(name, "_") {
					style.FunctionNaming = "snake_case"
				}
			}
		}

		// Analyze type naming patterns
		typeMatches := typeRegex.FindAllStringSubmatch(content, -1)
		for _, match := range typeMatches {
			if len(match) >= 2 {
				name := match[1]
				if strings.HasSuffix(name, "er") || strings.HasSuffix(name, "or") {
					style.InterfaceNaming = "er_suffix"
				}
			}
		}

		// Analyze constant naming patterns
		constMatches := constRegex.FindAllStringSubmatch(content, -1)
		for _, match := range constMatches {
			if len(match) >= 2 {
				name := match[1]
				if strings.ToUpper(name) == name {
					style.ConstantNaming = "UPPER_CASE"
				}
			}
		}
	}

	return style
}

// analyzeStructureStyle analyzes code structure conventions
func (psa *ProjectStyleAnalyzer) analyzeStructureStyle(packageFiles map[string]string) StructureStyle {
	return StructureStyle{
		PackageStructure: "domain-driven",
		FileNaming:       "snake_case",
		DirectoryNaming:  "snake_case",
		StructFieldOrder: "by_type",
		FunctionGrouping: "by_type",
		ConstantGrouping: "by_usage",
	}
}

// analyzeCommentingStyle analyzes commenting conventions
func (psa *ProjectStyleAnalyzer) analyzeCommentingStyle(packageFiles map[string]string) CommentingStyle {
	style := CommentingStyle{
		DocCommentStyle: "godoc",
		InlineComments:  "encouraged",
		TODOStyle:       "TODO:",
		CommentLength:   80,
	}

	// Look for documentation comment patterns
	for _, content := range packageFiles {
		if strings.Contains(content, "// TODO:") {
			style.TODOStyle = "TODO:"
		} else if strings.Contains(content, "// FIXME:") {
			style.TODOStyle = "FIXME:"
		}
	}

	return style
}

// analyzeImportStyle analyzes import conventions
func (psa *ProjectStyleAnalyzer) analyzeImportStyle(packageFiles map[string]string) ImportStyle {
	style := ImportStyle{
		GroupingStyle:    "stdlib_external_internal",
		AliasConventions: []string{},
	}

	// Analyze import patterns
	importRegex := regexp.MustCompile(`import\s+(?:([a-zA-Z_][a-zA-Z0-9_]*)\s+)?"([^"]+)"`)

	for _, content := range packageFiles {
		matches := importRegex.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 3 && match[1] != "" {
				// Found an aliased import
				alias := match[1]
				importPath := match[2]
				style.AliasConventions = append(style.AliasConventions, fmt.Sprintf("%s -> %s", alias, importPath))
			}
		}
	}

	return style
}

// analyzeErrorStyle analyzes error handling conventions
func (psa *ProjectStyleAnalyzer) analyzeErrorStyle(packageFiles map[string]string) ErrorStyle {
	style := ErrorStyle{
		ErrorWrapping: "fmt.Errorf",
		ErrorCreation: "errors.New",
		ErrorChecking: "explicit",
		ErrorLogging:  "structured",
	}

	// Analyze error patterns
	for _, content := range packageFiles {
		if strings.Contains(content, "errors.Wrap") {
			style.ErrorWrapping = "pkg/errors"
		} else if strings.Contains(content, "fmt.Errorf") {
			style.ErrorWrapping = "fmt.Errorf"
		}

		if strings.Contains(content, "logze") || strings.Contains(content, "logrus") || strings.Contains(content, "zap") {
			style.ErrorLogging = "structured"
		}
	}

	return style
}

// analyzeInterfaceStyle analyzes interface conventions
func (psa *ProjectStyleAnalyzer) analyzeInterfaceStyle(packageFiles map[string]string) InterfaceStyle {
	return InterfaceStyle{
		InterfaceSize:  "small",
		MethodNaming:   "verb_based",
		ParameterStyle: "context_first",
		ReturnStyle:    "error_last",
	}
}

// analyzeArchitecturalStyle analyzes architectural patterns
func (psa *ProjectStyleAnalyzer) analyzeArchitecturalStyle(ctx context.Context, request model.ReviewRequest, filePath string) (ArchitecturalStyle, error) {
	// This is a simplified implementation that would be expanded
	return ArchitecturalStyle{}, nil
}

// analyzeTestingConventions analyzes testing patterns
func (psa *ProjectStyleAnalyzer) analyzeTestingConventions(ctx context.Context, request model.ReviewRequest) (TestingConventions, error) {
	conventions := TestingConventions{
		TestFramework:   "standard",
		TestFileNaming:  "_test.go",
		TestFuncNaming:  "Test*",
		MockingStrategy: "interfaces",
		AssertionStyle:  "standard",
	}

	// Try to find test files and analyze patterns
	testFiles := []string{"example_test.go", "main_test.go", "config_test.go"}

	for _, testFile := range testFiles {
		content, err := psa.provider.GetFileContent(ctx, request.ProjectID, testFile, request.MergeRequest.TargetBranch)
		if err == nil {
			if strings.Contains(content, "testify") {
				conventions.TestFramework = "testify"
				conventions.AssertionStyle = "testify"
			}
			if strings.Contains(content, "ginkgo") {
				conventions.TestFramework = "ginkgo"
			}
		}
	}

	return conventions, nil
}

// analyzeErrorHandling analyzes error handling patterns
func (psa *ProjectStyleAnalyzer) analyzeErrorHandling(ctx context.Context, request model.ReviewRequest, filePath string) (ErrorHandlingStyle, error) {
	// This is a simplified implementation that would be expanded
	return ErrorHandlingStyle{}, nil
}

// analyzeSecurityPatterns analyzes security-related patterns
func (psa *ProjectStyleAnalyzer) analyzeSecurityPatterns(ctx context.Context, request model.ReviewRequest) (SecurityPatterns, error) {
	patterns := SecurityPatterns{
		AuthenticationPattern: "jwt",
		AuthorizationPattern:  "rbac",
	}

	// This would analyze security patterns from the codebase
	// For now, return defaults
	return patterns, nil
}

// analyzePerformanceHints analyzes performance-related patterns
func (psa *ProjectStyleAnalyzer) analyzePerformanceHints(ctx context.Context, request model.ReviewRequest) (PerformanceHints, error) {
	hints := PerformanceHints{
		ConcurrencyPatterns: []string{"worker_pools", "channels"},
	}

	// This would analyze performance patterns from the codebase
	// For now, return defaults
	return hints, nil
}
