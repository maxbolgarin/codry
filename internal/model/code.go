package model

import (
	"time"
)

// ProviderConfig represents provider-specific configuration
type ProviderConfig struct {
	BaseURL       string
	Token         string
	WebhookSecret string
	BotUsername   string
}

// User represents a user across different providers
type User struct {
	ID       string
	Username string
	Name     string
}

// MergeRequest represents a merge/pull request across different providers
type MergeRequest struct {
	ID           string
	IID          int
	Title        string
	Description  string
	SourceBranch string
	TargetBranch string
	Author       User
	Reviewers    []User
	URL          string
	State        string
	SHA          string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// FileDiff represents changes in a single file
type FileDiff struct {
	OldPath     string
	NewPath     string
	Diff        string
	IsNew       bool
	IsDeleted   bool
	IsRenamed   bool
	IsBinary    bool
	ContentType string
}

// Comment represents a code review comment
type Comment struct {
	ID        string
	Body      string
	FilePath  string
	Line      int         // Line number in the new file (for line-specific comments)
	OldLine   int         // Line number in the old file (for context)
	Position  int         // Position in the diff (provider-specific)
	Type      CommentType // Type of comment
	Author    User
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CommentType defines the type of comment
type CommentType string

const (
	CommentTypeGeneral CommentType = "general" // General MR/PR comment
	CommentTypeInline  CommentType = "inline"  // Inline code comment
	CommentTypeReview  CommentType = "review"  // Review comment with specific feedback
	CommentTypeSummary CommentType = "summary" // Summary comment
)

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

// FileReviewResult represents the result of a file review
type FileReviewResult struct {
	FilePath  string             `json:"file_path"`
	Comments  []*ReviewAIComment `json:"comments"`
	Summary   string             `json:"summary,omitempty"`
	HasIssues bool               `json:"has_issues"`
}

// MergeRequestFilter represents criteria for filtering merge requests
type MergeRequestFilter struct {
	State        []string   // e.g., "open", "closed", "merged"
	AuthorID     string     // Filter by author
	TargetBranch string     // Filter by target branch
	SourceBranch string     // Filter by source branch
	UpdatedAfter *time.Time // Filter by last update time
	CreatedAfter *time.Time // Filter by creation time
	Limit        int        // Maximum number of results (0 = no limit)
	Page         int        // Page number for pagination (0-based)
}
