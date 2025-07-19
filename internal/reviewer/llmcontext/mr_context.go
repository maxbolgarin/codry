package llmcontext

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/logze/v2"
)

const (
	startMarkerDesc = "<!-- Codry: ai-desc-start -->"
	endMarkerDesc   = "<!-- Codry: ai-desc-end -->"

	startMarkerOverview = "<!-- Codry: ai-overview-start -->"
	endMarkerOverview   = "<!-- Codry: ai-overview-end -->"

	startMarkerArchitecture = "<!-- Codry: ai-architecture-start -->"
	endMarkerArchitecture   = "<!-- Codry: ai-architecture-end -->"

	startMarkerCodeReview = "<!-- Codry: ai-code-review-start -->"
	endMarkerCodeReview   = "<!-- Codry: ai-code-review-end -->"
)

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
func gatherMRContext(projectID string, data *repoDataProvider) (*MRContext, error) {
	log := logze.With("project_id", projectID, "mr_iid", data.mr.IID)

	mrContext := &MRContext{
		IID:          data.mr.IID,
		IIDStr:       strconv.Itoa(data.mr.IID),
		SourceBranch: data.mr.SourceBranch,
		TargetBranch: data.mr.TargetBranch,
		SHA:          data.mr.SHA,
		Title:        data.mr.Title,
		BranchName:   data.mr.SourceBranch,
		Author:       data.mr.Author,
		CreatedAt:    data.mr.CreatedAt,
		UpdatedAt:    data.mr.UpdatedAt,
		FileDiffs:    data.diffs,
	}

	// Step 1: Process description and filter AI content
	if err := processDescription(data.mr.Description, mrContext); err != nil {
		log.Warn("failed to process description", "error", err)
		mrContext.Description = data.mr.Description
	}

	// Step 3: Gather author comments
	if err := gatherAuthorComments(data, mrContext); err != nil {
		log.Warn("failed to gather author comments", "error", err)
	}

	// Step 4: Gather commit information
	if err := gatherCommitInfo(data, mrContext, log); err != nil {
		log.Warn("failed to gather commit info", "error", err)
	}

	// Step 5: Extract linked issues and tickets
	if err := extractLinkedReferences(mrContext); err != nil {
		log.Warn("failed to extract linked references", "error", err)
	}

	// Step 6: Calculate metadata statistics
	calculateMetadata(mrContext)

	return mrContext, nil
}

// processDescription filters out AI-generated content using markers
func processDescription(originalDesc string, mrContext *MRContext) error {
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

// gatherAuthorComments collects all comments made by the MR author
func gatherAuthorComments(data *repoDataProvider, mrContext *MRContext) error {

	var authorComments []*model.Comment

commentsCycle:
	for _, comment := range data.allComments {
		if comment.Author.Username == data.mr.Author.Username {
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
func gatherCommitInfo(data *repoDataProvider, mrContext *MRContext, log logze.Logger) error {

	// Convert provider commits to our internal format with file changes
	for _, commit := range data.commits {
		commitInfo := processCommit(commit, data.commitsDiff)
		mrContext.Commits = append(mrContext.Commits, *commitInfo)
	}

	return nil
}

// processCommit converts a provider commit to our internal format with file changes
func processCommit(commit *model.Commit, commitDiff []*model.FileDiff) *CommitInfo {

	// Convert file diffs to our commit file change	s format
	fileChanges := make(map[string]CommitFileChange)
	for _, fileDiff := range commitDiff {
		status := determineFileStatus(fileDiff)

		// Parse the diff to count additions/deletions
		additions, deletions := parseDiffStats(fileDiff.Diff)

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

	return commitInfo
}

// determineFileStatus determines the status of a file change
func determineFileStatus(fileDiff *model.FileDiff) string {
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
func parseDiffStats(diff string) (additions, deletions int) {
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
func generateCommitSubject(mr *model.MergeRequest, fileChanges map[string]CommitFileChange) string {
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
func generateCommitBody(mr *model.MergeRequest, fileCount int) string {
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
func extractLinkedReferences(mrContext *MRContext) error {
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
	mrContext.LinkedIssues = extractIssueReferences(textToAnalyze)

	// Extract Jira tickets
	mrContext.LinkedTickets = extractTicketReferences(textToAnalyze)

	return nil
}

// extractIssueReferences finds GitHub/GitLab issue references
func extractIssueReferences(text string) []LinkedIssue {
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
func extractTicketReferences(text string) []LinkedTicket {
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
func calculateMetadata(mrContext *MRContext) {
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

// GetMostChangedFiles returns files sorted by total lines changed (additions + deletions)
func (mrContext *MRContext) getMostChangedFiles(limit int) []struct {
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
