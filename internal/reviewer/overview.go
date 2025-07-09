package reviewer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
)

func (s *Reviewer) generateChangesOverview(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.Generate.ChangesOverview {
		s.logFlow("changes overview generation is disabled, skipping")
		return
	}
	s.logFlow("generating changes overview")

	err := s.createOrUpdateChangesOverview(ctx, bundle.request, bundle.fullDiffString)
	if err != nil {
		msg := "failed to generate changes overview"
		bundle.log.Err(err, msg)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	s.logFlow("generated and updated changes overview comment")

	bundle.result.IsChangesOverviewCreated = true
}

func (s *Reviewer) createOrUpdateChangesOverview(ctx context.Context, request model.ReviewRequest, fullDiff string) error {
	changes, err := s.agent.GenerateChangesOverview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate changes overview")
	}

	// Create the new comment content
	newComment := s.createCommentWithChangesOverview(changes, request.Changes)

	// Wrap the overview content with markers
	wrappedContent := s.wrapOverviewContent(newComment.Body)

	// Check for existing changes overview comment
	existingComment, err := s.findExistingChangesOverviewComment(ctx, request.ProjectID, request.MergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to check for existing changes overview comment")
	}

	if existingComment != nil {
		// Update existing comment
		err = s.provider.UpdateComment(ctx, request.ProjectID, request.MergeRequest.IID, existingComment.ID, wrappedContent)
		if err != nil {
			return errm.Wrap(err, "failed to update existing changes overview comment")
		}
	} else {
		// Create new comment with wrapped content
		newComment.Body = wrappedContent
		err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, newComment)
		if err != nil {
			return errm.Wrap(err, "failed to create comment")
		}
	}

	return nil
}

// wrapOverviewContent wraps the overview content with markers
func (s *Reviewer) wrapOverviewContent(content string) string {
	var result strings.Builder
	result.Grow(len(content) + len(startMarkerOverview) + len(endMarkerOverview) + 4)

	result.WriteString(startMarkerOverview)
	result.WriteString("\n")
	result.WriteString(content)
	result.WriteString("\n")
	result.WriteString(endMarkerOverview)

	return result.String()
}

// findExistingChangesOverviewComment finds an existing changes overview comment by the bot
func (s *Reviewer) findExistingChangesOverviewComment(ctx context.Context, projectID string, mrIID int) (*model.Comment, error) {
	comments, err := s.provider.GetComments(ctx, projectID, mrIID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comments")
	}

	for _, comment := range comments {
		if s.isChangesOverviewComment(comment.Body) {
			return comment, nil
		}
	}

	return nil, nil
}

// isChangesOverviewComment checks if a comment body contains overview markers
func (s *Reviewer) isChangesOverviewComment(body string) bool {
	return strings.Contains(body, startMarkerOverview) && strings.Contains(body, endMarkerOverview)
}

func (s *Reviewer) createCommentWithChangesOverview(files []model.FileChangeInfo, changes []*model.FileDiff) *model.Comment {
	reviewHeaders := prompts.DefaultLanguages[s.cfg.Language].ListOfChangesHeaders

	slices.SortFunc(files, func(a, b model.FileChangeInfo) int {
		return a.Type.Compare(b.Type)
	})

	changesMap := make(map[string]string)
	for _, change := range changes {
		changesMap[change.NewPath] = change.Diff
	}

	comment := strings.Builder{}
	comment.WriteString("## ")
	comment.WriteString(reviewHeaders.Title)
	comment.WriteString("\n\n")
	comment.WriteString(reviewHeaders.TableHeader)
	comment.WriteString("\n|---|---|---|---|\n")

	for _, file := range files {
		// Count plus and minus lines for this file
		diffStats := countDiffLines(lang.Check(changesMap[file.FilePath], file.Diff))
		diffStatsStr := formatDiffStats(diffStats)

		comment.WriteString("| **")
		comment.WriteString(file.FilePath)
		comment.WriteString("** | ")
		comment.WriteString(reviewHeaders.GetByType(file.Type))
		comment.WriteString(" | *")
		comment.WriteString(diffStatsStr)
		comment.WriteString("* | ")
		comment.WriteString(file.Description)
		comment.WriteString("|\n")
	}

	body := comment.String()

	return &model.Comment{
		Body: body,
		Type: model.CommentTypeGeneral,
	}
}

// diffStats represents the statistics of a diff
type diffStats struct {
	plusLines  int
	minusLines int
}

// countDiffLines counts the plus and minus lines in a diff string
func countDiffLines(diff string) diffStats {
	stats := diffStats{}

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.plusLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.minusLines++
		}
	}

	return stats
}

// formatDiffStats formats the diff statistics for display
func formatDiffStats(stats diffStats) string {
	if stats.plusLines == 0 && stats.minusLines == 0 {
		return "No changes"
	}

	result := ""
	if stats.plusLines > 0 {
		result += fmt.Sprintf("+%d", stats.plusLines)
	}
	if stats.minusLines > 0 {
		if result != "" {
			result += "/"
		}
		result += fmt.Sprintf("-%d", stats.minusLines)
	}

	return result
}
