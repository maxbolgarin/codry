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
