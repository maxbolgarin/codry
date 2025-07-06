package reviewer

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// ContextGatherer provides enhanced context for code review
type ContextGatherer struct {
	provider interfaces.CodeProvider
	parser   *diffParser
	log      logze.Logger
}

// EnhancedContext contains rich context information for code analysis
type EnhancedContext struct {
	FilePath           string
	FileContent        string
	CleanDiff          string
	ImportedPackages   []string
	RelatedFiles       []RelatedFile
	FunctionSignatures []FunctionSignature
	TypeDefinitions    []TypeDefinition
	UsagePatterns      []UsagePattern
	SecurityContext    SecurityContext
	SemanticChanges    []ContextSemanticChange
}

// RelatedFile represents a file that has relationships with the target file
type RelatedFile struct {
	Path         string
	Relationship string // "imports", "imported_by", "same_package", "tests"
	Snippet      string // Relevant code snippet
}

// FunctionSignature represents a function definition
type FunctionSignature struct {
	Name       string
	Parameters []string
	Returns    []string
	IsExported bool
	LineNumber int
}

// TypeDefinition represents struct, interface, or type definitions
type TypeDefinition struct {
	Name       string
	Type       string // "struct", "interface", "type"
	Fields     []string
	Methods    []string
	IsExported bool
	LineNumber int
}

// UsagePattern represents how certain patterns are used in the codebase
type UsagePattern struct {
	Pattern      string
	Description  string
	Examples     []string
	BestPractice string
}

// SecurityContext provides security-related context
type SecurityContext struct {
	HasAuthenticationLogic  bool
	HasInputValidation      bool
	HandlesUserInput        bool
	AccessesDatabase        bool
	HandlesFileOperations   bool
	NetworkOperations       bool
	CryptographicOperations bool
}

// ContextSemanticChange represents a semantic change for context (avoids type conflicts)
type ContextSemanticChange struct {
	Type        string
	Impact      string
	Description string
	Lines       []int
	Context     string
}

// NewContextGatherer creates a new context gatherer
func NewContextGatherer(provider interfaces.CodeProvider) *ContextGatherer {
	return &ContextGatherer{
		provider: provider,
		parser:   newDiffParser(),
		log:      logze.With("component", "context-gatherer"),
	}
}

// GatherEnhancedContext collects comprehensive context for a file
func (cg *ContextGatherer) GatherEnhancedContext(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*EnhancedContext, error) {
	log := cg.log.WithFields("file", fileDiff.NewPath, "project", request.ProjectID)
	log.Debug("gathering enhanced context")

	enhancedCtx := &EnhancedContext{
		FilePath: fileDiff.NewPath,
	}

	// Get file content
	var err error
	if fileDiff.IsNew {
		enhancedCtx.FileContent = ""
	} else {
		enhancedCtx.FileContent, err = cg.getFileContent(ctx, request, fileDiff.NewPath)
		if err != nil {
			log.Warn("failed to get file content", "error", err)
		}
	}

	// Analyze imports and dependencies
	enhancedCtx.ImportedPackages = cg.extractImports(enhancedCtx.FileContent)

	// Find related files
	enhancedCtx.RelatedFiles, err = cg.findRelatedFiles(ctx, request, fileDiff, enhancedCtx.ImportedPackages)
	if err != nil {
		log.Warn("failed to find related files", "error", err)
	}

	// Extract function signatures
	enhancedCtx.FunctionSignatures = cg.extractFunctionSignatures(enhancedCtx.FileContent)

	// Extract type definitions
	enhancedCtx.TypeDefinitions = cg.extractTypeDefinitions(enhancedCtx.FileContent)

	// Analyze security context
	enhancedCtx.SecurityContext = cg.analyzeSecurityContext(enhancedCtx.FileContent, fileDiff.Diff)

	// Get usage patterns
	enhancedCtx.UsagePatterns = cg.getUsagePatterns(fileDiff.NewPath, enhancedCtx.FileContent)

	// Analyze semantic changes
	semanticChanges, err := cg.parser.AnalyzeSemanticChanges(fileDiff.Diff, enhancedCtx.FileContent)
	if err != nil {
		log.Warn("failed to analyze semantic changes", "error", err)
	} else {
		enhancedCtx.SemanticChanges = cg.convertSemanticChanges(semanticChanges)
	}

	return enhancedCtx, nil
}

