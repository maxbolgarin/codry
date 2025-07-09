package model

import "time"

// Commit represents a git commit
type Commit struct {
	SHA       string    `json:"sha"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Author    User      `json:"author"`
	Committer User      `json:"committer"`
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`

	// Statistics
	Stats CommitStats `json:"stats"`
}

// CommitStats represents commit statistics
type CommitStats struct {
	TotalFiles int `json:"total_files"`
	Additions  int `json:"additions"`
	Deletions  int `json:"deletions"`
}
