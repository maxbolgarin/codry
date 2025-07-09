package reviewer

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// ContextManager gathers comprehensive metadata about merge requests
type ContextManager struct {
	provider interfaces.CodeProvider
	log      logze.Logger
}

// NewContextManager creates a new context manager
func NewContextManager(provider interfaces.CodeProvider) *ContextManager {
	return &ContextManager{
		provider: provider,
		log:      logze.With("component", "context_manager"),
	}
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

var aiMarkers = []struct {
	start string
	end   string
}{
	{startMarkerDesc, endMarkerDesc},
	{startMarkerOverview, endMarkerOverview},
	{startMarkerArchitecture, endMarkerArchitecture},
	{startMarkerCodeReview, endMarkerCodeReview},
}

// GatherMRContext collects comprehensive metadata about a merge request
func (cm *ContextManager) GatherMRContext(ctx context.Context, projectID string, mr *model.MergeRequest) (*MRContext, error) {
	log := cm.log.WithFields("project_id", projectID, "mr_iid", mr.IID)

	mrContext := &MRContext{
		Title:      mr.Title,
		BranchName: mr.SourceBranch,
		Author:     mr.Author,
		CreatedAt:  mr.CreatedAt,
		UpdatedAt:  mr.UpdatedAt,
	}

	// Step 1: Process description and filter AI content
	if err := cm.processDescription(mr.Description, mrContext); err != nil {
		log.Warn("failed to process description", "error", err)
		mrContext.Description = mr.Description // Fallback to original
	}

	// Step 2: Gather file diffs
	if err := cm.gatherFileDiffs(ctx, projectID, mr.IID, mrContext); err != nil {
		return nil, errm.Wrap(err, "failed to gather file diffs")
	}

	// Step 3: Gather author comments
	if err := cm.gatherAuthorComments(ctx, projectID, mr.IID, mr.Author.Username, mrContext); err != nil {
		log.Warn("failed to gather author comments", "error", err)
	}

	// Step 4: Gather commit information
	if err := cm.gatherCommitInfo(ctx, projectID, mr, mrContext); err != nil {
		log.Warn("failed to gather commit info", "error", err)
	}

	// Step 5: Extract linked issues and tickets
	if err := cm.extractLinkedReferences(mrContext); err != nil {
		log.Warn("failed to extract linked references", "error", err)
	}

	// Step 6: Calculate metadata statistics
	cm.calculateMetadata(mrContext)

	return mrContext, nil
}

// processDescription filters out AI-generated content using markers
func (cm *ContextManager) processDescription(originalDesc string, mrContext *MRContext) error {
	if originalDesc == "" {
		mrContext.Description = ""
		return nil
	}

	filteredDesc := originalDesc

	// Remove AI-generated sections
	for _, marker := range aiMarkers {
		if strings.Contains(filteredDesc, marker.start) {
			// Remove content between markers
			re := regexp.MustCompile(regexp.QuoteMeta(marker.start) + `[\s\S]*?` + regexp.QuoteMeta(marker.end))
			filteredDesc = re.ReplaceAllString(filteredDesc, "")
		}
	}

	// Clean up extra whitespace
	filteredDesc = strings.TrimSpace(filteredDesc)
	filteredDesc = regexp.MustCompile(`\n\s*\n\s*\n`).ReplaceAllString(filteredDesc, "\n\n")

	mrContext.Description = filteredDesc

	return nil
}

// gatherFileDiffs collects all file diffs for the MR
func (cm *ContextManager) gatherFileDiffs(ctx context.Context, projectID string, mrIID int, mrContext *MRContext) error {
	diffs, err := cm.provider.GetMergeRequestDiffs(ctx, projectID, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get MR diffs")
	}

	mrContext.FileDiffs = diffs
	return nil
}

// gatherAuthorComments collects all comments made by the MR author
func (cm *ContextManager) gatherAuthorComments(ctx context.Context, projectID string, mrIID int, authorUsername string, mrContext *MRContext) error {
	allComments, err := cm.provider.GetComments(ctx, projectID, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get comments")
	}

	var authorComments []*model.Comment

commentsCycle:
	for _, comment := range allComments {
		if comment.Author.Username == authorUsername {
			for _, marker := range aiMarkers {
				if strings.Contains(comment.Body, marker.start) {
					continue commentsCycle
				}
			}
			authorComments = append(authorComments, comment)
		}
	}

	mrContext.AuthorComments = authorComments
	return nil
}

// gatherCommitInfo collects detailed commit information using provider APIs
func (cm *ContextManager) gatherCommitInfo(ctx context.Context, projectID string, mr *model.MergeRequest, mrContext *MRContext) error {
	log := cm.log.WithFields("project_id", projectID, "mr_iid", mr.IID)

	// Get all commits for this MR/PR
	commits, err := cm.provider.GetMergeRequestCommits(ctx, projectID, mr.IID)
	if err != nil {
		log.Warn("failed to get MR commits, falling back to single commit approach", "error", err)
		return cm.gatherCommitInfoFallback(mr, mrContext)
	}

	log.Debug("retrieved commits from provider", "commit_count", len(commits))

	// Convert provider commits to our internal format with file changes
	for _, commit := range commits {
		commitInfo, err := cm.processCommit(ctx, projectID, commit, mrContext)
		if err != nil {
			log.Warn("failed to process commit, skipping", "commit_sha", commit.SHA, "error", err)
			continue
		}

		mrContext.Commits = append(mrContext.Commits, *commitInfo)
	}

	// If no commits were successfully processed, fall back to the old method
	if len(mrContext.Commits) == 0 {
		log.Warn("no commits were successfully processed, using fallback")
		return cm.gatherCommitInfoFallback(mr, mrContext)
	}

	return nil
}

// processCommit converts a provider commit to our internal format with file changes
func (cm *ContextManager) processCommit(ctx context.Context, projectID string, commit *model.Commit, mrContext *MRContext) (*CommitInfo, error) {
	// Get file changes for this specific commit
	fileDiffs, err := cm.provider.GetCommitDiffs(ctx, projectID, commit.SHA)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get commit diffs")
	}

	// Convert file diffs to our commit file changes format
	fileChanges := make(map[string]CommitFileChange)
	for _, fileDiff := range fileDiffs {
		status := cm.determineFileStatus(fileDiff)

		// Parse the diff to count additions/deletions
		additions, deletions := cm.parseDiffStats(fileDiff.Diff)

		fileChange := CommitFileChange{
			Status:    status,
			Additions: additions,
			Deletions: deletions,
			OldPath:   fileDiff.OldPath,
			NewPath:   fileDiff.NewPath,
			IsBinary:  fileDiff.IsBinary,
		}

		// Use new path as key, fallback to old path for deleted files
		key := fileDiff.NewPath
		if key == "" {
			key = fileDiff.OldPath
		}

		fileChanges[key] = fileChange
	}

	// Create our internal commit info
	commitInfo := &CommitInfo{
		SHA:         commit.SHA,
		Subject:     commit.Subject,
		Body:        commit.Body,
		Author:      commit.Author.Name,
		Timestamp:   commit.Timestamp,
		FileChanges: fileChanges,
		TotalFiles:  len(fileChanges),
	}

	return commitInfo, nil
}

