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
	Line      int
	Author    User
	CreatedAt time.Time
	UpdatedAt time.Time
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
