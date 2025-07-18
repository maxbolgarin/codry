package astparser

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	sitter "github.com/smacker/go-tree-sitter"
)

// ExternalRefsAnalyzer analyzes symbols and their relationships across the codebase
type ExternalRefsAnalyzer struct {
	astParser *Parser
	provider  interfaces.CodeProvider
	log       logze.Logger
}

// CallerInfo represents information about where a symbol is called
type CallerInfo struct {
	FilePath     string `json:"file_path"`
	LineNumber   int    `json:"line_number"`
	FunctionName string `json:"function_name"`
	Code         string `json:"code"`
	CodeSnippet  string `json:"code_snippet"`
}

// DependencyInfo represents information about a dependency of a symbol
type DependencyInfo struct {
	SymbolName      string `json:"symbol_name"`
	SourceFile      string `json:"source_file"`
	ImportStatement string `json:"import_statement,omitempty"`
	Documentation   string `json:"documentation,omitempty"`
	Code            string `json:"code"`
}

// SymbolUsageContext provides comprehensive context about symbol usage
type SymbolUsageContext struct {
	Callers      []CallerInfo     `json:"callers"`
	Dependencies []DependencyInfo `json:"dependencies"`
	RelatedFiles []string         `json:"related_files"`
	ImportedBy   []string         `json:"imported_by"`
	ExportsTo    []string         `json:"exports_to"`
}

// NewSymbolAnalyzer creates a new symbol analyzer
func NewExternalRefsAnalyzer(provider interfaces.CodeProvider) *ExternalRefsAnalyzer {
	return &ExternalRefsAnalyzer{
		astParser: NewParser(),
		provider:  provider,
		log:       logze.With("component", "external_refs_analyzer"),
	}
}

// AnalyzeSymbolUsage analyzes the usage context of an affected symbol
func (sa *ExternalRefsAnalyzer) AnalyzeSymbolUsage(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) (SymbolUsageContext, error) {
	usage := SymbolUsageContext{}

	// Find callers of this symbol
	callers, err := sa.FindSymbolCallers(ctx, data, symbol)
	if err != nil {
		sa.log.Warn("failed to find symbol callers", "error", err, "symbol", symbol.Name)
	} else {
		usage.Callers = callers
	}

	// Analyze dependencies of this symbol
	dependencies, err := sa.AnalyzeDependencies(ctx, data, symbol)
	if err != nil {
		sa.log.Warn("failed to analyze dependencies", "error", err, "symbol", symbol.Name)
	} else {
		usage.Dependencies = dependencies
	}

	// Find related files (files in the same directory)
	relatedFiles, err := sa.findRelatedFiles(ctx, data, symbol.FilePath)
	if err != nil {
		sa.log.Warn("failed to find related files", "error", err, "symbol", symbol.Name)
	} else {
		usage.RelatedFiles = relatedFiles
	}

	return usage, nil
}

// findSymbolCallers finds all places where a symbol is called/used
func (sa *ExternalRefsAnalyzer) FindSymbolCallers(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) ([]CallerInfo, error) {
	var callers []CallerInfo

	if symbol.Name == "" {
		return callers, nil
	}

	// Analyze each potential file with AST parsing for accurate results
	for _, file := range getAffectedFiles(data, symbol) {
		fileCallers, err := sa.analyzeFileForCallers(ctx, file.Content, file.Path, symbol)
		if err != nil {
			//sa.log.Warn("failed to analyze file for callers", "error", err, "file", file.Path)
			continue
		}
		callers = append(callers, fileCallers...)
	}

	return callers, nil
}

func getAffectedFiles(data *model.RepositorySnapshot, symbol AffectedSymbol) []*model.RepositoryFile {
	var files []*model.RepositoryFile
	for _, file := range data.Files {
		if strings.Contains(file.Content, symbol.Name) {
			files = append(files, file)
		}
	}
	return files
}

// analyzeFileForCallers analyzes a specific file to find callers of a symbol
func (sa *ExternalRefsAnalyzer) analyzeFileForCallers(ctx context.Context, content string, filePath string, symbol AffectedSymbol) ([]CallerInfo, error) {
	if symbol.Name == "" {
		return nil, nil
	}

	rootNode, err := sa.astParser.GetFileAST(ctx, filePath, content)
	if err != nil {
		return nil, errm.Wrap(err, "failed to parse file to AST")
	}

	var callers []CallerInfo
	sa.walkASTForCallers(rootNode, content, filePath, symbol, &callers)

	return callers, nil
}