// gatherCommitInfoFallback is the fallback method when provider APIs fail
func (cm *ContextManager) gatherCommitInfoFallback(mr *model.MergeRequest, mrContext *MRContext) error {
	if mr.SHA != "" {
		// Create a commit entry representing the current state of the MR
		fileChanges := make(map[string]CommitFileChange)

		// Map file diffs to commit file changes
		for _, fileDiff := range mrContext.FileDiffs {
			status := cm.determineFileStatus(fileDiff)

			// Parse the diff to count additions/deletions
			additions, deletions := cm.parseDiffStats(fileDiff.Diff)

			fileChange := CommitFileChange{
				Status:    status,
				Additions: additions,
				Deletions: deletions,
				OldPath:   fileDiff.OldPath,
				NewPath:   fileDiff.NewPath,
				IsBinary:  fileDiff.IsBinary,
			}

			// Use new path as key, fallback to old path for deleted files
			key := fileDiff.NewPath
			if key == "" {
				key = fileDiff.OldPath
			}

			fileChanges[key] = fileChange
		}

		commit := CommitInfo{
			SHA:         mr.SHA,
			Subject:     cm.generateCommitSubject(mr, fileChanges),
			Body:        cm.generateCommitBody(mr, len(fileChanges)),
			Author:      mr.Author.Name,
			Timestamp:   mr.UpdatedAt,
			FileChanges: fileChanges,
			TotalFiles:  len(fileChanges),
		}

		mrContext.Commits = append(mrContext.Commits, commit)
	}

	return nil
}

