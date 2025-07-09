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
	Line    int `json:"line"`
	EndLine int `json:"end_line,omitempty"`

	IssueType       IssueType       `json:"issue_type"`
	IssueImpact     IssueImpact     `json:"issue_impact"`
	FixPriority     FixPriority     `json:"fix_priority"`
	ModelConfidence ModelConfidence `json:"model_confidence"`

	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
	CodeSnippet string `json:"code_snippet,omitempty"`

	// Set in code
	FilePath     string `json:"-"`
	OldLine      int    `json:"-"`
	Position     int    `json:"-"`
	CodeLanguage string `json:"-"`
}

// IsRangeComment returns true if this comment spans mul	tiple lines
func (lrc *ReviewAIComment) IsRangeComment() bool {
	return lrc.EndLine > 0 && lrc.EndLine > lrc.Line
}

// IssueType categorizes the type of issue found
type IssueType string

const (
	IssueTypeFailure     IssueType = "failure"
	IssueTypeBug         IssueType = "bug"
	IssueTypeSecurity    IssueType = "security"
	IssueTypePerformance IssueType = "performance"
	IssueTypeRefactor    IssueType = "refactor"
	IssueTypeIdea        IssueType = "idea"
	IssueTypeBadPractice IssueType = "bad_practice"
	IssueTypeOther       IssueType = "other"
)

// IssueImpact defines the impact level of review issues by AI
type IssueImpact string

const (
	IssueImpactCritical IssueImpact = "critical"
	IssueImpactHigh     IssueImpact = "high"
	IssueImpactMedium   IssueImpact = "medium"
	IssueImpactLow      IssueImpact = "low"
)

// ModelConfidence defines the confidence level of review issues by AI
type ModelConfidence string

const (
	ModelConfidenceVeryHigh ModelConfidence = "very_high"
	ModelConfidenceHigh     ModelConfidence = "high"
	ModelConfidenceMedium   ModelConfidence = "medium"
	ModelConfidenceLow      ModelConfidence = "low"
)

// FixPriority defines the priority level of review issues by AI
type FixPriority string

const (
	FixPriorityHotfix  FixPriority = "hotfix"
	FixPriorityFirst   FixPriority = "first"
	FixPrioritySecond  FixPriority = "second"
	FixPriorityBacklog FixPriority = "backlog"
)

// FileChangeType represents the type of change in a file
type FileChangeType string

const (
	FileChangeTypeNewFeature FileChangeType = "new_feature"
	FileChangeTypeBugFix     FileChangeType = "bug_fix"
	FileChangeTypeRefactor   FileChangeType = "refactor"
	FileChangeTypeTest       FileChangeType = "test"
	FileChangeTypeConfig     FileChangeType = "config"
	FileChangeTypeDeploy     FileChangeType = "deploy"
	FileChangeTypeDocs       FileChangeType = "docs"
	FileChangeTypeCleanup    FileChangeType = "cleanup"
	FileChangeTypeStyle      FileChangeType = "style"
	FileChangeTypeOther      FileChangeType = "other"
)

var fileChangeTypePriority = abstract.NewSafeMap(map[FileChangeType]int{
	FileChangeTypeNewFeature: 1,
	FileChangeTypeBugFix:     2,
	FileChangeTypeRefactor:   3,
	FileChangeTypeTest:       4,
	FileChangeTypeConfig:     5,
	FileChangeTypeDeploy:     6,
	FileChangeTypeDocs:       7,
	FileChangeTypeCleanup:    8,
	FileChangeTypeStyle:      9,
	FileChangeTypeOther:      10,
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