// extractCodeSnippet extracts a code snippet around a specific line
func (sa *ExternalRefsAnalyzer) extractCodeSnippet(lines []string, centerLine, contextLines int) string {
	start := centerLine - contextLines
	if start < 0 {
		start = 0
	}

	end := centerLine + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	snippet := strings.Join(lines[start:end], "\n")
	return strings.TrimSpace(snippet)
}

// walkASTForCallers walks the AST to find function calls
func (sa *ExternalRefsAnalyzer) walkASTForCallers(node *sitter.Node, content, filePath string, symbol AffectedSymbol, callers *[]CallerInfo) {
	nodeType := node.Type()

	if strings.Contains(nodeType, "call") {
		call := sa.astParser.extractDependency(node, content)
		if call.Name == symbol.Name || strings.HasSuffix(call.Name, "."+symbol.Name) {
			caller := CallerInfo{
				FilePath:    filePath,
				LineNumber:  call.Line,
				Code:        sa.astParser.getNodeText(node, content),
				CodeSnippet: sa.getCallSiteSnippet(node, content),
			}

			// Find the containing function
			containingFunc := sa.findContainingFunctionNode(node, content, filePath)
			if containingFunc != "" {
				caller.FunctionName = containingFunc
			}

			*callers = append(*callers, caller)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			sa.walkASTForCallers(child, content, filePath, symbol, callers)
		}
	}
}

// getCallSiteSnippet gets a code snippet around a function call site
func (sa *ExternalRefsAnalyzer) getCallSiteSnippet(node *sitter.Node, content string) string {
	// Get the line of the call
	line := int(node.StartPoint().Row)
	lines := strings.Split(content, "\n")

	return sa.extractCodeSnippet(lines, line, 2)
}

// findContainingFunctionNode finds the name of the function containing a node
func (sa *ExternalRefsAnalyzer) findContainingFunctionNode(node *sitter.Node, content, filePath string) string {
	current := node.Parent()

	for current != nil {
		if sa.astParser.IsSymbolNode(current.Type()) {
			return sa.astParser.extractSymbolName(current, content, filePath)
		}
		current = current.Parent()
	}

	return "unknown"
}

// AnalyzeDependencies analyzes the dependencies of a symbol by finding their definitions in the codebase
func (sa *ExternalRefsAnalyzer) AnalyzeDependencies(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) ([]DependencyInfo, error) {
	var dependencies []DependencyInfo

	// For each dependency of the symbol, find its definition in the codebase
	for _, dep := range symbol.Dependencies {
		depInfo := DependencyInfo{
			SymbolName: dep.Name,
		}

		// Find the definition of this dependency in the codebase
		definition, found := sa.findSymbolDefinitionInSnapshot(ctx, data, dep.Name)
		if found {
			depInfo.SourceFile = definition.FilePath
			depInfo.Code = definition.Code
			depInfo.Documentation = definition.Documentation
		}

		dependencies = append(dependencies, depInfo)
	}

	return dependencies, nil
}

// SymbolDefinition represents a symbol definition found in the codebase
type SymbolDefinition struct {
	FilePath      string
	LineNumber    int
	Code          string
	Documentation string
}

// findSymbolDefinitionInSnapshot finds the definition of a symbol within the repository snapshot
func (sa *ExternalRefsAnalyzer) findSymbolDefinitionInSnapshot(ctx context.Context, data *model.RepositorySnapshot, symbolName string) (SymbolDefinition, bool) {
	if symbolName == "" {
		return SymbolDefinition{}, false
	}

	// Search through all files in the snapshot
	for _, file := range data.Files {
		if file.IsBinary {
			continue // Skip binary files
		}

		// Try AST parsing first for more accurate results
		definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName)
		if found {
			return definition, true
		}

		// Fallback to text search if AST parsing fails
		definition, found = sa.findDefinitionWithTextSearch(file, symbolName)
		if found {
			return definition, true
		}
	}

	return SymbolDefinition{}, false
}

// findDefinitionInFileWithAST finds symbol definition using AST parsing
func (sa *ExternalRefsAnalyzer) findDefinitionInFileWithAST(ctx context.Context, file *model.RepositoryFile, symbolName string) (SymbolDefinition, bool) {
	rootNode, err := sa.astParser.GetFileAST(ctx, file.Path, file.Content)
	if err != nil {
		return SymbolDefinition{}, false
	}

	var definition SymbolDefinition
	found := sa.walkASTForDefinition(rootNode, file.Content, file.Path, symbolName, &definition)

	return definition, found
}

