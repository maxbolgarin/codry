package ast

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxbolgarin/errm"
	sitter "github.com/smacker/go-tree-sitter"
)

// Parser handles parsing code using Tree-sitter to map lines to syntax nodes
type Parser struct {
	languages map[ProgrammingLanguage]*sitter.Language
}

// SymbolType represents the type of a code symbol
type SymbolType string

const (
	SymbolTypeFunction  SymbolType = "function"
	SymbolTypeMethod    SymbolType = "method"
	SymbolTypeClass     SymbolType = "class"
	SymbolTypeStruct    SymbolType = "struct"
	SymbolTypeInterface SymbolType = "interface"
	SymbolTypeVariable  SymbolType = "variable"
	SymbolTypeConstant  SymbolType = "constant"
	SymbolTypeType      SymbolType = "type"
	SymbolTypePackage   SymbolType = "package"
	SymbolTypeImport    SymbolType = "import"
	SymbolTypeUnknown   SymbolType = "unknown"
)

// AffectedSymbol represents a code symbol that was affected by changes
type AffectedSymbol struct {
	Name         string         `json:"symbol_name"`
	Type         SymbolType     `json:"symbol_type"`
	FilePath     string         `json:"file_path"`
	StartLine    int            `json:"start_line"`
	EndLine      int            `json:"end_line"`
	FullCode     string         `json:"full_code"`
	Signature    string         `json:"signature"`
	DocComment   string         `json:"doc_comment"`
	Context      SymbolContext  `json:"context"`
	Dependencies []FunctionCall `json:"dependencies"`
	Parameters   []Parameter    `json:"parameters"`
	ReturnType   string         `json:"return_type"`
}

// SymbolContext provides context information about where the symbol is defined
type SymbolContext struct {
	ParentSymbol *AffectedSymbol  `json:"parent_symbol,omitempty"`
	ChildSymbols []AffectedSymbol `json:"child_symbols,omitempty"`
	Package      string           `json:"package"`
	Module       string           `json:"module"`
	Namespace    string           `json:"namespace"`
}

// FunctionCall represents a function call dependency
type FunctionCall struct {
	Name       string   `json:"name"`
	Module     string   `json:"module,omitempty"`
	Package    string   `json:"package,omitempty"`
	Line       int      `json:"line"`
	Arguments  []string `json:"arguments,omitempty"`
	IsExternal bool     `json:"is_external"`
}

// Parameter represents a function/method parameter
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NodePosition represents a position in the source code
type NodePosition struct {
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
}

// newASTParser creates a new AST parser with supported languages
func newASTParser() *Parser {
	languages := make(map[ProgrammingLanguage]*sitter.Language, len(languagesParsers))
	for name, parser := range languagesParsers {
		languages[name] = parser
	}
	return &Parser{
		languages: languages,
	}
}

