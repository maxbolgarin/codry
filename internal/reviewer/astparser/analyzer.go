package astparser

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/logze/v2"
	sitter "github.com/smacker/go-tree-sitter"
)

// Analyzer analyzes symbols and their relationships across the codebase
type Analyzer struct {
	astParser *ASTParser
	provider  interfaces.CodeProvider
	log       logze.Logger
}

// NewSymbolAnalyzer creates a new symbol analyzer
func NewAnalyzer(provider interfaces.CodeProvider) *Analyzer {
	return &Analyzer{
		astParser: NewParser(),
		provider:  provider,
		log:       logze.With("component", "analyzer"),
	}
}

// AnalyzeSymbolUsage analyzes the usage context of an affected symbol
func (sa *Analyzer) AnalyzeSymbolUsage(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) (usage SymbolUsageContext, err error) {
	// Find callers of this symbol
	usage.Callers, err = sa.FindSymbolCallers(ctx, data, symbol)
	if err != nil {
		sa.log.Warn("failed to find symbol callers", "error", err, "symbol", symbol.Name)
	}

	// Analyze dependencies of this symbol
	usage.Dependencies, err = sa.AnalyzeDependencies(ctx, data, symbol)
	if err != nil {
		sa.log.Warn("failed to analyze dependencies", "error", err, "symbol", symbol.Name)
	}

	return usage, nil
}

// findSymbolCallers finds all places where a symbol is called/used
func (sa *Analyzer) FindSymbolCallers(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) ([]Caller, error) {
	var callers []Caller

	if symbol.Name == "" {
		return callers, nil
	}

	potentialFiles := sa.getAffectedFiles(data, symbol)

	for _, file := range potentialFiles {
		rootNode, err := sa.astParser.GetFileAST(ctx, file.Path, file.Content)
		if err != nil {
			sa.log.Warn("failed to parse file to AST", "error", err, "file", file.Path)
			continue
		}
		sa.walkASTForCallers(rootNode, file.Content, file.Path, symbol, &callers)
	}

	return callers, nil
}

// getAffectedFiles uses more flexible matching to find potential caller files
func (sa *Analyzer) getAffectedFiles(data *model.RepositorySnapshot, symbol AffectedSymbol) []*model.RepositoryFile {
	var files []*model.RepositoryFile

	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		// Include the same file for internal method calls, but be selective
		if file.Path == symbol.FilePath {
			// For methods/functions, check if there are internal calls within the same struct/class
			if symbol.Type == "method" || symbol.Type == "function" {
				// Look for method calls that could be internal
				if strings.Contains(file.Content, symbol.Name+"(") {
					files = append(files, file)
				}
			}
			continue
		}

		// Quick text-based check first (most efficient)
		if strings.Contains(file.Content, symbol.Name) {
			files = append(files, file)
		}
	}

	return files
}