// walkASTForDefinition walks the AST to find symbol definitions
func (sa *ExternalRefsAnalyzer) walkASTForDefinition(node *sitter.Node, content, filePath, symbolName string, definition *SymbolDefinition) bool {
	nodeType := node.Type()

	// Check for function definitions, variable declarations, etc.
	if sa.astParser.IsSymbolNode(nodeType) {
		extractedName := sa.astParser.extractSymbolName(node, content, filePath)
		if extractedName == symbolName {
			*definition = SymbolDefinition{
				FilePath:      filePath,
				LineNumber:    int(node.StartPoint().Row) + 1,
				Code:          sa.astParser.getNodeText(node, content),
				Documentation: sa.extractDocumentation(node, content),
			}
			return true
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			if sa.walkASTForDefinition(child, content, filePath, symbolName, definition) {
				return true
			}
		}
	}

	return false
}

// findDefinitionWithTextSearch finds symbol definition using text patterns (fallback)
func (sa *ExternalRefsAnalyzer) findDefinitionWithTextSearch(file *model.RepositoryFile, symbolName string) (SymbolDefinition, bool) {
	lines := strings.Split(file.Content, "\n")

	// Common definition patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(fmt.Sprintf(`^func\s+%s\s*\(`, regexp.QuoteMeta(symbolName))),     // Go function
		regexp.MustCompile(fmt.Sprintf(`^function\s+%s\s*\(`, regexp.QuoteMeta(symbolName))), // JavaScript function
		regexp.MustCompile(fmt.Sprintf(`^def\s+%s\s*\(`, regexp.QuoteMeta(symbolName))),      // Python function
		regexp.MustCompile(fmt.Sprintf(`^class\s+%s\b`, regexp.QuoteMeta(symbolName))),       // Class definition
		regexp.MustCompile(fmt.Sprintf(`^type\s+%s\s`, regexp.QuoteMeta(symbolName))),        // Go type definition
		regexp.MustCompile(fmt.Sprintf(`^const\s+%s\s*=`, regexp.QuoteMeta(symbolName))),     // Constant definition
		regexp.MustCompile(fmt.Sprintf(`^var\s+%s\s`, regexp.QuoteMeta(symbolName))),         // Variable definition
		regexp.MustCompile(fmt.Sprintf(`^%s\s*:=`, regexp.QuoteMeta(symbolName))),            // Go short variable declaration
		regexp.MustCompile(fmt.Sprintf(`^%s\s*=`, regexp.QuoteMeta(symbolName))),             // Assignment
	}

	for lineNum, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		for _, pattern := range patterns {
			if pattern.MatchString(trimmedLine) {
				definition := SymbolDefinition{
					FilePath:      file.Path,
					LineNumber:    lineNum + 1,
					Code:          trimmedLine,
					Documentation: sa.extractDocumentationFromLines(lines, lineNum),
				}
				return definition, true
			}
		}
	}

	return SymbolDefinition{}, false
}

// extractDocumentation extracts documentation comments for a symbol
func (sa *ExternalRefsAnalyzer) extractDocumentation(node *sitter.Node, content string) string {
	// Look for comments immediately before the symbol
	lines := strings.Split(content, "\n")
	symbolLine := int(node.StartPoint().Row)

	return sa.extractDocumentationFromLines(lines, symbolLine)
}

// extractDocumentationFromLines extracts documentation from lines around a symbol
func (sa *ExternalRefsAnalyzer) extractDocumentationFromLines(lines []string, symbolLine int) string {
	var docLines []string

	// Look backwards for comment lines
	for i := symbolLine - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue // Skip empty lines
		}

		// Check for different comment styles
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "\"\"\"") ||
			strings.HasPrefix(line, "'''") {
			docLines = append([]string{line}, docLines...) // Prepend to maintain order
		} else {
			break // Stop at first non-comment line
		}

		// Don't look too far back
		if symbolLine-i > 10 {
			break
		}
	}

	if len(docLines) > 0 {
		return strings.Join(docLines, "\n")
	}

	return ""
}

// isStandardLibraryCall checks if a function call is to a standard library
func (sa *ExternalRefsAnalyzer) isStandardLibraryCall(callName string) bool {
	// Go standard library patterns
	stdLibPatterns := []string{
		"fmt.", "os.", "io.", "net.", "http.", "json.", "time.", "strings.", "strconv.",
		"context.", "sync.", "log.", "errors.", "path.", "filepath.", "regexp.",
	}

	for _, pattern := range stdLibPatterns {
		if strings.HasPrefix(callName, pattern) {
			return true
		}
	}

	// Language-specific standard library calls
	languageStdLib := map[string][]string{
		"python":     {"print", "len", "range", "str", "int", "list", "dict"},
		"javascript": {"console.log", "JSON.parse", "JSON.stringify", "parseInt", "parseFloat"},
	}

	for _, stdFuncs := range languageStdLib {
		for _, stdFunc := range stdFuncs {
			if callName == stdFunc || strings.HasPrefix(callName, stdFunc+".") {
				return true
			}
		}
	}

	return false
}