// determineFileStatus determines the status of a file change
func (cm *ContextManager) determineFileStatus(fileDiff *model.FileDiff) string {
	if fileDiff.IsNew {
		return "added"
	}
	if fileDiff.IsDeleted {
		return "deleted"
	}
	if fileDiff.IsRenamed {
		return "renamed"
	}
	return "modified"
}

// parseDiffStats parses a diff string to count additions and deletions
func (cm *ContextManager) parseDiffStats(diff string) (additions, deletions int) {
	if diff == "" {
		return 0, 0
	}

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	return additions, deletions
}

// generateCommitSubject creates a commit subject based on MR info
func (cm *ContextManager) generateCommitSubject(mr *model.MergeRequest, fileChanges map[string]CommitFileChange) string {
	// Try to extract meaningful subject from MR title or use default
	if mr.Title != "" {
		// Truncate title if too long for commit subject
		title := mr.Title
		if len(title) > 72 {
			title = title[:69] + "..."
		}
		return title
	}

	// Fallback: describe the changes
	fileCount := len(fileChanges)
	if fileCount == 1 {
		for filename := range fileChanges {
			return "Update " + filename
		}
	}

	return fmt.Sprintf("Update %d files", fileCount)
}

// generateCommitBody creates a commit body with change summary
func (cm *ContextManager) generateCommitBody(mr *model.MergeRequest, fileCount int) string {
	var body strings.Builder

	if mr.Description != "" && len(mr.Description) < 500 {
		body.WriteString(mr.Description)
		body.WriteString("\n\n")
	}

	body.WriteString("Changes from MR: ")
	body.WriteString(mr.Title)
	body.WriteString("\n")
	body.WriteString("Files modified: ")
	body.WriteString(intToString(fileCount))
	body.WriteString("\n")
	body.WriteString("Branch: ")
	body.WriteString(mr.SourceBranch)
	body.WriteString(" -> ")
	body.WriteString(mr.TargetBranch)

	return body.String()
}

// extractLinkedReferences extracts linked issues and tickets from title, description, and commits
func (cm *ContextManager) extractLinkedReferences(mrContext *MRContext) error {
	textToAnalyze := strings.Join([]string{
		mrContext.Title,
		mrContext.Description,
		mrContext.BranchName,
	}, " ")

	// Add commit messages to analysis
	for _, commit := range mrContext.Commits {
		textToAnalyze += " " + commit.Subject + " " + commit.Body
	}

	// Extract GitHub/GitLab issues
	mrContext.LinkedIssues = cm.extractIssueReferences(textToAnalyze)

	// Extract Jira tickets
	mrContext.LinkedTickets = cm.extractTicketReferences(textToAnalyze)

	return nil
}

// extractIssueReferences finds GitHub/GitLab issue references
func (cm *ContextManager) extractIssueReferences(text string) []LinkedIssue {
	var issues []LinkedIssue

	// Common patterns for issue references
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`#(\d+)`),                      // #123
		regexp.MustCompile(`(?i)issue[s]?\s*#?(\d+)`),     // issue 123, issues #123
		regexp.MustCompile(`(?i)fix(?:es)?\s+#?(\d+)`),    // fixes #123
		regexp.MustCompile(`(?i)close[s]?\s+#?(\d+)`),     // closes #123
		regexp.MustCompile(`(?i)resolve[s]?\s+#?(\d+)`),   // resolves #123
		regexp.MustCompile(`(?i)implement[s]?\s+#?(\d+)`), // implements #123
	}

	issueNumbers := make(map[int]bool)

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				if num := parseInt(match[1]); num > 0 {
					issueNumbers[num] = true
				}
			}
		}
	}

	// Convert to LinkedIssue structs (would be enriched with actual API calls)
	for num := range issueNumbers {
		issues = append(issues, LinkedIssue{
			ID:          "",
			Number:      num,
			Title:       "",
			Description: "",
			State:       "unknown",
			URL:         "",
			Labels:      []string{},
		})
	}

	return issues
}

