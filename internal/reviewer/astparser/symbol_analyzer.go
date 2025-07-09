package astparser

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	sitter "github.com/smacker/go-tree-sitter"
)

// SymbolAnalyzer analyzes symbols and their relationships across the codebase
type SymbolAnalyzer struct {
	astParser *Parser
	provider  interfaces.CodeProvider
	log       logze.Logger
}

// CallerInfo represents information about where a symbol is called
type CallerInfo struct {
	FilePath     string `json:"file_path"`
	LineNumber   int    `json:"line_number"`
	FunctionName string `json:"function_name"`
	Context      string `json:"context"`
	CodeSnippet  string `json:"code_snippet"`
	CallType     string `json:"call_type"` // "direct", "method", "indirect"
}

// DependencyInfo represents information about a dependency of a symbol
type DependencyInfo struct {
	SymbolName      string `json:"symbol_name"`
	Source          string `json:"source"`           // "internal", "external", "standard_library"
	FilePath        string `json:"file_path"`        // For internal dependencies
	Documentation   string `json:"documentation"`    // For external/stdlib dependencies
	ImportStatement string `json:"import_statement"` // How it's imported
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
func NewSymbolAnalyzer(provider interfaces.CodeProvider) *SymbolAnalyzer {
	return &SymbolAnalyzer{
		astParser: NewParser(),
		provider:  provider,
		log:       logze.With("component", "symbol_analyzer"),
	}
}

// AnalyzeSymbolUsage analyzes the usage context of an affected symbol
func (sa *SymbolAnalyzer) AnalyzeSymbolUsage(ctx context.Context, projectID, commitSHA string, symbol AffectedSymbol) (SymbolUsageContext, error) {
	usage := SymbolUsageContext{}

	// Find callers of this symbol
	callers, err := sa.findSymbolCallers(ctx, projectID, commitSHA, symbol)
	if err != nil {
		sa.log.Warn("failed to find symbol callers", "error", err, "symbol", symbol.Name)
	} else {
		usage.Callers = callers
	}

	// Analyze dependencies of this symbol
	dependencies, err := sa.analyzeDependencies(ctx, projectID, commitSHA, symbol)
	if err != nil {
		sa.log.Warn("failed to analyze dependencies", "error", err, "symbol", symbol.Name)
	} else {
		usage.Dependencies = dependencies
	}

	// Find related files (files that import or use this symbol's file)
	relatedFiles, err := sa.findRelatedFiles(ctx, projectID, commitSHA, symbol.FilePath)
	if err != nil {
		sa.log.Warn("failed to find related files", "error", err, "symbol", symbol.Name)
	} else {
		usage.RelatedFiles = relatedFiles
	}

	return usage, nil
}

// findSymbolCallers finds all places where a symbol is called/used
func (sa *SymbolAnalyzer) findSymbolCallers(ctx context.Context, projectID, commitSHA string, symbol AffectedSymbol) ([]CallerInfo, error) {
	var callers []CallerInfo

	// First, try to use text-based search to find potential callers
	// This is faster than parsing every file
	potentialFiles, err := sa.findFilesContainingSymbol(ctx, projectID, commitSHA, symbol.Name)
	if err != nil {
		return nil, errm.Wrap(err, "failed to find files containing symbol")
	}

	// Analyze each potential file with AST parsing for accurate results
	for _, filePath := range potentialFiles {
		if filePath == symbol.FilePath {
			continue // Skip the file where symbol is defined
		}

		fileCallers, err := sa.analyzeFileForCallers(ctx, projectID, commitSHA, filePath, symbol)
		if err != nil {
			sa.log.Warn("failed to analyze file for callers", "error", err, "file", filePath)
			continue
		}

		callers = append(callers, fileCallers...)
	}

	return callers, nil
}

// findFilesContainingSymbol uses text search to find files that might contain the symbol
func (sa *SymbolAnalyzer) findFilesContainingSymbol(ctx context.Context, projectID, commitSHA, symbolName string) ([]string, error) {
	// This is a simplified implementation
	// In a real implementation, you might want to:
	// 1. Use grep or ripgrep for fast text search
	// 2. Get a list of all files in the repository
	// 3. Search through them for the symbol name

	// For now, we'll return an empty slice and let the calling code handle it
	// In a real implementation, you would integrate with your git provider
	// to get file contents and search through them

	return []string{}, nil
}

// analyzeFileForCallers analyzes a specific file to find callers of a symbol
func (sa *SymbolAnalyzer) analyzeFileForCallers(ctx context.Context, projectID, commitSHA, filePath string, symbol AffectedSymbol) ([]CallerInfo, error) {
	// Get file content
	content, err := sa.provider.GetFileContent(ctx, projectID, filePath, commitSHA)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get file content")
	}

	// Parse file to AST
	rootNode, err := sa.astParser.ParseFileToAST(ctx, filePath, content)
	if err != nil {
		// If AST parsing fails, fall back to text search
		return sa.findCallersWithTextSearch(content, filePath, symbol), nil
	}

	// Find function calls in the AST
	var callers []CallerInfo
	sa.walkASTForCallers(rootNode, content, filePath, symbol, &callers)

	return callers, nil
}