// ParseFileToAST parses a file's content to AST using Tree-sitter
func (p *Parser) ParseFileToAST(ctx context.Context, filePath, content string) (*sitter.Node, error) {
	language := DetectProgrammingLanguage(filePath)
	languageParser, ok := p.languages[language]
	if !ok {
		return nil, errm.Errorf("unsupported file type for AST parsing: %s", language)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(languageParser)

	tree, err := parser.ParseCtx(ctx, nil, []byte(content))
	if err != nil {
		return nil, errm.Wrap(err, "failed to parse AST", "file", filePath)
	}

	return tree.RootNode(), nil
}

// FindSmallestEnclosingNode finds the smallest AST node that encloses the given line
func (p *Parser) FindSmallestEnclosingNode(rootNode *sitter.Node, lineNumber int) *sitter.Node {
	return p.findSmallestEnclosingNodeRecursive(rootNode, lineNumber)
}

// findSmallestEnclosingNodeRecursive recursively searches for the smallest enclosing node
func (p *Parser) findSmallestEnclosingNodeRecursive(node *sitter.Node, lineNumber int) *sitter.Node {
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

// findAffectedSymbols finds all symbols affected by changes in the given lines
func (p *Parser) findAffectedSymbols(ctx context.Context, filePath, content string, changedLines []int) ([]AffectedSymbol, error) {
	rootNode, err := p.ParseFileToAST(ctx, filePath, content)
	if err != nil {
		return nil, errm.Wrap(err, "failed to parse file to AST", "file", filePath)
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

		symbol := p.extractSymbolFromNode(symbolNode, filePath, content)
		if symbol.Name == "" {
			continue
		}

		// Use a unique key to avoid duplicates
		symbolKey := fmt.Sprintf("%s:%s:%d", symbol.Type, symbol.Name, symbol.StartLine)
		if !symbolsFound[symbolKey] {
			symbols = append(symbols, symbol)
			symbolsFound[symbolKey] = true
		}
	}

	return symbols, nil
}

// findParentSymbolNode finds the parent node that represents a symbol (function, class, etc.)
func (p *Parser) findParentSymbolNode(node *sitter.Node) *sitter.Node {
	current := node

	for current != nil {
		nodeType := current.Type()

		// Check if this node represents a symbol we're interested in
		if p.isSymbolNode(nodeType) {
			return current
		}

		current = current.Parent()
	}

	return nil
}

// isSymbolNode checks if a node type represents a code symbol we're interested in
func (p *Parser) isSymbolNode(nodeType string) bool {
	symbolNodes := map[string]bool{
		// Go
		"function_declaration": true,
		"method_declaration":   true,
		"type_declaration":     true,
		"var_declaration":      true,
		"const_declaration":    true,
		"interface_type":       true,
		"struct_type":          true,

		// JavaScript/TypeScript
		"function_expression":    true,
		"arrow_function":         true,
		"method_definition":      true,
		"class_declaration":      true,
		"variable_declaration":   true,
		"interface_declaration":  true,
		"type_alias_declaration": true,

		// Python
		"function_def":       true,
		"async_function_def": true,
		"class_def":          true,
		"assignment":         true,
	}

	return symbolNodes[nodeType]
}

// extractSymbolFromNode extracts symbol information from an AST node
func (p *Parser) extractSymbolFromNode(node *sitter.Node, filePath, content string) AffectedSymbol {
	symbol := AffectedSymbol{
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1, // Convert to 1-based
		EndLine:   int(node.EndPoint().Row) + 1,   // Convert to 1-based
	}

	nodeType := node.Type()
	lines := strings.Split(content, "\n")

	// Extract symbol name and type based on AST node type
	switch nodeType {
	case "function_declaration", "function_def", "async_function_def":
		symbol.Type = SymbolTypeFunction
		symbol.Name = p.extractFunctionName(node, content)
		symbol.Parameters = p.extractFunctionParameters(node, content)
		symbol.ReturnType = p.extractReturnType(node, content)

	case "method_declaration", "method_definition":
		symbol.Type = SymbolTypeMethod
		symbol.Name = p.extractFunctionName(node, content)
		symbol.Parameters = p.extractFunctionParameters(node, content)
		symbol.ReturnType = p.extractReturnType(node, content)

	case "class_declaration", "class_def":
		symbol.Type = SymbolTypeClass
		symbol.Name = p.extractClassName(node, content)

	case "type_declaration":
		symbol.Type = SymbolTypeType
		symbol.Name = p.extractTypeName(node, content)

	case "struct_type":
		symbol.Type = SymbolTypeStruct
		symbol.Name = p.extractStructName(node, content)

	case "interface_type", "interface_declaration":
		symbol.Type = SymbolTypeInterface
		symbol.Name = p.extractInterfaceName(node, content)

	case "var_declaration", "variable_declaration", "assignment":
		symbol.Type = SymbolTypeVariable
		symbol.Name = p.extractVariableName(node, content)

	case "const_declaration":
		symbol.Type = SymbolTypeConstant
		symbol.Name = p.extractConstantName(node, content)

	default:
		symbol.Type = SymbolTypeUnknown
		symbol.Name = nodeType
	}

	// Extract full code for the symbol
	if symbol.StartLine > 0 && symbol.EndLine > 0 && symbol.EndLine <= len(lines) {
		symbolLines := lines[symbol.StartLine-1 : symbol.EndLine]
		symbol.FullCode = strings.Join(symbolLines, "\n")
	}

	// Extract signature (first line or declaration line)
	if symbol.StartLine > 0 && symbol.StartLine <= len(lines) {
		symbol.Signature = strings.TrimSpace(lines[symbol.StartLine-1])
	}

	// Extract documentation comment
	symbol.DocComment = p.extractDocComment(node, content, symbol.StartLine)

	// Extract dependencies (function calls within this symbol)
	symbol.Dependencies = p.extractDependencies(node, content)

	return symbol
}

// extractFunctionName extracts the function name from a function declaration node
func (p *Parser) extractFunctionName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if childType == "identifier" || childType == "field_identifier" {
			return p.getNodeText(child, content)
		}
	}
	return ""
}

// extractClassName extracts the class name from a class declaration node
func (p *Parser) extractClassName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractTypeName extracts the type name from a type declaration node
func (p *Parser) extractTypeName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractStructName extracts the struct name from a struct declaration node
func (p *Parser) extractStructName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractInterfaceName extracts the interface name from an interface declaration node
func (p *Parser) extractInterfaceName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractVariableName extracts the variable name from a variable declaration node
func (p *Parser) extractVariableName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractConstantName extracts the constant name from a constant declaration node
func (p *Parser) extractConstantName(node *sitter.Node, content string) string {
	return p.extractFunctionName(node, content) // Same logic for most languages
}