// isExternalLibraryCall checks if a function call is to an external library
func (sa *ExternalRefsAnalyzer) isExternalLibraryCall(callName string) bool {
	// This is a heuristic - in a real implementation, you might:
	// 1. Parse import statements
	// 2. Check against a list of known external libraries
	// 3. Use go.mod for Go projects

	// For now, assume calls with dots that aren't standard library are external
	return strings.Contains(callName, ".") && !sa.isStandardLibraryCall(callName)
}

// getStandardLibraryDoc gets documentation for standard library functions
func (sa *ExternalRefsAnalyzer) getStandardLibraryDoc(callName string) string {
	// This would ideally fetch from language documentation
	// For now, return a simple description
	if strings.HasPrefix(callName, "fmt.") {
		return "Go formatting package for I/O operations"
	}
	if strings.HasPrefix(callName, "json.") {
		return "JSON encoding and decoding functions"
	}
	// Add more as needed
	return "Standard library function"
}

// getExternalLibraryDoc gets documentation for external library functions
func (sa *ExternalRefsAnalyzer) getExternalLibraryDoc(callName string) string {
	// This would ideally integrate with package documentation APIs
	// For now, return a generic description
	return "External library function"
}

// findSymbolDefinition finds the file where a symbol is defined
func (sa *ExternalRefsAnalyzer) findSymbolDefinition(ctx context.Context, projectID, commitSHA, symbolName string) (string, error) {
	// This is a simplified implementation
	// In a real implementation, you would:
	// 1. Search through project files for symbol definitions
	// 2. Use language-specific tools (like gopls for Go)
	// 3. Parse import statements and module definitions

	return "", errm.New("symbol definition search not implemented")
}

// findRelatedFiles finds files that are related to the given file (files in the same directory)
func (sa *ExternalRefsAnalyzer) findRelatedFiles(ctx context.Context, data *model.RepositorySnapshot, filePath string) ([]string, error) {
	var relatedFiles []string

	// Get the directory of the target file
	targetDir := filepath.Dir(filePath)

	// Find all files in the same directory
	for _, file := range data.Files {
		fileDir := filepath.Dir(file.Path)

		// Skip the file itself and files in different directories
		if file.Path == filePath || fileDir != targetDir {
			continue
		}

		// Skip binary files and common non-source files
		if file.IsBinary || sa.shouldSkipFile(file.Path) {
			continue
		}

		relatedFiles = append(relatedFiles, file.Path)
	}

	return relatedFiles, nil
}

// shouldSkipFile determines if a file should be skipped when finding related files
func (sa *ExternalRefsAnalyzer) shouldSkipFile(filePath string) bool {
	skipPatterns := []string{
		".git/", ".svn/", ".hg/",
		"node_modules/", "vendor/", "__pycache__/",
		".pyc", ".pyo", ".class", ".o", ".so", ".dylib", ".dll",
		".exe", ".bin", ".tar", ".gz", ".zip", ".rar",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
	}

	lowerPath := strings.ToLower(filePath)
	for _, pattern := range skipPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	return false
}

// Enhanced context gathering for configuration files

// AnalyzeConfigFileChanges analyzes changes in configuration files
func (sa *ExternalRefsAnalyzer) AnalyzeConfigFileChanges(ctx context.Context, projectID, commitSHA string, configFilePath string, changes []string) ([]DependencyInfo, error) {
	var dependencies []DependencyInfo

	// Find files that read this configuration
	consumers, err := sa.findConfigFileConsumers(ctx, projectID, commitSHA, configFilePath)
	if err != nil {
		return nil, errm.Wrap(err, "failed to find config file consumers")
	}

	for _, consumer := range consumers {
		dep := DependencyInfo{
			SymbolName:    "config_reader",
			SourceFile:    consumer,
			Documentation: fmt.Sprintf("Code that reads configuration from %s", configFilePath),
		}
		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}

// findConfigFileConsumers finds code that reads a configuration file
func (sa *ExternalRefsAnalyzer) findConfigFileConsumers(ctx context.Context, projectID, commitSHA, configFilePath string) ([]string, error) {
	// This would search for:
	// 1. File path references in code
	// 2. Configuration loading patterns
	// 3. Environment variable usage patterns

	var consumers []string

	// Simple heuristic: look for files that mention the config file name
	configFileName := filepath.Base(configFilePath)

	// In a real implementation, you would:
	// 1. Use grep/ripgrep to search for the config file name
	// 2. Parse code to find configuration loading patterns
	// 3. Analyze import statements and file operations

	_ = configFileName // Placeholder to avoid unused variable

	return consumers, nil
}