// getFileContent retrieves file content with fallback strategies
func (cg *ContextGatherer) getFileContent(ctx context.Context, request model.ReviewRequest, filePath string) (string, error) {
	// Try target branch first
	if request.MergeRequest.TargetBranch != "" {
		content, err := cg.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.TargetBranch)
		if err == nil {
			return content, nil
		} else {
			cg.log.Debug("failed to get file content from target branch", "error", err, "file", filePath)
		}
	}

	// Fallback to source commit
	if request.MergeRequest.SHA != "" {
		content, err := cg.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.SHA)
		if err == nil {
			return content, nil
		} else {
			cg.log.Debug("failed to get file content from source commit", "error", err, "file", filePath)
		}
	}

	return "", errm.New("failed to get file content from any source")
}

// extractImports finds imported packages/modules
func (cg *ContextGatherer) extractImports(content string) []string {
	var imports []string

	// Go imports
	goImportRegex := regexp.MustCompile(`import\s+(?:\(\s*([^)]+)\s*\)|"([^"]+)")`)
	matches := goImportRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if match[1] != "" {
			// Multi-line import block
			lines := strings.Split(match[1], "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "//") {
					imports = append(imports, strings.Trim(line, `"`))
				}
			}
		} else if match[2] != "" {
			// Single import
			imports = append(imports, match[2])
		}
	}

	// JavaScript/TypeScript imports
	jsImportRegex := regexp.MustCompile(`import\s+(?:.*?from\s+)?['"]([^'"]+)['"]`)
	jsMatches := jsImportRegex.FindAllStringSubmatch(content, -1)
	for _, match := range jsMatches {
		if match[1] != "" {
			imports = append(imports, match[1])
		}
	}

	// Python imports
	pyImportRegex := regexp.MustCompile(`(?:from\s+([^\s]+)\s+)?import\s+([^\n]+)`)
	pyMatches := pyImportRegex.FindAllStringSubmatch(content, -1)
	for _, match := range pyMatches {
		if match[1] != "" {
			imports = append(imports, match[1])
		}
		if match[2] != "" {
			parts := strings.Split(match[2], ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					imports = append(imports, part)
				}
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, imp := range imports {
		if !seen[imp] {
			seen[imp] = true
			unique = append(unique, imp)
		}
	}

	sort.Strings(unique)
	return unique
}

// findRelatedFiles finds files that are related to the current file
func (cg *ContextGatherer) findRelatedFiles(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, imports []string) ([]RelatedFile, error) {
	var related []RelatedFile

	// Find test files
	testFile := cg.findTestFile(fileDiff.NewPath)
	if testFile != "" {
		content, err := cg.provider.GetFileContent(ctx, request.ProjectID, testFile, request.MergeRequest.TargetBranch)
		if err == nil {
			related = append(related, RelatedFile{
				Path:         testFile,
				Relationship: "tests",
				Snippet:      cg.extractRelevantSnippet(content, 200),
			})
		}
	}

	// Find files in the same package/directory
	dir := filepath.Dir(fileDiff.NewPath)
	packageFiles := cg.findPackageFiles(dir, fileDiff.NewPath)
	for _, file := range packageFiles {
		content, err := cg.provider.GetFileContent(ctx, request.ProjectID, file, request.MergeRequest.TargetBranch)
		if err == nil {
			related = append(related, RelatedFile{
				Path:         file,
				Relationship: "same_package",
				Snippet:      cg.extractRelevantSnippet(content, 150),
			})
		}
		if len(related) >= 5 { // Limit to avoid too much context
			break
		}
	}

	return related, nil
}

// extractFunctionSignatures extracts function signatures from code
func (cg *ContextGatherer) extractFunctionSignatures(content string) []FunctionSignature {
	var functions []FunctionSignature

	// Go functions
	goFuncRegex := regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s+)?([A-Z][a-zA-Z0-9_]*)\s*\(([^)]*)\)(?:\s*\(([^)]*)\)|\s+([^{]+))?\s*{`)
	matches := goFuncRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		funcSig := FunctionSignature{
			Name:       match[1],
			IsExported: strings.ToUpper(match[1][:1]) == match[1][:1],
		}

		if match[2] != "" {
			funcSig.Parameters = strings.Split(match[2], ",")
		}

		if match[3] != "" {
			funcSig.Returns = strings.Split(match[3], ",")
		} else if match[4] != "" {
			funcSig.Returns = []string{match[4]}
		}

		functions = append(functions, funcSig)
	}

	// JavaScript/TypeScript functions
	jsFuncRegex := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(([^)]*)\)`)
	jsMatches := jsFuncRegex.FindAllStringSubmatch(content, -1)
	for _, match := range jsMatches {
		funcSig := FunctionSignature{
			Name:       match[1],
			IsExported: strings.Contains(match[0], "export"),
		}

		if match[2] != "" {
			funcSig.Parameters = strings.Split(match[2], ",")
		}

		functions = append(functions, funcSig)
	}

	return functions
}

