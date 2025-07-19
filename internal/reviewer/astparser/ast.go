package astparser

import (
	"context"
	"maps"
	"strings"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/erro"
	sitter "github.com/smacker/go-tree-sitter"
)

// ASTParser handles parsing code using Tree-sitter to map lines to syntax nodes
type ASTParser struct {
	languages map[ProgrammingLanguage]*sitter.Language

	astCache *abstract.SafeMap[string, *sitter.Node]
}

// NewParser creates a new AST parser with supported languages
func NewParser() *ASTParser {
	languages := make(map[ProgrammingLanguage]*sitter.Language, len(languagesParsers))
	maps.Copy(languages, languagesParsers)
	return &ASTParser{
		languages: languages,
		astCache:  abstract.NewSafeMap[string, *sitter.Node](),
	}
}

// ParseFileToAST parses a file's content to AST using Tree-sitter
func (p *ASTParser) GetFileAST(ctx context.Context, filePath, content string) (*sitter.Node, error) {
	if node, ok := p.astCache.Lookup(filePath); ok {
		return node, nil
	}

	language := DetectProgrammingLanguage(filePath)
	languageParser, ok := p.languages[language]
	if !ok {
		return nil, erro.New("unsupported file type for AST parsing: %s", language)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(languageParser)

	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil {
		return nil, erro.Wrap(err, "failed to parse AST", "file", filePath)
	}

	p.astCache.Set(filePath, tree.RootNode())

	return tree.RootNode(), nil
}

// FindSmallestEnclosingNode finds the smallest AST node that encloses the given line
func (p *ASTParser) FindSmallestEnclosingNode(rootNode *sitter.Node, lineNumber int) *sitter.Node {
	return p.findSmallestEnclosingNodeRecursive(rootNode, lineNumber)
}

// findSmallestEnclosingNodeRecursive recursively searches for the smallest enclosing node
func (p *ASTParser) findSmallestEnclosingNodeRecursive(node *sitter.Node, lineNumber int) *sitter.Node {
	// Convert line number to 0-based (Tree-sitter uses 0-based line numbers)
	targetLine := uint32(lineNumber - 1)

	startLine := node.StartPoint().Row
	endLine := node.EndPoint().Row

	// Check if the line is within this node
	if targetLine < startLine || targetLine > endLine {
		return nil
	}

	// Check children to find a more specific node
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		// Recursively check child
		if result := p.findSmallestEnclosingNodeRecursive(child, lineNumber); result != nil {
			return result
		}
	}

	// If no child contains the line more specifically, this node is the smallest enclosing one
	return node
}

// FindAffectedSymbols finds all symbols affected by changes in the given lines
func (p *ASTParser) FindAffectedSymbols(ctx context.Context, filePath, fileContent string, changedLines []int) ([]AffectedSymbol, error) {
	rootNode, err := p.GetFileAST(ctx, filePath, fileContent)
	if err != nil {
		return nil, erro.Wrap(err, "failed to parse file to AST", "file", filePath)
	}

	var symbols []AffectedSymbol
	symbolsFound := make(map[string]bool) // To avoid duplicates

	// Find symbols for each changed line
	for _, lineNumber := range changedLines {
		enclosingNode := p.FindSmallestEnclosingNode(rootNode, lineNumber)
		if enclosingNode == nil {
			continue
		}

		// Find the parent symbol (function, class, etc.)
		symbolNode := p.findParentSymbolNode(enclosingNode)
		if symbolNode == nil {
			continue
		}

		symbol := p.ExtractSymbolFromNode(symbolNode, filePath, fileContent)
		if symbol.Name == "" {
			continue
		}

		// Use a unique key to avoid duplicates
		symbolKey := symbol.FilePath + ":" + string(symbol.Type) + ":" + symbol.Name
		if !symbolsFound[symbolKey] {
			symbols = append(symbols, symbol)
			symbolsFound[symbolKey] = true
		}
	}

	return symbols, nil
}

