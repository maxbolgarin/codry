package astparser

import "github.com/maxbolgarin/codry/internal/model"

// ChangeType represents the type of file change
type ChangeType string

const (
	ChangeTypeModified ChangeType = "Modified"
	ChangeTypeAdded    ChangeType = "Added"
	ChangeTypeDeleted  ChangeType = "Deleted"
	ChangeTypeRenamed  ChangeType = "Renamed"
)

// FileContext represents context for a single changed file
type FileContext struct {
	FilePath        string           `json:"file_path"`
	ChangeType      ChangeType       `json:"change_type"`
	Diff            string           `json:"diff"`
	AffectedSymbols []AffectedSymbol `json:"affected_symbols"`
	RelatedFiles    []RelatedFile    `json:"related_files"`
	ConfigContext   *ConfigContext   `json:"config_context,omitempty"`
}

// RelatedFile represents a file related to the changed file
type RelatedFile struct {
	FilePath         string `json:"file_path"`
	Relationship     string `json:"relationship"` // "caller", "dependency", "test", "same_package"
	CodeSnippet      string `json:"code_snippet"`
	Line             int    `json:"line,omitempty"`
	RelevantFunction string `json:"relevant_function,omitempty"`
}

// ConfigContext represents context for configuration file changes
type ConfigContext struct {
	ConfigType       string        `json:"config_type"` // "yaml", "json", "env", etc.
	ChangedKeys      []string      `json:"changed_keys"`
	ConsumingCode    []RelatedFile `json:"consuming_code"`
	ImpactAssessment string        `json:"impact_assessment"`
}

// SymbolUsageContext provides comprehensive context about symbol usage
type SymbolUsageContext struct {
	Callers      []Caller     `json:"callers"`
	Dependencies []Dependency `json:"dependencies"`
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

	Callers      []Caller     `json:"callers"`
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

	SourceFile    string `json:"source_file"`
	SourceCode    string `json:"source_code"`
	Documentation string `json:"documentation"`
}

type Caller struct {
	FilePath     string     `json:"file_path"`
	Name         string     `json:"name"`
	Snippet      string     `json:"snippet"`
	Type         SymbolType `json:"type"`
	Line         int        `json:"line"`
	FunctionName string     `json:"function_name"`
}

// determineChangeType determines the type of change for a file
func (cf *ContextManager) determineChangeType(fileDiff *model.FileDiff) ChangeType {
	if fileDiff.IsNew {
		return ChangeTypeAdded
	}
	if fileDiff.IsDeleted {
		return ChangeTypeDeleted
	}
	if fileDiff.IsRenamed {
		return ChangeTypeRenamed
	}
	return ChangeTypeModified
}
