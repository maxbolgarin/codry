package astparser

import (
	"context"
	"maps"
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
	SymbolTypeEnum      SymbolType = "enum"
	SymbolTypePackage   SymbolType = "package"
	SymbolTypeImport    SymbolType = "import"
)

// AffectedSymbol represents a code symbol that was affected by changes
type AffectedSymbol struct {
	Name       string        `json:"symbol_name"`
	Type       SymbolType    `json:"symbol_type"`
	FullCode   string        `json:"full_code"`
	DocComment string        `json:"doc_comment"`
	Context    SymbolContext `json:"context"`

	Callers      []Dependency `json:"callers"`
	Dependencies []Dependency `json:"dependencies"`

	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// SymbolContext provides context information about where the symbol is defined
type SymbolContext struct {
	ParentSymbol *AffectedSymbol  `json:"parent_symbol,omitempty"`
	ChildSymbols []AffectedSymbol `json:"child_symbols,omitempty"`
	Package      string           `json:"package"`
	Module       string           `json:"module"`
	Namespace    string           `json:"namespace"`
}

// Dependency represents a function call dependency
type Dependency struct {
	Name    string     `json:"name"`
	Snippet string     `json:"snippet"`
	Line    int        `json:"line"`
	Type    SymbolType `json:"type"`
}

// NodePosition represents a position in the source code
type NodePosition struct {
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
}

// NewParser creates a new AST parser with supported languages
func NewParser() *Parser {
	languages := make(map[ProgrammingLanguage]*sitter.Language, len(languagesParsers))
	maps.Copy(languages, languagesParsers)
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

// FindAffectedSymbols finds all symbols affected by changes in the given lines
func (p *Parser) FindAffectedSymbols(ctx context.Context, filePath, fileContent string, changedLines []int) ([]AffectedSymbol, error) {
	rootNode, err := p.ParseFileToAST(ctx, filePath, fileContent)
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
func (p *Parser) findParentSymbolNode(node *sitter.Node) *sitter.Node {
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
func (p *Parser) IsSymbolNode(nodeType string) bool {
	return symbolNodes[nodeType]
}

// ExtractSymbolFromNode extracts symbol information from an AST node
func (p *Parser) ExtractSymbolFromNode(node *sitter.Node, filePath, fileContent string) AffectedSymbol {
	symbol := AffectedSymbol{
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1, // Convert to 1-based
		EndLine:   int(node.EndPoint().Row) + 1,   // Convert to 1-based
		Name:      p.extractSymbolName(node, fileContent),
		Type:      getSymbolType(node.Type()),
	}
	symbol.DocComment = p.extractDocComment(node, fileContent, symbol.StartLine)
	symbol.Dependencies = p.extractDependencies(node, fileContent, symbol.Type == SymbolTypeFunction || symbol.Type == SymbolTypeMethod)

	lines := strings.Split(fileContent, "\n")

	if symbol.StartLine > 0 && symbol.EndLine > 0 && symbol.EndLine <= len(lines) {
		symbolLines := lines[symbol.StartLine-1 : symbol.EndLine]
		symbol.FullCode = strings.Join(symbolLines, "\n")
	}

	return symbol
}

// extractSymbolName extracts the symbol name from a symbol declaration node
func (p *Parser) extractSymbolName(node *sitter.Node, content string) string {
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") {
			return p.getNodeText(child, content)
		}
	}
	return ""
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
func (p *Parser) extractDependencies(node *sitter.Node, content string, isRootFunction bool) []Dependency {
	var dependencies []Dependency

	p.walkNodeForDependencies(node, content, &dependencies, isRootFunction, true)

	return dependencies
}

// walkNodeForDependencies recursively walks the AST to find function calls
func (p *Parser) walkNodeForDependencies(node *sitter.Node, content string, dependencies *[]Dependency, isRootFunction, isFirstNode bool) {
	nodeType := node.Type()

	// Check if this node represents a function call
	if strings.Contains(nodeType, "call") {
		call := p.extractDependency(node, content)
		if call.Name != "" {
			*dependencies = append(*dependencies, call)
		}
	}

	if !isRootFunction && !isFirstNode && (strings.Contains(nodeType, "declaration") || strings.Contains(nodeType, "definition")) {
		declaration := p.extractDependency(node, content)
		if declaration.Name != "" {
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

// extractFunctionCall extracts a function call from a call expression node
func (p *Parser) extractDependency(node *sitter.Node, content string) Dependency {
	call := Dependency{
		Snippet: p.getNodeText(node, content),
		Line:    int(node.StartPoint().Row) + 1,
		Type:    getSymbolType(node.Type()),
	}

	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}

		childType := child.Type()
		if strings.Contains(childType, "identifier") || strings.Contains(childType, "expression") {
			call.Name = p.getNodeText(child, content)
			break
		}
	}

	return call
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

// findAllSymbolsInFile finds all symbols in a file
func (p *Parser) findAllSymbolsInFile(filePath, content string) ([]AffectedSymbol, error) {
	rootNode, err := p.ParseFileToAST(context.Background(), filePath, content)
	if err != nil {
		return nil, err
	}

	var symbols []AffectedSymbol
	p.walkASTForSymbols(rootNode, filePath, content, &symbols)

	return symbols, nil
}

// walkASTForSymbols walks the AST to find all symbol definitions
func (p *Parser) walkASTForSymbols(node *sitter.Node, filePath, content string, symbols *[]AffectedSymbol) {
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
	switch {
	case strings.Contains(nodeType, "function"):
		return SymbolTypeFunction

	case strings.Contains(nodeType, "method"):
		return SymbolTypeMethod

	case strings.Contains(nodeType, "class"):
		return SymbolTypeClass

	case strings.Contains(nodeType, "interface"):
		return SymbolTypeInterface

	case strings.Contains(nodeType, "type_"):
		return SymbolTypeType

	case strings.Contains(nodeType, "struct"):
		return SymbolTypeStruct

	case strings.Contains(nodeType, "enum"):
		return SymbolTypeEnum

	case strings.Contains(nodeType, "var"):
		return SymbolTypeVariable

	case strings.Contains(nodeType, "const"):
		return SymbolTypeConstant

	case strings.Contains(nodeType, "import"):
		return SymbolTypeImport

	default:
		return SymbolType(nodeType)
	}
}