// findParentSymbolNode finds the parent node that represents a symbol (function, class, etc.)
func (p *ASTParser) findParentSymbolNode(node *sitter.Node) *sitter.Node {
	current := node

	for current != nil {
		nodeType := current.Type()

		// Check if this node represents a symbol we're interested in
		if p.IsSymbolNode(nodeType) {
			return current
		}

		current = current.Parent()
	}

	return nil
}

// IsSymbolNode checks if a node type represents a code symbol we're interested in
func (p *ASTParser) IsSymbolNode(nodeType string) bool {
	return symbolNodes[nodeType]
}

// ExtractSymbolFromNode extracts symbol information from an AST node
func (p *ASTParser) ExtractSymbolFromNode(node *sitter.Node, filePath, fileContent string) AffectedSymbol {
	symbol := AffectedSymbol{
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1, // Convert to 1-based
		EndLine:   int(node.EndPoint().Row) + 1,   // Convert to 1-based
		Name:      p.extractSymbolName(node, fileContent, filePath),
		Type:      getSymbolType(node.Type()),
	}
	symbol.DocComment = p.extractDocComment(fileContent, symbol.StartLine)
	symbol.Dependencies = p.extractDependencies(node, fileContent, symbol.Type == SymbolTypeFunction || symbol.Type == SymbolTypeMethod)

	lines := strings.Split(fileContent, "\n")

	if symbol.StartLine > 0 && symbol.EndLine > 0 && symbol.EndLine <= len(lines) {
		symbolLines := lines[symbol.StartLine-1 : symbol.EndLine]
		symbol.FullCode = strings.Join(symbolLines, "\n")
	}

	return symbol
}

// extractSymbolName extracts the symbol name from a symbol declaration node
func (p *ASTParser) extractSymbolName(node *sitter.Node, content string, filePath string) string {
	childCount := int(node.ChildCount())

	// First, try to find a direct identifier child
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") ||
			strings.Contains(childType, "name") ||
			strings.Contains(childType, "variable_name") ||
			strings.Contains(childType, "function_name") ||
			strings.Contains(childType, "class_name") ||
			strings.Contains(childType, "method_name") {
			name := p.getNodeText(child, content)
			if name != "" {
				return name
			}
		}
	}

	// For languages with different patterns, try alternative approaches
	language := DetectProgrammingLanguage(filePath)
	switch language {
	case LanguageGo:
		return p.extractGoSymbolName(node, content)
	case LanguageJavaScript, LanguageTypeScript, LanguageTSX:
		return p.extractJSSymbolName(node, content)
	case LanguagePython:
		return p.extractPythonSymbolName(node, content)
	case LanguageJava:
		return p.extractJavaSymbolName(node, content)
	case LanguageCpp:
		return p.extractCppSymbolName(node, content)
	}

	// Fallback: look for any identifier in the node
	return p.findFirstIdentifier(node, content)
}

// extractGoSymbolName extracts symbol names from Go AST nodes
func (p *ASTParser) extractGoSymbolName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	switch nodeType {
	case "function_declaration", "method_declaration":
		// Look for the function name in the function spec
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "function_spec" {
				return p.extractGoSymbolName(child, content)
			}
		}
	case "function_spec":
		// Function name is usually the first identifier
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && strings.Contains(child.Type(), "identifier") {
				return p.getNodeText(child, content)
			}
		}
	case "type_declaration", "var_declaration", "const_declaration":
		// Look for the type/var/const spec
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && (strings.Contains(child.Type(), "spec") || strings.Contains(child.Type(), "declaration")) {
				return p.extractGoSymbolName(child, content)
			}
		}
	}

	return p.findFirstIdentifier(node, content)
}