// extractFunctionParameters extracts function parameters from a function declaration
func (p *Parser) extractFunctionParameters(node *sitter.Node, content string) []Parameter {
	var parameters []Parameter

	// Find parameter list node
	paramListNode := p.findChildByType(node, "parameter_list")
	if paramListNode == nil {
		paramListNode = p.findChildByType(node, "parameters")
	}
	if paramListNode == nil {
		return parameters
	}

	// Extract individual parameters
	childCount := int(paramListNode.ChildCount())
	for i := 0; i < childCount; i++ {
		child := paramListNode.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if childType == "parameter_declaration" || childType == "identifier" {
			param := p.extractParameter(child, content)
			if param.Name != "" {
				parameters = append(parameters, param)
			}
		}
	}

	return parameters
}

// extractParameter extracts a single parameter from a parameter node
func (p *Parser) extractParameter(node *sitter.Node, content string) Parameter {
	param := Parameter{}

	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if childType == "identifier" && param.Name == "" {
			param.Name = p.getNodeText(child, content)
		} else if childType == "type_identifier" || childType == "primitive_type" {
			param.Type = p.getNodeText(child, content)
		}
	}

	return param
}

// extractReturnType extracts the return type from a function declaration
func (p *Parser) extractReturnType(node *sitter.Node, content string) string {
	// Find return type node (varies by language)
	returnTypeNode := p.findChildByType(node, "type_identifier")
	if returnTypeNode == nil {
		returnTypeNode = p.findChildByType(node, "primitive_type")
	}
	if returnTypeNode == nil {
		return ""
	}

	return p.getNodeText(returnTypeNode, content)
}

// extractDocComment extracts documentation comment before a symbol
func (p *Parser) extractDocComment(node *sitter.Node, content string, symbolStartLine int) string {
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
func (p *Parser) extractDependencies(node *sitter.Node, content string) []FunctionCall {
	var dependencies []FunctionCall

	p.walkNodeForDependencies(node, content, &dependencies)

	return dependencies
}

// walkNodeForDependencies recursively walks the AST to find function calls
func (p *Parser) walkNodeForDependencies(node *sitter.Node, content string, dependencies *[]FunctionCall) {
	nodeType := node.Type()

	// Check if this node represents a function call
	if p.isFunctionCallNode(nodeType) {
		call := p.extractFunctionCall(node, content)
		if call.Name != "" {
			*dependencies = append(*dependencies, call)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			p.walkNodeForDependencies(child, content, dependencies)
		}
	}
}

// isFunctionCallNode checks if a node represents a function call
func (p *Parser) isFunctionCallNode(nodeType string) bool {
	callNodes := map[string]bool{
		"call_expression":   true,
		"method_invocation": true,
		"function_call":     true,
		"invocation":        true,
	}
	return callNodes[nodeType]
}

// extractFunctionCall extracts a function call from a call expression node
func (p *Parser) extractFunctionCall(node *sitter.Node, content string) FunctionCall {
	call := FunctionCall{
		Line: int(node.StartPoint().Row) + 1, // Convert to 1-based
	}

	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if childType == "identifier" || childType == "field_identifier" {
			call.Name = p.getNodeText(child, content)
			break
		} else if childType == "selector_expression" || childType == "member_expression" {
			// Handle method calls like obj.method()
			call.Name = p.getNodeText(child, content)
			break
		}
	}

	// Determine if it's an external call (simple heuristic)
	call.IsExternal = strings.Contains(call.Name, ".") &&
		!strings.HasPrefix(call.Name, "this.") &&
		!strings.HasPrefix(call.Name, "self.")

	return call
}

// Helper methods

// findChildByType finds the first child node of a specific type
func (p *Parser) findChildByType(node *sitter.Node, nodeType string) *sitter.Node {
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil && child.Type() == nodeType {
			return child
		}
	}
	return nil
}

// getNodeText extracts the text content of a node
func (p *Parser) getNodeText(node *sitter.Node, content string) string {
	startByte := int(node.StartByte())
	endByte := int(node.EndByte())

	contentBytes := []byte(content)
	if startByte >= 0 && endByte <= len(contentBytes) && startByte < endByte {
		return string(contentBytes[startByte:endByte])
	}

	return ""
}

// getNodePosition returns the position of a node in the source code
func (p *Parser) getNodePosition(node *sitter.Node) NodePosition {
	return NodePosition{
		StartLine:   int(node.StartPoint().Row) + 1,    // Convert to 1-based
		EndLine:     int(node.EndPoint().Row) + 1,      // Convert to 1-based
		StartColumn: int(node.StartPoint().Column) + 1, // Convert to 1-based
		EndColumn:   int(node.EndPoint().Column) + 1,   // Convert to 1-based
	}
}
