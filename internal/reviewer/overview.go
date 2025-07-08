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
	if !s.cfg.EnableChangesOverviewGeneration {
		bundle.log.InfoIf(s.cfg.Verbose, "changes overview generation is disabled, skipping")
		return
	}
	bundle.log.DebugIf(s.cfg.Verbose, "generating changes overview")

	err := s.createChangesOverview(ctx, bundle.request, bundle.fullDiffString)
	if err != nil {
		msg := "failed to generate changes overview"
		bundle.log.Error(msg, "error", err)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	bundle.log.InfoIf(s.cfg.Verbose, "generated and created changes overview comment")

	bundle.result.IsChangesOverviewCreated = true
}

func (s *Reviewer) createChangesOverview(ctx context.Context, request model.ReviewRequest, fullDiff string) error {
	changes, err := s.agent.GenerateChangesOverview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate changes overview")
	}

	// Update description with changes section
	newComment := s.createCommentWithChangesOverview(changes, request.Changes)

	// Update MR description
	err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, newComment)
	if err != nil {
		return errm.Wrap(err, "failed to create comment")
	}

	return nil
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

	var parts []string
	if stats.plusLines > 0 {
		parts = append(parts, fmt.Sprintf("+%d", stats.plusLines))
	}
	if stats.minusLines > 0 {
		parts = append(parts, fmt.Sprintf("-%d", stats.minusLines))
	}

	return strings.Join(parts, " ")
}
