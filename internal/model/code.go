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
	Email    string
}

// RepositoryInfo represents repository metadata and statistics
type RepositoryInfo struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	FullName      string         `json:"full_name"`
	Description   string         `json:"description"`
	URL           string         `json:"url"`
	DefaultBranch string         `json:"default_branch"`
	Size          int64          `json:"size"` // Repository size in KB
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Branches      []BranchInfo   `json:"branches"`
	Languages     map[string]int `json:"languages"` // Language -> lines of code
}

// BranchInfo represents information about a repository branch
type BranchInfo struct {
	Name      string    `json:"name"`
	SHA       string    `json:"sha"` // Last commit SHA
	Protected bool      `json:"protected"`
	Default   bool      `json:"default"`
	UpdatedAt time.Time `json:"updated_at"` // Last commit time
}

// RepositorySnapshot represents a complete snapshot of repository files at a specific commit
type RepositorySnapshot struct {
	CommitSHA string            `json:"commit_sha"`
	Timestamp time.Time         `json:"timestamp"`
	Files     []*RepositoryFile `json:"files"`
	TotalSize int64             `json:"total_size"` // Total size in bytes
}

// RepositoryFile represents a single file in the repository snapshot
type RepositoryFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Size        int64  `json:"size"` // File size in bytes
	Mode        string `json:"mode"` // File mode (e.g., "100644")
	IsBinary    bool   `json:"is_binary"`
	ContentType string `json:"content_type"`
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
