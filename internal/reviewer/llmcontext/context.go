package llmcontext

import (
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
)

// ContextBundle represents the final structured context for LLM
type ContextBundle struct {
	Overview  OverviewContext         `json:"overview"`
	Files     []astparser.FileContext `json:"files"`
	Summary   SummaryContext          `json:"summary"`
	Metadata  MetadataContext         `json:"metadata"`
	MRContext *MRContext              `json:"mr_context"`
}

// OverviewContext provides high-level overview of the changes
type OverviewContext struct {
	TotalFiles        int                       `json:"total_files"`
	TotalSymbols      int                       `json:"total_symbols"`
	ImpactScore       float64                   `json:"impact_score"`
	ChangeComplexity  astparser.ComplexityLevel `json:"change_complexity"`
	HighImpactChanges []string                  `json:"high_impact_changes"`
	ConfigChanges     []ConfigChangeInfo        `json:"config_changes"`
	DeletedSymbols    []DeletedSymbolInfo       `json:"deleted_symbols"`
	PotentialIssues   []string                  `json:"potential_issues"`
}

// SummaryContext provides summary information for the LLM
type SummaryContext struct {
	ChangesSummary  string         `json:"changes_summary"`
	AffectedAreas   []string       `json:"affected_areas"`
	ReviewFocus     []string       `json:"review_focus"`
	RiskAssessment  RiskAssessment `json:"risk_assessment"`
	Recommendations []string       `json:"recommendations"`
}

// MetadataContext provides metadata about the analysis
type MetadataContext struct {
	AnalysisTimestamp  string   `json:"analysis_timestamp"`
	AnalysisVersion    string   `json:"analysis_version"`
	SupportedLanguages []string `json:"supported_languages"`
	Limitations        []string `json:"limitations"`
}

// ConfigChangeInfo represents information about configuration changes
type ConfigChangeInfo struct {
	FilePath      string   `json:"file_path"`
	ConfigType    string   `json:"config_type"`
	ChangedKeys   []string `json:"changed_keys"`
	Impact        string   `json:"impact"`
	AffectedFiles []string `json:"affected_files"`
}

// DeletedSymbolInfo represents information about deleted symbols
type DeletedSymbolInfo struct {
	Symbol           astparser.AffectedSymbol `json:"symbol"`
	BrokenReferences []astparser.RelatedFile  `json:"broken_references"`
	Impact           string                   `json:"impact"`
}

// RiskAssessment provides risk assessment for the changes
type RiskAssessment struct {
	Level       RiskLevel `json:"level"`
	Score       float64   `json:"score"`
	Factors     []string  `json:"factors"`
	Mitigations []string  `json:"mitigations"`
}

// RiskLevel represents the risk level of changes
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)