// extractTypeDefinitions extracts struct, interface, and type definitions
func (cg *ContextGatherer) extractTypeDefinitions(content string) []TypeDefinition {
	var types []TypeDefinition

	// Go structs
	structRegex := regexp.MustCompile(`(?ms)type\s+([A-Z][a-zA-Z0-9_]*)\s+struct\s*{([^}]*)}`)
	matches := structRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		typeDef := TypeDefinition{
			Name:       match[1],
			Type:       "struct",
			IsExported: strings.ToUpper(match[1][:1]) == match[1][:1],
		}

		// Extract fields
		fields := strings.Split(match[2], "\n")
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field != "" && !strings.HasPrefix(field, "//") {
				typeDef.Fields = append(typeDef.Fields, field)
			}
		}

		types = append(types, typeDef)
	}

	// Go interfaces
	interfaceRegex := regexp.MustCompile(`(?ms)type\s+([A-Z][a-zA-Z0-9_]*)\s+interface\s*{([^}]*)}`)
	interfaceMatches := interfaceRegex.FindAllStringSubmatch(content, -1)
	for _, match := range interfaceMatches {
		typeDef := TypeDefinition{
			Name:       match[1],
			Type:       "interface",
			IsExported: strings.ToUpper(match[1][:1]) == match[1][:1],
		}

		// Extract methods
		methods := strings.Split(match[2], "\n")
		for _, method := range methods {
			method = strings.TrimSpace(method)
			if method != "" && !strings.HasPrefix(method, "//") {
				typeDef.Methods = append(typeDef.Methods, method)
			}
		}

		types = append(types, typeDef)
	}

	return types
}

// analyzeSecurityContext analyzes security-relevant aspects of the code
func (cg *ContextGatherer) analyzeSecurityContext(content, diff string) SecurityContext {
	combinedText := content + "\n" + diff

	return SecurityContext{
		HasAuthenticationLogic:  cg.containsPattern(combinedText, []string{"auth", "login", "password", "token", "session", "jwt"}),
		HasInputValidation:      cg.containsPattern(combinedText, []string{"validate", "sanitize", "escape", "filter"}),
		HandlesUserInput:        cg.containsPattern(combinedText, []string{"input", "form", "request", "query", "param"}),
		AccessesDatabase:        cg.containsPattern(combinedText, []string{"sql", "database", "db", "query", "select", "insert", "update", "delete"}),
		HandlesFileOperations:   cg.containsPattern(combinedText, []string{"file", "read", "write", "upload", "download", "os."}),
		NetworkOperations:       cg.containsPattern(combinedText, []string{"http", "api", "fetch", "request", "client", "server"}),
		CryptographicOperations: cg.containsPattern(combinedText, []string{"crypto", "encrypt", "decrypt", "hash", "sign", "verify"}),
	}
}