// walkASTForCallers walks the AST to find function calls
func (sa *Analyzer) walkASTForCallers(node *sitter.Node, content, filePath string, symbol AffectedSymbol, callers *[]Caller) {
	nodeType := node.Type()

	// Check for various call patterns
	if sa.isCallNode(nodeType) {
		// Try different extraction methods for call names
		callName := sa.extractCallNameFromNode(node, content)
		sa.log.Debug("found call node", "file", filePath, "node_type", nodeType, "call_name", callName, "looking_for", symbol.Name)

		if callName != "" && sa.isSymbolMatch(callName, symbol.Name) {
			sa.log.Debug("matched symbol call", "call_name", callName, "symbol", symbol.Name)

			call := Caller{
				FilePath: filePath,
				Name:     callName,
				Snippet:  sa.getCallSiteSnippet(node, content),
				Line:     int(node.StartPoint().Row) + 1,
				Type:     "function_call",
			}

			// Find the containing function
			containingFunc := sa.findContainingFunctionNode(node, content, filePath)
			if containingFunc != "" {
				call.FunctionName = containingFunc
			} else {
				call.FunctionName = "global"
			}

			*callers = append(*callers, call)
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

// isCallNode checks if the node represents a function/method call
func (sa *Analyzer) isCallNode(nodeType string) bool {
	// Common call patterns across different languages
	callPatterns := []string{
		"call_expression",
		"method_call",
		"function_call",
		"selector_expression",
		"field_expression",
		"index_expression",
		"postfix_expression",
		"invocation_expression",
		"member_access_expression",
	}

	for _, pattern := range callPatterns {
		if nodeType == pattern {
			return true
		}
	}

	// For debugging: also check if it contains certain keywords
	if strings.Contains(nodeType, "call") ||
		strings.Contains(nodeType, "invoke") ||
		strings.Contains(nodeType, "selector") {
		sa.log.Debug("potential call node found", "node_type", nodeType)
		return true
	}

	return false
}

// isSymbolMatch checks if a call name matches the target symbol
func (sa *Analyzer) isSymbolMatch(callName, symbolName string) bool {
	if callName == "" || symbolName == "" {
		return false
	}

	// Direct match
	if callName == symbolName {
		return true
	}

	// Check for qualified calls: something.SymbolName
	if strings.HasSuffix(callName, "."+symbolName) {
		return true
	}

	// Check for receiver method calls in Go: (receiver).SymbolName
	if strings.Contains(callName, ".") {
		parts := strings.Split(callName, ".")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == symbolName {
				return true
			}
		}
	}

	// For method calls, also check if the call name ends with the symbol
	if strings.Contains(callName, symbolName) {
		// Make sure it's not just a substring match, but a proper name match
		if strings.HasSuffix(callName, symbolName) {
			// Check if it's preceded by a dot or is at the start
			if len(callName) == len(symbolName) {
				return true
			}
			if len(callName) > len(symbolName) {
				precedingChar := callName[len(callName)-len(symbolName)-1]
				if precedingChar == '.' || precedingChar == ' ' {
					return true
				}
			}
		}
	}

	return false
}

// extractCallNameFromNode extracts the function/method name being called from an AST node
func (sa *Analyzer) extractCallNameFromNode(node *sitter.Node, content string) string {
	nodeType := node.Type()

	// Handle different types of call expressions
	switch nodeType {
	case "call_expression":
		// Look for the function name child
		childCount := int(node.ChildCount())
		for i := 0; i < childCount; i++ {
			child := node.Child(i)
			if child != nil {
				childType := child.Type()
				if childType == "identifier" {
					return sa.astParser.getNodeText(child, content)
				} else if childType == "selector_expression" || childType == "field_expression" {
					// For method calls like obj.method()
					return sa.extractMethodCallName(child, content)
				}
			}
		}
	case "selector_expression", "field_expression":
		return sa.extractMethodCallName(node, content)
	}

	// Fallback: extract text and try to parse it
	nodeText := sa.astParser.getNodeText(node, content)
	if strings.Contains(nodeText, "(") {
		// Extract just the function name before the parentheses
		parts := strings.Split(nodeText, "(")
		if len(parts) > 0 {
			callName := strings.TrimSpace(parts[0])
			// Clean up whitespace and newlines
			callName = strings.ReplaceAll(callName, "\n", " ")
			callName = strings.ReplaceAll(callName, "\t", " ")
			// If it has dots, it might be a method call
			if strings.Contains(callName, ".") {
				dotParts := strings.Split(callName, ".")
				if len(dotParts) > 1 {
					return strings.TrimSpace(dotParts[len(dotParts)-1])
				}
			}
			return callName
		}
	}

	return ""
}

// extractMethodCallName extracts method name from selector/field expressions
func (sa *Analyzer) extractMethodCallName(node *sitter.Node, content string) string {
	// For expressions like "obj.method", we want to extract "method"
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == "field_identifier" {
			return sa.astParser.getNodeText(child, content)
		}
	}

	// Fallback: parse the full text
	nodeText := sa.astParser.getNodeText(node, content)
	if strings.Contains(nodeText, ".") {
		parts := strings.Split(nodeText, ".")
		if len(parts) > 1 {
			lastPart := strings.TrimSpace(parts[len(parts)-1])
			// Remove any trailing parentheses or other characters
			if idx := strings.Index(lastPart, "("); idx != -1 {
				lastPart = lastPart[:idx]
			}
			return lastPart
		}
	}

	return ""
}

// getCallSiteSnippet gets a code snippet around a function call site
func (sa *Analyzer) getCallSiteSnippet(node *sitter.Node, content string) string {
	// Get the line of the call
	line := int(node.StartPoint().Row)
	lines := strings.Split(content, "\n")
	contextLines := 3

	start := line - contextLines
	if start < 0 {
		start = 0
	}

	end := line + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	snippet := strings.Join(lines[start:end], "\n")
	return strings.TrimSpace(snippet)
}

// findContainingFunctionNode finds the name of the function containing a node
func (sa *Analyzer) findContainingFunctionNode(node *sitter.Node, content, filePath string) string {
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
func (sa *Analyzer) AnalyzeDependencies(ctx context.Context, data *model.RepositorySnapshot, symbol AffectedSymbol) ([]Dependency, error) {
	var dependencies []Dependency

	// For each dependency of the symbol, find its definition in the codebase
	for _, dep := range symbol.Dependencies {
		if dep.Name == "" {
			continue
		}

		sa.log.Debug("analyzing dependency", "dep_name", dep.Name, "symbol_file", symbol.FilePath)

		// Find the definition of this dependency in the codebase
		definition, found := sa.findSymbolDefinitionInSnapshot(ctx, data, dep.Name, symbol.FilePath)
		if found {
			dep.SourceFile = definition.FilePath
			dep.SourceCode = definition.Code
			dep.Documentation = definition.Documentation
			sa.log.Debug("found dependency definition", "dep_name", dep.Name, "source_file", definition.FilePath)
		} else {
			dep.SourceFile = "external"
			dep.SourceCode = "// " + dep.Name + "() - definition not found in current scope"
			sa.log.Debug("dependency not found", "dep_name", dep.Name)
		}

		dependencies = append(dependencies, dep)
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

// findSymbolDefinitionInSnapshotImproved uses enhanced search to find symbol definitions
func (sa *Analyzer) findSymbolDefinitionInSnapshot(ctx context.Context, data *model.RepositorySnapshot, symbolName, originFilePath string) (SymbolDefinition, bool) {
	// Clean the symbol name (remove receiver patterns like "cf.processFileDiff" -> "processFileDiff")
	cleanSymbolName := sa.cleanSymbolName(symbolName)

	// Try exact match first (most common case)
	if definition, found := sa.findDefinitionByExactMatch(ctx, data, cleanSymbolName, originFilePath); found {
		return definition, true
	}

	// Try same package (second most common)
	if definition, found := sa.findDefinitionInSamePackage(ctx, data, cleanSymbolName, originFilePath); found {
		return definition, true
	}

	return SymbolDefinition{}, false
}

// cleanSymbolName extracts the actual symbol name from qualified names
func (sa *Analyzer) cleanSymbolName(symbolName string) string {
	// Handle patterns like "cf.processFileDiff" -> "processFileDiff"
	if strings.Contains(symbolName, ".") {
		parts := strings.Split(symbolName, ".")
		return parts[len(parts)-1]
	}
	return symbolName
}

// findDefinitionByExactMatch finds definitions using AST-based symbol detection (rewritten for accuracy)
func (sa *Analyzer) findDefinitionByExactMatch(ctx context.Context, data *model.RepositorySnapshot, symbolName, originFilePath string) (SymbolDefinition, bool) {
	// First, check the same file where the symbol is used (most common case for method definitions)
	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		if file.Path == originFilePath {
			if definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName); found {
				return definition, true
			}
			break // We found the origin file, no need to continue this loop
		}
	}

	// Then prioritize files in the same directory
	originDir := filepath.Dir(originFilePath)

	// Check same directory (excluding the origin file which we already checked)
	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		if filepath.Dir(file.Path) == originDir && file.Path != originFilePath {
			if definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName); found {
				return definition, true
			}
		}
	}

	// Finally check other files
	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		// Skip files we already checked
		if filepath.Dir(file.Path) == originDir {
			continue
		}

		if definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName); found {
			return definition, true
		}
	}

	return SymbolDefinition{}, false
}