// extractJSSymbolName extracts symbol names from JavaScript/TypeScript AST nodes
func (p *ASTParser) extractJSSymbolName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	switch nodeType {
	case "function_declaration", "function_expression", "arrow_function":
		// Look for the function name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	case "class_declaration":
		// Look for the class name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	case "variable_declarator":
		// Look for the variable name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	}

	return p.findFirstIdentifier(node, content)
}

// extractPythonSymbolName extracts symbol names from Python AST nodes
func (p *ASTParser) extractPythonSymbolName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	switch nodeType {
	case "function_definition":
		// Look for the function name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	case "class_definition":
		// Look for the class name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	}

	return p.findFirstIdentifier(node, content)
}

// extractJavaSymbolName extracts symbol names from Java AST nodes
func (p *ASTParser) extractJavaSymbolName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	switch nodeType {
	case "method_declaration", "constructor_declaration":
		// Look for the method/constructor name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	case "class_declaration", "interface_declaration":
		// Look for the class/interface name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	}

	return p.findFirstIdentifier(node, content)
}

// extractCppSymbolName extracts symbol names from C++ AST nodes
func (p *ASTParser) extractCppSymbolName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	switch nodeType {
	case "function_definition", "function_declarator":
		// Look for the function name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	case "class_specifier", "struct_specifier":
		// Look for the class/struct name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "identifier" {
				return p.getNodeText(child, content)
			}
		}
	}

	return p.findFirstIdentifier(node, content)
}

// findFirstIdentifier finds the first identifier in a node tree
func (p *ASTParser) findFirstIdentifier(node *sitter.Node, content string) string {
	if node == nil {
		return ""
	}

	if strings.Contains(node.Type(), "identifier") {
		return p.getNodeText(node, content)
	}

	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			if result := p.findFirstIdentifier(child, content); result != "" {
				return result
			}
		}
	}

	return ""
}

// extractDocComment extracts documentation comment before a symbol
func (p *ASTParser) extractDocComment(content string, symbolStartLine int) string {
	lines := strings.Split(content, "\n")
	if symbolStartLine <= 1 {
		return ""
	}

	var docLines []string
	// Look backwards from the symbol line for comments
	for i := symbolStartLine - 2; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Check for various comment styles
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "/*") || strings.Contains(line, "/**") ||
			strings.HasPrefix(line, "\"\"\"") || strings.HasPrefix(line, "'''") {
			docLines = append([]string{line}, docLines...)
		} else {
			break
		}
	}

	return strings.Join(docLines, "\n")
}

// extractDependencies extracts function calls and dependencies from within a symbol
func (p *ASTParser) extractDependencies(node *sitter.Node, content string, isRootFunction bool) []Dependency {
	var dependencies []Dependency

	p.walkNodeForDependencies(node, content, &dependencies, isRootFunction, true)

	// Filter out standard library calls and basic operations
	return p.filterDependencies(dependencies)
}

// filterDependencies filters out standard library calls and basic operations
func (p *ASTParser) filterDependencies(dependencies []Dependency) []Dependency {
	var filtered []Dependency

	for _, dep := range dependencies {
		if p.shouldIncludeDependency(dep) {
			filtered = append(filtered, dep)
		}
	}

	return filtered
}

// shouldIncludeDependency determines if a dependency should be included
func (p *ASTParser) shouldIncludeDependency(dep Dependency) bool {
	// Skip empty names
	if dep.Name == "" {
		return false
	}

	// Skip standard library calls
	if p.isStandardLibraryCall(dep.Name) {
		return false
	}

	// Skip basic operations and common patterns
	if p.isBasicOperation(dep.Name) {
		return false
	}

	// Skip self-references and common patterns
	if p.isSelfReference(dep.Name) {
		return false
	}

	return true
}

