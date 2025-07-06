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
	"github.com/maxbolgarin/logze/v2"
)

// DiffStats represents the statistics of a diff
type DiffStats struct {
	PlusLines  int
	MinusLines int
}

// countDiffLines counts the plus and minus lines in a diff string
func countDiffLines(diff string) DiffStats {
	stats := DiffStats{}

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.PlusLines++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.MinusLines++
		}
	}

	return stats
}

// formatDiffStats formats the diff statistics for display
func formatDiffStats(stats DiffStats) string {
	if stats.PlusLines == 0 && stats.MinusLines == 0 {
		return "No changes"
	}

	var parts []string
	if stats.PlusLines > 0 {
		parts = append(parts, fmt.Sprintf("+%d", stats.PlusLines))
	}
	if stats.MinusLines > 0 {
		parts = append(parts, fmt.Sprintf("-%d", stats.MinusLines))
	}

	return strings.Join(parts, " ")
}

func (s *Reviewer) createChangesOverview(ctx context.Context, request model.ReviewRequest, fullDiff string, log logze.Logger) error {
	if !s.cfg.EnableChangesOverviewGeneration {
		log.Info("changes overview generation is disabled, skipping")
		return nil
	}

	log.Info("generating changes overview")

	changes, err := s.agent.GenerateChangesOverview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate changes overview")
	}

	// Update description with changes section
	newComment := s.createCommentWithChangesOverview(changes, request.Changes)

	// Update MR description
	err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, newComment)
	if err != nil {
		return errm.Wrap(err, "failed to update MR description")
	}

	log.Info("changes overview created and updated")

	return nil
}

func (s *Reviewer) createCommentWithChangesOverview(files []model.FileChange, changes []*model.FileDiff) *model.Comment {
	reviewHeaders := prompts.DefaultLanguages[s.cfg.Language].ListOfChangesHeaders

	slices.SortFunc(files, func(a, b model.FileChange) int {
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