// findDefinitionInSamePackage prioritizes definitions in the same package/directory using AST
func (sa *Analyzer) findDefinitionInSamePackage(ctx context.Context, data *model.RepositorySnapshot, symbolName, originFilePath string) (SymbolDefinition, bool) {
	// First check the same file
	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		if file.Path == originFilePath {
			if definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName); found {
				return definition, true
			}
			break
		}
	}

	// Then check other files in the same directory
	originDir := filepath.Dir(originFilePath)

	for _, file := range data.Files {
		if file.IsBinary {
			continue
		}

		// Only check files in the same directory (excluding the origin file)
		if filepath.Dir(file.Path) == originDir && file.Path != originFilePath {
			if definition, found := sa.findDefinitionInFileWithAST(ctx, file, symbolName); found {
				return definition, true
			}
		}
	}

	return SymbolDefinition{}, false
}

// findDefinitionInFileWithAST finds symbol definition using AST parsing (replaces string-based search)
func (sa *Analyzer) findDefinitionInFileWithAST(ctx context.Context, file *model.RepositoryFile, symbolName string) (SymbolDefinition, bool) {
	// Parse the file to get its AST
	rootNode, err := sa.astParser.GetFileAST(ctx, file.Path, file.Content)
	if err != nil {
		// Fallback to string-based search if AST parsing fails
		return sa.findDefinitionWithStringSearch(file, symbolName)
	}

	// Walk the AST to find the symbol definition
	var definition SymbolDefinition
	found := sa.walkASTForDefinition(rootNode, file.Content, file.Path, symbolName, &definition)

	return definition, found
}