// extractTicketReferences finds Jira and other ticket references
func (cm *ContextManager) extractTicketReferences(text string) []LinkedTicket {
	var tickets []LinkedTicket

	// Common patterns for ticket references
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`([A-Z]{2,}-\d+)`),                 // PROJ-123
		regexp.MustCompile(`(?i)jira[:\s]+([A-Z]{2,}-\d+)`),   // JIRA: PROJ-123
		regexp.MustCompile(`(?i)ticket[:\s]+([A-Z]{2,}-\d+)`), // Ticket: PROJ-123
		regexp.MustCompile(`(?i)story[:\s]+([A-Z]{2,}-\d+)`),  // Story: PROJ-123
		regexp.MustCompile(`(?i)task[:\s]+([A-Z]{2,}-\d+)`),   // Task: PROJ-123
	}

	ticketKeys := make(map[string]bool)

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				key := strings.ToUpper(match[1])
				ticketKeys[key] = true
			}
		}
	}

	// Convert to LinkedTicket structs (would be enriched with actual API calls)
	for key := range ticketKeys {
		tickets = append(tickets, LinkedTicket{
			ID:          "",
			Key:         key,
			Title:       "",
			Description: "",
			Status:      "unknown",
			URL:         "",
			Type:        "unknown",
		})
	}

	return tickets
}

// calculateMetadata computes statistics about the MR
func (cm *ContextManager) calculateMetadata(mrContext *MRContext) {
	mrContext.TotalCommits = len(mrContext.Commits)
	mrContext.TotalFiles = len(mrContext.FileDiffs)
	mrContext.FilesStat = make(map[string]FileDiffInfo, len(mrContext.FileDiffs))

	// Calculate additions and deletions from diffs
	for _, diff := range mrContext.FileDiffs {
		stat := FileDiffInfo{}

		for line := range strings.SplitSeq(diff.Diff, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				stat.TotalAdditions++
				mrContext.TotalAdditions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				stat.TotalDeletions++
				mrContext.TotalDeletions++
			}
		}

		mrContext.FilesStat[diff.NewPath] = stat
	}
}

// Helper function to safely parse integers
func parseInt(s string) int {
	result := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			result = result*10 + int(r-'0')
		} else {
			return 0
		}
	}
	return result
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
	mostChanged := mrContext.GetMostChangedFiles(5)
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

// GetFileChangeHistory returns a map of file paths to their change history across commits
func (mrContext *MRContext) GetFileChangeHistory() map[string][]CommitFileChange {
	fileHistory := make(map[string][]CommitFileChange)

	for _, commit := range mrContext.Commits {
		for filePath, change := range commit.FileChanges {
			fileHistory[filePath] = append(fileHistory[filePath], change)
		}
	}

	return fileHistory
}

// GetMostChangedFiles returns files sorted by total lines changed (additions + deletions)
func (mrContext *MRContext) GetMostChangedFiles(limit int) []struct {
	FilePath     string
	TotalChanges int
	Additions    int
	Deletions    int
} {
	type fileChange struct {
		FilePath     string
		TotalChanges int
		Additions    int
		Deletions    int
	}

	fileStats := make(map[string]*fileChange)

	// Aggregate changes per file across all commits
	for _, commit := range mrContext.Commits {
		for filePath, change := range commit.FileChanges {
			if _, exists := fileStats[filePath]; !exists {
				fileStats[filePath] = &fileChange{FilePath: filePath}
			}
			fileStats[filePath].Additions += change.Additions
			fileStats[filePath].Deletions += change.Deletions
			fileStats[filePath].TotalChanges += change.Additions + change.Deletions
		}
	}

	// Convert to slice for sorting
	var files []struct {
		FilePath     string
		TotalChanges int
		Additions    int
		Deletions    int
	}

	for _, stat := range fileStats {
		files = append(files, struct {
			FilePath     string
			TotalChanges int
			Additions    int
			Deletions    int
		}{
			FilePath:     stat.FilePath,
			TotalChanges: stat.TotalChanges,
			Additions:    stat.Additions,
			Deletions:    stat.Deletions,
		})
	}

	// Simple bubble sort by TotalChanges (descending)
	for i := 0; i < len(files)-1; i++ {
		for j := 0; j < len(files)-i-1; j++ {
			if files[j].TotalChanges < files[j+1].TotalChanges {
				files[j], files[j+1] = files[j+1], files[j]
			}
		}
	}

	// Apply limit
	if limit > 0 && limit < len(files) {
		files = files[:limit]
	}

	return files
}

// intToString converts int to string without imports
func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	// Reverse digits
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

func sortByTotalChanges(changes map[string]CommitFileChange) []CommitFileChange {
	var sortedChanges []CommitFileChange
	for _, change := range changes {
		sortedChanges = append(sortedChanges, change)
	}
	sort.Slice(sortedChanges, func(i, j int) bool {
		return sortedChanges[i].Additions+sortedChanges[i].Deletions > sortedChanges[j].Additions+sortedChanges[j].Deletions
	})
	return sortedChanges
}