// isStandardLibraryCall checks if a call is to a standard library function
func (p *ASTParser) isStandardLibraryCall(name string) bool {
	// Common standard library patterns
	stdLibPatterns := []string{
		"std::", "java.", "System.", "String.", "Integer.", "List.", "Map.", "Set.",
		"os.", "sys.", "json.", "time.", "datetime.", "re.", "collections.",
		"fmt.", "strings.", "strconv.", "io.", "net.", "http.", "encoding.",
		"console.", "Math.", "Array.", "Object.", "JSON.", "Date.",
		"print", "println", "printf", "sprintf", "fprintf",
		"len", "cap", "make", "new", "append", "copy",
		"len", "str", "int", "float", "bool", "list", "dict", "set",
		"toString", "equals", "hashCode", "compareTo",
		"substring", "indexOf", "contains", "startsWith", "endsWith",
		"toLowerCase", "toUpperCase", "trim", "split", "join",
		"push", "pop", "shift", "unshift", "slice", "splice",
		"add", "remove", "get", "set", "put", "getOrDefault",
	}

	for _, pattern := range stdLibPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}

	return false
}

// isBasicOperation checks if a call is a basic operation
func (p *ASTParser) isBasicOperation(name string) bool {
	basicOps := []string{
		"+", "-", "*", "/", "%", "=", "==", "!=", "<", ">", "<=", ">=",
		"&&", "||", "!", "&", "|", "^", "<<", ">>",
		"++", "--", "+=", "-=", "*=", "/=", "%=",
		"->", ".", "::", "[]", "()", "{}",
	}

	for _, op := range basicOps {
		if name == op {
			return true
		}
	}

	return false
}

// isSelfReference checks if a call is a self-reference
func (p *ASTParser) isSelfReference(name string) bool {
	selfRefs := []string{
		"this", "self", "super", "base", "me", "current",
		"this.", "self.", "super.", "base.", "me.", "current.",
	}

	for _, ref := range selfRefs {
		if strings.Contains(name, ref) {
			return true
		}
	}

	return false
}

// walkNodeForDependencies recursively walks the AST to find function calls
func (p *ASTParser) walkNodeForDependencies(node *sitter.Node, content string, dependencies *[]Dependency, isRootFunction, isFirstNode bool) {
	nodeType := node.Type()

	// Check if this node represents a function call
	if strings.Contains(nodeType, "call") {
		call := p.extractDependency(node, content)
		if call.Name != "" && p.isSignificantCall(call.Name) {
			*dependencies = append(*dependencies, call)
		}
	}

	// Only include declarations/definitions if they're not the root function
	if !isRootFunction && !isFirstNode && (strings.Contains(nodeType, "declaration") || strings.Contains(nodeType, "definition")) {
		declaration := p.extractDependency(node, content)
		if declaration.Name != "" && p.isSignificantDeclaration(declaration.Name) {
			*dependencies = append(*dependencies, declaration)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			p.walkNodeForDependencies(child, content, dependencies, isRootFunction, false)
		}
	}
}

// isSignificantCall determines if a function call is significant enough to include
func (p *ASTParser) isSignificantCall(name string) bool {
	// Skip very short names (likely operators or basic operations)
	if len(name) <= 2 {
		return false
	}

	// Skip common patterns that are not meaningful dependencies
	skipPatterns := []string{
		"get", "set", "add", "remove", "find", "create", "update", "delete",
		"toString", "equals", "hashCode", "clone", "copy", "clear",
		"size", "length", "count", "empty", "isEmpty", "hasNext",
		"next", "previous", "first", "last", "begin", "end",
	}

	for _, pattern := range skipPatterns {
		if strings.EqualFold(name, pattern) {
			return false
		}
	}

	return true
}

// isSignificantDeclaration determines if a declaration is significant enough to include
func (p *ASTParser) isSignificantDeclaration(name string) bool {
	// Skip very short names
	if len(name) <= 2 {
		return false
	}

	// Skip common variable names that are not meaningful
	skipNames := []string{
		"i", "j", "k", "x", "y", "z", "a", "b", "c", "n", "m",
		"temp", "tmp", "var", "val", "item", "obj", "data",
		"result", "res", "ret", "value", "val", "item", "element",
	}

	for _, skipName := range skipNames {
		if strings.EqualFold(name, skipName) {
			return false
		}
	}

	return true
}