// getUsagePatterns identifies common patterns and suggests best practices
func (cg *ContextGatherer) getUsagePatterns(filePath, content string) []UsagePattern {
	var patterns []UsagePattern

	// Error handling patterns
	if strings.Contains(content, "error") || strings.Contains(content, "err") {
		patterns = append(patterns, UsagePattern{
			Pattern:      "error_handling",
			Description:  "Error handling is present in this file",
			BestPractice: "Ensure all errors are properly wrapped with context and handled appropriately",
		})
	}

	// HTTP handling patterns
	if strings.Contains(content, "http") || strings.Contains(content, "Request") || strings.Contains(content, "Response") {
		patterns = append(patterns, UsagePattern{
			Pattern:      "http_handling",
			Description:  "HTTP request/response handling detected",
			BestPractice: "Validate all inputs, handle timeouts, and provide proper error responses",
		})
	}

	// Database patterns
	if cg.containsPattern(content, []string{"sql", "database", "query"}) {
		patterns = append(patterns, UsagePattern{
			Pattern:      "database_operations",
			Description:  "Database operations detected",
			BestPractice: "Use parameterized queries, handle transactions properly, and implement proper connection pooling",
		})
	}

	return patterns
}

// Helper functions

func (cg *ContextGatherer) findTestFile(filePath string) string {
	dir := filepath.Dir(filePath)
	filename := filepath.Base(filePath)
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	// Common test file patterns
	testPatterns := []string{
		fmt.Sprintf("%s_test%s", nameWithoutExt, ext),
		fmt.Sprintf("%s.test%s", nameWithoutExt, ext),
		fmt.Sprintf("test_%s%s", nameWithoutExt, ext),
	}

	for _, pattern := range testPatterns {
		testFile := filepath.Join(dir, pattern)
		return testFile // Return potential test file path (caller should check if it exists)
	}

	return ""
}

func (cg *ContextGatherer) findPackageFiles(dir, excludePath string) []string {
	// This would normally scan the directory, but since we don't have filesystem access,
	// we'll return common file patterns that might exist
	var files []string

	// Add common files that might be in the same package
	commonFiles := []string{
		"config.go", "types.go", "constants.go", "errors.go", "utils.go",
		"helpers.go", "models.go", "handlers.go", "service.go", "repository.go",
	}

	for _, file := range commonFiles {
		fullPath := filepath.Join(dir, file)
		if fullPath != excludePath {
			files = append(files, fullPath)
		}
	}

	return files
}

func (cg *ContextGatherer) extractRelevantSnippet(content string, maxLength int) string {
	lines := strings.Split(content, "\n")

	// Find the most relevant lines (exported functions, types, etc.)
	var relevantLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "func ") ||
			strings.HasPrefix(line, "type ") ||
			strings.HasPrefix(line, "const ") ||
			strings.HasPrefix(line, "var ") {
			relevantLines = append(relevantLines, line)
		}
	}

	snippet := strings.Join(relevantLines, "\n")
	if len(snippet) > maxLength {
		snippet = snippet[:maxLength] + "..."
	}

	return snippet
}

func (cg *ContextGatherer) containsPattern(text string, patterns []string) bool {
	lowerText := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(lowerText, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// convertSemanticChanges converts diff parser SemanticChange to context SemanticChange
func (cg *ContextGatherer) convertSemanticChanges(changes []SemanticChange) []ContextSemanticChange {
	var converted []ContextSemanticChange
	for _, change := range changes {
		// Create a simple struct that matches the prompts.SemanticChange
		simpleChange := ContextSemanticChange{
			Type:        string(change.Type),
			Impact:      string(change.Impact),
			Description: change.Description,
			Lines:       change.Lines,
			Context:     change.Context,
		}
		converted = append(converted, simpleChange)
	}
	return converted
}
