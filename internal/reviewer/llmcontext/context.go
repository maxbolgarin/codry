package llmcontext

import (
	"fmt"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
)

// ContextBundle represents the final structured context for LLM
type ContextBundle struct {
	Files     []*astparser.FileContext `json:"files"`
	MRContext *MRContext               `json:"mr_context"`
}

// MRContext holds comprehensive metadata about a merge request
type MRContext struct {
	// Basic MR information
	Title       string    `json:"title"`
	Description string    `json:"description"`
	BranchName  string    `json:"branch_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Author information
	Author         model.User       `json:"author"`
	AuthorComments []*model.Comment `json:"author_comments"`

	// Commit information
	Commits []CommitInfo `json:"commits"`

	// Issue/ticket links
	LinkedIssues  []LinkedIssue  `json:"linked_issues"`
	LinkedTickets []LinkedTicket `json:"linked_tickets"`

	// File changes
	FileDiffs []*model.FileDiff       `json:"file_diffs"`
	FilesStat map[string]FileDiffInfo `json:"files_stat"`

	// Context metadata
	TotalCommits   int `json:"total_commits"`
	TotalFiles     int `json:"total_files"`
	TotalAdditions int `json:"total_additions"`
	TotalDeletions int `json:"total_deletions"`
}

type FileDiffInfo struct {
	TotalAdditions int `json:"total_additions"`
	TotalDeletions int `json:"total_deletions"`
}

// CommitInfo contains detailed commit information
type CommitInfo struct {
	SHA         string                      `json:"sha"`
	Subject     string                      `json:"subject"`
	Body        string                      `json:"body"`
	Author      string                      `json:"author"`
	Timestamp   time.Time                   `json:"timestamp"`
	FileChanges map[string]CommitFileChange `json:"file_changes"`
	TotalFiles  int                         `json:"total_files"`
}

// CommitFileChange represents file changes in a specific commit
type CommitFileChange struct {
	Status    string `json:"status"`    // added, modified, deleted, renamed
	Additions int    `json:"additions"` // lines added in this file
	Deletions int    `json:"deletions"` // lines deleted in this file
	OldPath   string `json:"old_path"`  // for renamed files
	NewPath   string `json:"new_path"`  // current file path
	IsBinary  bool   `json:"is_binary"` // whether file is binary
}

// LinkedIssue represents a linked GitHub/GitLab issue
type LinkedIssue struct {
	ID          string   `json:"id"`
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	URL         string   `json:"url"`
	Labels      []string `json:"labels"`
}

// LinkedTicket represents a linked Jira/external ticket
type LinkedTicket struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	URL         string `json:"url"`
	Type        string `json:"type"`
}

// BuildContextSummary creates a structured summary of the MR context for use in prompts
func (mrContext *MRContext) BuildContextSummary() string {
	var summary strings.Builder
	summary.Grow(2000) // Pre-allocate reasonable capacity

	summary.WriteString("# MERGE REQUEST CONTEXT\n\n")

	// Basic MR information
	summary.WriteString("## Basic Information\n")
	summary.WriteString("- Title: ")
	summary.WriteString(mrContext.Title)
	summary.WriteString("\n- Branch: `")
	summary.WriteString(mrContext.BranchName)
	summary.WriteString("`\n- Author: ")
	summary.WriteString(mrContext.Author.Name)
	summary.WriteString(" (@")
	summary.WriteString(mrContext.Author.Username)
	summary.WriteString(")\n")

	// Original description (filtered from AI content)
	if mrContext.Description != "" {
		summary.WriteString("- Original Description:\n```\n")
		summary.WriteString(mrContext.Description)
		summary.WriteString("\n```\n")
	}

	// Statistics
	summary.WriteString("\n## Change Statistics\n")
	summary.WriteString("Total Changes: +")
	summary.WriteString(intToString(mrContext.TotalAdditions))
	summary.WriteString(" additions, -")
	summary.WriteString(intToString(mrContext.TotalDeletions))
	summary.WriteString(" deletions across ")
	summary.WriteString(intToString(mrContext.TotalFiles))
	summary.WriteString(" files\n")

	// Show top changed files
	mostChanged := mrContext.getMostChangedFiles(5)
	if len(mostChanged) > 0 {
		summary.WriteString("Most Changed Files:\n")
		for _, file := range mostChanged {
			summary.WriteString("  - ")
			summary.WriteString(file.FilePath)
			summary.WriteString(": +")
			summary.WriteString(intToString(file.Additions))
			summary.WriteString(", -")
			summary.WriteString(intToString(file.Deletions))
			summary.WriteString(" (")
			summary.WriteString(intToString(file.TotalChanges))
			summary.WriteString(" total)\n")
		}
	}
	summary.WriteString("\n## Commits: ")
	summary.WriteString(intToString(mrContext.TotalCommits))
	summary.WriteString(" commits\n")
	for i, commit := range mrContext.Commits {
		// Commit header with short SHA
		summary.WriteString(fmt.Sprintf("%d. ", i+1))
		summary.WriteString(commit.Subject)
		summary.WriteString(" (@") // Author
		summary.WriteString(commit.Author)
		summary.WriteString(")\nTime: ")
		summary.WriteString(commit.Timestamp.Format(time.RFC3339))
		summary.WriteString("\nFiles: ")
		summary.WriteString(intToString(commit.TotalFiles))
		summary.WriteString("\n")

		// File changes in this commit
		if len(commit.FileChanges) > 0 {
			for _, change := range sortByTotalChanges(commit.FileChanges) {
				filePath := change.NewPath
				if filePath == "" {
					filePath = change.OldPath
				}

				summary.WriteString("  - ")
				summary.WriteString(filePath)
				summary.WriteString(": [")
				summary.WriteString(change.Status)
				summary.WriteString("]: +")
				summary.WriteString(intToString(change.Additions))
				summary.WriteString(", -")
				summary.WriteString(intToString(change.Deletions))

				// Show rename information
				if change.Status == "renamed" && change.OldPath != "" && change.OldPath != change.NewPath {
					summary.WriteString(" (from ")
					summary.WriteString(change.OldPath)
					summary.WriteString(")")
				}

				// Binary file indicator
				if change.IsBinary {
					summary.WriteString(" [binary]")
				}

				summary.WriteString("\n")
			}
		}

		// Commit body if available and not too long
		if commit.Body != "" && len(commit.Body) <= 200 {
			summary.WriteString("  Description: ")
			summary.WriteString(commit.Body)
			summary.WriteString("\n")
		} else if commit.Body != "" {
			summary.WriteString("  Description: ")
			summary.WriteString(commit.Body[:197])
			summary.WriteString("...\n")
		}

		summary.WriteString("\n")
	}

	// Linked issues and tickets
	if len(mrContext.LinkedIssues) > 0 || len(mrContext.LinkedTickets) > 0 {
		summary.WriteString("\n## Linked References\n")

		if len(mrContext.LinkedIssues) > 0 {
			summary.WriteString("- Issues: ")
			for i, issue := range mrContext.LinkedIssues {
				if i > 0 {
					summary.WriteString(", ")
				}
				summary.WriteString("#")
				summary.WriteString(intToString(issue.Number))
			}
			summary.WriteString("\n")
		}

		if len(mrContext.LinkedTickets) > 0 {
			summary.WriteString("- Tickets: ")
			for i, ticket := range mrContext.LinkedTickets {
				if i > 0 {
					summary.WriteString(", ")
				}
				summary.WriteString(ticket.Key)
			}
			summary.WriteString("\n")
		}
	}

	// Author comments (provide context about author's intentions)
	if len(mrContext.AuthorComments) > 0 {
		summary.WriteString("\n## Author Comments & Context\n")
		for i, comment := range mrContext.AuthorComments {
			if i >= 3 {
				summary.WriteString("- ... and ")
				summary.WriteString(intToString(len(mrContext.AuthorComments) - 3))
				summary.WriteString(" more comments\n")
				break
			}
			summary.WriteString("- ")
			// Truncate long comments
			commentBody := comment.Body
			if len(commentBody) > 150 {
				commentBody = commentBody[:150] + "..."
			}
			summary.WriteString(commentBody)
			summary.WriteString("\n")
		}
	}

	return summary.String()
}