// walkASTForDefinition walks the AST to find symbol definitions with complete code extraction
func (sa *Analyzer) walkASTForDefinition(node *sitter.Node, content, filePath, symbolName string, definition *SymbolDefinition) bool {
	nodeType := node.Type()

	// Check for function definitions, variable declarations, etc.
	if sa.astParser.IsSymbolNode(nodeType) {
		extractedName := sa.astParser.extractSymbolName(node, content, filePath)
		sa.log.Debug("checking node for definition", "node_type", nodeType, "extracted_name", extractedName, "looking_for", symbolName)

		if extractedName == symbolName {
			// Extract the complete code using AST node boundaries
			fullCode := sa.astParser.getNodeText(node, content)

			// For functions, try to get the complete implementation including body
			if strings.Contains(nodeType, "function") || strings.Contains(nodeType, "method") {
				fullCode = sa.extractCompleteFunction(node, content)
			}

			*definition = SymbolDefinition{
				FilePath:      filePath,
				LineNumber:    int(node.StartPoint().Row) + 1,
				Code:          strings.TrimSpace(fullCode),
				Documentation: sa.extractDocumentation(node, content),
			}
			sa.log.Debug("found symbol definition", "symbol", symbolName, "file", filePath, "line", definition.LineNumber)
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

// extractCompleteFunction extracts the complete function code including body using AST
func (sa *Analyzer) extractCompleteFunction(node *sitter.Node, content string) string {
	// Get the full function including the body
	startByte := int(node.StartByte())
	endByte := int(node.EndByte())

	contentBytes := []byte(content)
	if startByte >= 0 && endByte <= len(contentBytes) && startByte < endByte {
		functionCode := string(contentBytes[startByte:endByte])

		// For large functions, limit to first few lines of body to avoid huge context
		lines := strings.Split(functionCode, "\n")
		if len(lines) > 20 {
			// Take function signature + some body lines
			var result []string
			openBraceFound := false
			bodyLines := 0

			for i, line := range lines {
				result = append(result, line)

				// Track when we find the opening brace
				if strings.Contains(line, "{") {
					openBraceFound = true
				}

				// After opening brace, count body lines
				if openBraceFound && i > 0 {
					bodyLines++
					if bodyLines >= 10 { // Limit body to ~10 lines
						result = append(result, "\t// ... rest of function body ...")

						// Try to add the closing brace
						for j := len(lines) - 1; j >= 0; j-- {
							if strings.Contains(lines[j], "}") {
								result = append(result, lines[j])
								break
							}
						}
						break
					}
				}
			}

			return strings.Join(result, "\n")
		}

		return functionCode
	}

	// Fallback to basic node text
	return sa.astParser.getNodeText(node, content)
}

// findDefinitionWithStringSearch provides fallback string-based search when AST parsing fails
func (sa *Analyzer) findDefinitionWithStringSearch(file *model.RepositoryFile, symbolName string) (SymbolDefinition, bool) {
	lines := strings.Split(file.Content, "\n")

	for lineNum, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check for function definitions (Go patterns)
		if strings.HasPrefix(trimmedLine, "func ") && strings.Contains(trimmedLine, symbolName) {
			// Extract function signature and some body
			codeSnippet := sa.extractDefinitionSnippet(lines, lineNum)

			return SymbolDefinition{
				FilePath:      file.Path,
				LineNumber:    lineNum + 1,
				Code:          codeSnippet,
				Documentation: sa.extractDocumentationFromLines(lines, lineNum),
			}, true
		}

		// Check for method definitions (Go patterns)
		if strings.Contains(trimmedLine, "func (") && strings.Contains(trimmedLine, symbolName) {
			codeSnippet := sa.extractDefinitionSnippet(lines, lineNum)

			return SymbolDefinition{
				FilePath:      file.Path,
				LineNumber:    lineNum + 1,
				Code:          codeSnippet,
				Documentation: sa.extractDocumentationFromLines(lines, lineNum),
			}, true
		}
	}

	return SymbolDefinition{}, false
}

// extractDefinitionSnippet extracts code snippet for string-based fallback
func (sa *Analyzer) extractDefinitionSnippet(lines []string, definitionLine int) string {
	start := definitionLine
	end := definitionLine + 1

	// For functions, try to capture the signature and opening brace
	if definitionLine < len(lines) {
		line := strings.TrimSpace(lines[definitionLine])

		if strings.Contains(line, "func ") {
			// Look for opening brace in next few lines
			for i := definitionLine; i < len(lines) && i < definitionLine+10; i++ {
				if strings.Contains(lines[i], "{") {
					end = i + 1
					break
				}
			}

			// Include a few lines of the body
			if end < len(lines) {
				bodyLines := min(8, len(lines)-end)
				end += bodyLines
			}
		}
	}

	if end > len(lines) {
		end = len(lines)
	}

	snippet := strings.Join(lines[start:end], "\n")
	return strings.TrimSpace(snippet)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractDocumentation extracts documentation comments for a symbol
func (sa *Analyzer) extractDocumentation(node *sitter.Node, content string) string {
	// Look for comments immediately before the symbol
	lines := strings.Split(content, "\n")
	symbolLine := int(node.StartPoint().Row)

	return sa.extractDocumentationFromLines(lines, symbolLine)
}

// extractDocumentationFromLines extracts documentation from lines around a symbol
func (sa *Analyzer) extractDocumentationFromLines(lines []string, symbolLine int) string {
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
