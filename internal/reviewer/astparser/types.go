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