// extractFunctionName extracts the function name from a call expression
func (p *ASTParser) extractFunctionName(node *sitter.Node, content string) string {
	nodeType := node.Type()

	// Handle different types of call expressions
	switch {
	case strings.Contains(nodeType, "call"):
		return p.extractCallExpressionName(node, content)
	case strings.Contains(nodeType, "declaration"):
		return p.extractDeclarationName(node, content)
	case strings.Contains(nodeType, "definition"):
		return p.extractDefinitionName(node, content)
	default:
		return p.findFirstIdentifier(node, content)
	}
}

// extractDependency extracts a Dependency struct from an AST node
func (p *ASTParser) extractDependency(node *sitter.Node, content string) Dependency {
	dep := Dependency{
		Snippet: p.getNodeText(node, content),
		Line:    int(node.StartPoint().Row) + 1,
		Type:    getSymbolType(node.Type()),
	}

	// Extract the function/symbol name based on the node type
	dep.Name = p.extractFunctionName(node, content)

	return dep
}

// extractCallExpressionName extracts the function name from a call expression
func (p *ASTParser) extractCallExpressionName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())

	// First, try to get the full call expression text and parse it
	fullCallText := p.getNodeText(node, content)
	if extractedName := p.parseCallFromText(fullCallText); extractedName != "" {
		return extractedName
	}

	// Look for the function name in the call expression
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()

		// Function name is usually in the first child or in a specific pattern
		if strings.Contains(childType, "identifier") ||
			strings.Contains(childType, "name") ||
			strings.Contains(childType, "function") ||
			strings.Contains(childType, "expression") {
			name := p.getNodeText(child, content)
			if name != "" && !strings.Contains(name, "(") {
				return name
			}
		}

		// For method calls, look for the method name
		if strings.Contains(childType, "member") ||
			strings.Contains(childType, "field") ||
			strings.Contains(childType, "selector") { // Go selector expressions
			name := p.extractMethodName(child, content)
			if name != "" {
				return name
			}

			// Try to get the full qualified name (receiver.method)
			if qualifiedName := p.getNodeText(child, content); qualifiedName != "" {
				return qualifiedName
			}
		}

		// Recursively check children for complex expressions
		if strings.Contains(childType, "expression") {
			if childName := p.extractCallExpressionName(child, content); childName != "" {
				return childName
			}
		}
	}

	return ""
}

// parseCallFromText extracts function name from call text using simple parsing
func (p *ASTParser) parseCallFromText(callText string) string {
	if callText == "" {
		return ""
	}

	// Remove whitespace and newlines
	callText = strings.ReplaceAll(callText, "\n", " ")
	callText = strings.TrimSpace(callText)

	// Look for patterns like "receiver.method(" or "function("
	if parenIndex := strings.Index(callText, "("); parenIndex > 0 {
		functionPart := strings.TrimSpace(callText[:parenIndex])

		// Handle qualified calls (receiver.method)
		if dotIndex := strings.LastIndex(functionPart, "."); dotIndex > 0 {
			receiver := strings.TrimSpace(functionPart[:dotIndex])
			method := strings.TrimSpace(functionPart[dotIndex+1:])
			if receiver != "" && method != "" {
				return receiver + "." + method
			}
		}

		// Return the function name
		return functionPart
	}

	return ""
}

// extractMethodName extracts the method name from a member expression
func (p *ASTParser) extractMethodName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())

	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") || strings.Contains(childType, "name") {
			return p.getNodeText(child, content)
		}
	}

	return ""
}