// findCallersWithTextSearch finds callers using simple text matching (fallback)
func (sa *SymbolAnalyzer) findCallersWithTextSearch(content, filePath string, symbol AffectedSymbol) []CallerInfo {
	var callers []CallerInfo
	lines := strings.Split(content, "\n")

	// Create regex patterns for different call types
	patterns := sa.createCallPatterns(symbol)

	for lineNum, line := range lines {
		for _, pattern := range patterns {
			if matches := pattern.regex.FindAllStringSubmatch(line, -1); len(matches) > 0 {
				caller := CallerInfo{
					FilePath:    filePath,
					LineNumber:  lineNum + 1, // Convert to 1-based
					Context:     strings.TrimSpace(line),
					CodeSnippet: sa.extractCodeSnippet(lines, lineNum, 2),
					CallType:    pattern.callType,
				}

				// Try to determine the function this call is in
				caller.FunctionName = sa.findContainingFunction(lines, lineNum)

				callers = append(callers, caller)
			}
		}
	}

	return callers
}

// callPattern represents a regex pattern for finding function calls
type callPattern struct {
	regex    *regexp.Regexp
	callType string
}

// createCallPatterns creates regex patterns to find different types of calls to a symbol
func (sa *SymbolAnalyzer) createCallPatterns(symbol AffectedSymbol) []callPattern {
	symbolName := regexp.QuoteMeta(symbol.Name)

	patterns := []callPattern{
		{
			regex:    regexp.MustCompile(fmt.Sprintf(`\b%s\s*\(`, symbolName)),
			callType: "direct",
		},
		{
			regex:    regexp.MustCompile(fmt.Sprintf(`\.%s\s*\(`, symbolName)),
			callType: "method",
		},
		{
			regex:    regexp.MustCompile(fmt.Sprintf(`\b%s\b`, symbolName)),
			callType: "reference",
		},
	}

	return patterns
}

// extractCodeSnippet extracts a code snippet around a specific line
func (sa *SymbolAnalyzer) extractCodeSnippet(lines []string, centerLine, contextLines int) string {
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

// findContainingFunction finds the function that contains a specific line
func (sa *SymbolAnalyzer) findContainingFunction(lines []string, targetLine int) string {
	// Look backwards from the target line to find a function declaration
	for i := targetLine; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		// Simple patterns for function declarations
		funcPatterns := []*regexp.Regexp{
			regexp.MustCompile(`^func\s+(\w+)`),           // Go
			regexp.MustCompile(`^function\s+(\w+)`),       // JavaScript
			regexp.MustCompile(`^def\s+(\w+)`),            // Python
			regexp.MustCompile(`^(\w+)\s*\([^)]*\)\s*\{`), // C-style
			regexp.MustCompile(`^(\w+):\s*function`),      // JavaScript object method
		}

		for _, pattern := range funcPatterns {
			if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
				return matches[1]
			}
		}

		// Don't look too far back
		if targetLine-i > 50 {
			break
		}
	}

	return "unknown"
}

