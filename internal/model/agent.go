package model

import (
	"time"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/lang"
)

type Language string

const (
	LanguageEnglish    Language = "en"
	LanguageRussian    Language = "ru"
	LanguageSpanish    Language = "es"
	LanguageFrench     Language = "fr"
	LanguageItalian    Language = "it"
	LanguageGerman     Language = "de"
	LanguagePortuguese Language = "pt"
	LanguageJapanese   Language = "ja"
	LanguageKorean     Language = "ko"
	LanguageChinese    Language = "zh"
)

// ModelConfig represents model-specific configuration
type ModelConfig struct {
	APIKey   string
	Model    string
	URL      string
	ProxyURL string
	IsTest   bool
}

// APIRequest represents a request to an LLM API
type APIRequest struct {
	Prompt       string
	SystemPrompt string
	MaxTokens    int
	Temperature  float32
	URL          string
	ResponseType string
}

// APIResponse represents a response from an LLM API
type APIResponse struct {
	CreateTime       time.Time
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Prompt represents a structured prompt for LLM
type Prompt struct {
	SystemPrompt string
	UserPrompt   string
	Language     Language
}

// FileReviewResult represents the result of a file review
type FileReviewResult struct {
	File      string             `json:"file"`
	Comments  []*ReviewAIComment `json:"comments"`
	HasIssues bool               `json:"has_issues"`
}

// ReviewAIComment represents a structured review comment for a specific line or range of lines
type ReviewAIComment struct {
	FilePath     string           `json:"file_path"`
	Line         int              `json:"line"`               // Start line (for single line comments)
	EndLine      int              `json:"end_line,omitempty"` // End line (for range comments, optional)
	OldLine      int              `json:"old_line,omitempty"`
	Position     int              `json:"position,omitempty"`
	IssueType    IssueType        `json:"issue_type"`
	Confidence   ReviewConfidence `json:"confidence"`
	Priority     ReviewPriority   `json:"priority"`
	CodeLanguage string           `json:"code_language"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Suggestion   string           `json:"suggestion,omitempty"`
	CodeSnippet  string           `json:"code_snippet,omitempty"`
}

// IssueScore represents a score for a code review issue
type IssueScore struct {
	// Overall score from 0.0 to 1.0 (higher = more important/relevant)
	OverallScore float64 `json:"overall_score"`

	// Severity score (0.0 = info, 1.0 = critical)
	SeverityScore float64 `json:"severity_score"`

	// Confidence score (0.0 = low confidence, 1.0 = high confidence)
	ConfidenceScore float64 `json:"confidence_score"`

	// Relevance score (0.0 = not relevant to current change, 1.0 = highly relevant)
	RelevanceScore float64 `json:"relevance_score"`

	// Actionability score (0.0 = vague feedback, 1.0 = specific actionable)
	ActionabilityScore float64 `json:"actionability_score"`

	// Whether this issue should be filtered out
	ShouldFilter bool `json:"should_filter"`

	// Reason for filtering (if ShouldFilter is true)
	FilterReason string `json:"filter_reason,omitempty"`
}

// IsRangeComment returns true if this comment spans mul	tiple lines
func (lrc *ReviewAIComment) IsRangeComment() bool {
	return lrc.EndLine > 0 && lrc.EndLine > lrc.Line
}

// IssueType categorizes the type of issue found
type IssueType string

const (
	IssueTypeCritical    IssueType = "critical"
	IssueTypeBug         IssueType = "bug"
	IssueTypePerformance IssueType = "performance"
	IssueTypeSecurity    IssueType = "security"
	IssueTypeRefactor    IssueType = "refactor"
	IssueTypeOther       IssueType = "other"
)

// ReviewConfidence defines the confidence level of review issues by AI
type ReviewConfidence string

const (
	ConfidenceVeryHigh ReviewConfidence = "very_high"
	ConfidenceHigh     ReviewConfidence = "high"
	ConfidenceMedium   ReviewConfidence = "medium"
	ConfidenceLow      ReviewConfidence = "low"
)

// ReviewPriority defines the priority level of review issues by AI
type ReviewPriority string

const (
	ReviewPriorityCritical ReviewPriority = "critical"
	ReviewPriorityHigh     ReviewPriority = "high"
	ReviewPriorityMedium   ReviewPriority = "medium"
	ReviewPriorityBacklog  ReviewPriority = "backlog"
)

// FileChangeType represents the type of change in a file
type FileChangeType string

const (
	FileChangeTypeNewFeature FileChangeType = "new_feature"
	FileChangeTypeBugFix     FileChangeType = "bug_fix"
	FileChangeTypeRefactor   FileChangeType = "refactor"
	FileChangeTypeTest       FileChangeType = "test"
	FileChangeTypeDeploy     FileChangeType = "deploy"
	FileChangeTypeDocs       FileChangeType = "docs"
	FileChangeTypeCleanup    FileChangeType = "cleanup"
	FileChangeTypeStyle      FileChangeType = "style"
	FileChangeTypeOther      FileChangeType = "other"
)

var fileChangeTypePriority = abstract.NewSafeMap[FileChangeType, int](map[FileChangeType]int{
	FileChangeTypeNewFeature: 1,
	FileChangeTypeBugFix:     2,
	FileChangeTypeRefactor:   3,
	FileChangeTypeTest:       4,
	FileChangeTypeDeploy:     5,
	FileChangeTypeDocs:       6,
	FileChangeTypeCleanup:    7,
	FileChangeTypeStyle:      8,
	FileChangeTypeOther:      9,
})

func (fct FileChangeType) Compare(other FileChangeType) int {
	return lang.If(fct == other, 0, lang.If(fileChangeTypePriority.Get(fct) < fileChangeTypePriority.Get(other), -1, 1))
}

// FileChangeInfo represents a change in a file
type FileChangeInfo struct {
	FilePath    string         `json:"file"`
	Diff        string         `json:"diff"`
	Type        FileChangeType `json:"type"`
	Description string         `json:"description"`
}