// extractDeclarationName extracts the name from a declaration
func (p *ASTParser) extractDeclarationName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())

	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") ||
			strings.Contains(childType, "name") ||
			strings.Contains(childType, "variable") {
			return p.getNodeText(child, content)
		}
	}

	return ""
}

// extractDefinitionName extracts the name from a definition
func (p *ASTParser) extractDefinitionName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())

	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") ||
			strings.Contains(childType, "name") ||
			strings.Contains(childType, "function") ||
			strings.Contains(childType, "class") {
			return p.getNodeText(child, content)
		}
	}

	return ""
}

// getNodeText extracts the text content of a node
func (p *ASTParser) getNodeText(node *sitter.Node, content string) string {
	startByte := int(node.StartByte())
	endByte := int(node.EndByte())

	contentBytes := []byte(content)
	if startByte >= 0 && endByte <= len(contentBytes) && startByte < endByte {
		return string(contentBytes[startByte:endByte])
	}

	return ""
}

// findAllSymbolsInFile finds all symbols in a file
func (p *ASTParser) FindAllSymbolsInFile(ctx context.Context, filePath, content string) ([]AffectedSymbol, error) {
	rootNode, err := p.GetFileAST(ctx, filePath, content)
	if err != nil {
		return nil, err
	}

	var symbols []AffectedSymbol
	p.walkASTForSymbols(rootNode, filePath, content, &symbols)

	return symbols, nil
}

// walkASTForSymbols walks the AST to find all symbol definitions
func (p *ASTParser) walkASTForSymbols(node *sitter.Node, filePath, content string, symbols *[]AffectedSymbol) {
	if p.IsSymbolNode(node.Type()) {
		symbol := p.ExtractSymbolFromNode(node, filePath, content)
		if symbol.Name != "" {
			*symbols = append(*symbols, symbol)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			p.walkASTForSymbols(child, filePath, content, symbols)
		}
	}
}

func getSymbolType(nodeType string) SymbolType {
	// First, check for exact matches
	switch nodeType {
	case "function_declaration", "function_definition", "function_expression", "arrow_function":
		return SymbolTypeFunction
	case "method_declaration", "method_definition":
		return SymbolTypeMethod
	case "class_declaration", "class_definition", "class_specifier":
		return SymbolTypeClass
	case "interface_declaration", "interface_type":
		return SymbolTypeInterface
	case "struct_declaration", "struct_specifier", "struct_type":
		return SymbolTypeStruct
	case "enum_declaration", "enum_specifier":
		return SymbolTypeEnum
	case "variable_declaration", "var_declaration", "const_declaration":
		return SymbolTypeVariable
	case "import_statement", "import_declaration":
		return SymbolTypeImport
	case "package_declaration":
		return SymbolTypePackage
	}

	// Check for patterns
	switch {
	case strings.Contains(nodeType, "function"):
		return SymbolTypeFunction
	case strings.Contains(nodeType, "method"):
		return SymbolTypeMethod
	case strings.Contains(nodeType, "class"):
		return SymbolTypeClass
	case strings.Contains(nodeType, "interface"):
		return SymbolTypeInterface
	case strings.Contains(nodeType, "struct"):
		return SymbolTypeStruct
	case strings.Contains(nodeType, "enum"):
		return SymbolTypeEnum
	case strings.Contains(nodeType, "var") || strings.Contains(nodeType, "variable"):
		return SymbolTypeVariable
	case strings.Contains(nodeType, "const"):
		return SymbolTypeConstant
	case strings.Contains(nodeType, "type"):
		return SymbolTypeType
	case strings.Contains(nodeType, "import"):
		return SymbolTypeImport
	case strings.Contains(nodeType, "package"):
		return SymbolTypePackage
	case strings.Contains(nodeType, "call"):
		return SymbolTypeFunction // Function calls
	case strings.Contains(nodeType, "declaration"):
		// Default to variable for generic declarations
		return SymbolTypeVariable
	default:
		return SymbolType(nodeType)
	}
}