// walkASTForCallers walks the AST to find function calls
func (sa *SymbolAnalyzer) walkASTForCallers(node *sitter.Node, content, filePath string, symbol AffectedSymbol, callers *[]CallerInfo) {
	nodeType := node.Type()

	// Check if this node is a function call that matches our symbol
	if sa.astParser.isFunctionCallNode(nodeType) {
		call := sa.astParser.extractFunctionCall(node, content)
		if sa.isCallToSymbol(call, symbol) {
			caller := CallerInfo{
				FilePath:    filePath,
				LineNumber:  call.Line,
				Context:     sa.astParser.getNodeText(node, content),
				CallType:    "direct",
				CodeSnippet: sa.getCallSiteSnippet(node, content),
			}

			// Find the containing function
			containingFunc := sa.findContainingFunctionNode(node, content)
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

// isCallToSymbol checks if a function call is calling our target symbol
func (sa *SymbolAnalyzer) isCallToSymbol(call FunctionCall, symbol AffectedSymbol) bool {
	// Simple name matching - could be enhanced with more sophisticated logic
	return call.Name == symbol.Name || strings.HasSuffix(call.Name, "."+symbol.Name)
}

// getCallSiteSnippet gets a code snippet around a function call site
func (sa *SymbolAnalyzer) getCallSiteSnippet(node *sitter.Node, content string) string {
	// Get the line of the call
	line := int(node.StartPoint().Row)
	lines := strings.Split(content, "\n")

	return sa.extractCodeSnippet(lines, line, 2)
}

// findContainingFunctionNode finds the name of the function containing a node
func (sa *SymbolAnalyzer) findContainingFunctionNode(node *sitter.Node, content string) string {
	current := node.Parent()

	for current != nil {
		if sa.astParser.IsSymbolNode(current.Type()) {
			return sa.astParser.extractFunctionName(current, content)
		}
		current = current.Parent()
	}

	return "unknown"
}

// analyzeDependencies analyzes the dependencies of a symbol
func (sa *SymbolAnalyzer) analyzeDependencies(ctx context.Context, projectID, commitSHA string, symbol AffectedSymbol) ([]DependencyInfo, error) {
	var dependencies []DependencyInfo

	// Analyze the function calls within the symbol
	for _, call := range symbol.Dependencies {
		dep := DependencyInfo{
			SymbolName: call.Name,
		}

		// Determine if it's internal, external, or standard library
		if sa.isStandardLibraryCall(call.Name) {
			dep.Source = "standard_library"
			dep.Documentation = sa.getStandardLibraryDoc(call.Name)
		} else if sa.isExternalLibraryCall(call.Name) {
			dep.Source = "external"
			dep.Documentation = sa.getExternalLibraryDoc(call.Name)
		} else {
			dep.Source = "internal"
			// Try to find the file where this symbol is defined
			definitionFile, err := sa.findSymbolDefinition(ctx, projectID, commitSHA, call.Name)
			if err == nil {
				dep.FilePath = definitionFile
			}
		}

		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}

// isStandardLibraryCall checks if a function call is to a standard library
func (sa *SymbolAnalyzer) isStandardLibraryCall(callName string) bool {
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
func (sa *SymbolAnalyzer) isExternalLibraryCall(callName string) bool {
	// This is a heuristic - in a real implementation, you might:
	// 1. Parse import statements
	// 2. Check against a list of known external libraries
	// 3. Use go.mod for Go projects

	// For now, assume calls with dots that aren't standard library are external
	return strings.Contains(callName, ".") && !sa.isStandardLibraryCall(callName)
}

// getStandardLibraryDoc gets documentation for standard library functions
func (sa *SymbolAnalyzer) getStandardLibraryDoc(callName string) string {
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
func (sa *SymbolAnalyzer) getExternalLibraryDoc(callName string) string {
	// This would ideally integrate with package documentation APIs
	// For now, return a generic description
	return "External library function"
}

// findSymbolDefinition finds the file where a symbol is defined
func (sa *SymbolAnalyzer) findSymbolDefinition(ctx context.Context, projectID, commitSHA, symbolName string) (string, error) {
	// This is a simplified implementation
	// In a real implementation, you would:
	// 1. Search through project files for symbol definitions
	// 2. Use language-specific tools (like gopls for Go)
	// 3. Parse import statements and module definitions

	return "", errm.New("symbol definition search not implemented")
}

// findRelatedFiles finds files that are related to the given file
func (sa *SymbolAnalyzer) findRelatedFiles(ctx context.Context, projectID, commitSHA, filePath string) ([]string, error) {
	// This would find:
	// 1. Files that import this file
	// 2. Files that this file imports
	// 3. Files in the same package/module
	// 4. Test files for this file

	var relatedFiles []string

	// Add test files (simple heuristic)
	if testFile := sa.findTestFile(filePath); testFile != "" {
		relatedFiles = append(relatedFiles, testFile)
	}

	// Add files in the same directory (package for Go)
	dirFiles := sa.findFilesInSameDirectory(filePath)
	relatedFiles = append(relatedFiles, dirFiles...)

	return relatedFiles, nil
}

// findTestFile finds the corresponding test file for a source file
func (sa *SymbolAnalyzer) findTestFile(filePath string) string {
	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filePath, ext)

	// Go test files
	if ext == ".go" {
		return base + "_test.go"
	}

	// Python test files
	if ext == ".py" {
		dir := filepath.Dir(filePath)
		filename := filepath.Base(base)
		return filepath.Join(dir, "test_"+filename+ext)
	}

	// JavaScript test files
	if ext == ".js" {
		return base + ".test.js"
	}

	return ""
}

// findFilesInSameDirectory finds other source files in the same directory
func (sa *SymbolAnalyzer) findFilesInSameDirectory(filePath string) []string {
	// This is a simplified implementation
	// In a real implementation, you would list directory contents
	return []string{}
}

// Enhanced context gathering for configuration files

// AnalyzeConfigFileChanges analyzes changes in configuration files
func (sa *SymbolAnalyzer) AnalyzeConfigFileChanges(ctx context.Context, projectID, commitSHA string, configFilePath string, changes []string) ([]DependencyInfo, error) {
	var dependencies []DependencyInfo

	// Find files that read this configuration
	consumers, err := sa.findConfigFileConsumers(ctx, projectID, commitSHA, configFilePath)
	if err != nil {
		return nil, errm.Wrap(err, "failed to find config file consumers")
	}

	for _, consumer := range consumers {
		dep := DependencyInfo{
			SymbolName:    "config_reader",
			Source:        "internal",
			FilePath:      consumer,
			Documentation: fmt.Sprintf("Code that reads configuration from %s", configFilePath),
		}
		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}

// findConfigFileConsumers finds code that reads a configuration file
func (sa *SymbolAnalyzer) findConfigFileConsumers(ctx context.Context, projectID, commitSHA, configFilePath string) ([]string, error) {
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
