package reviewer

import (
	"context"
	"slices"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/logze/v2"
)

// scoreAndFilterComments scores review comments and filters out low-quality ones
func (s *Reviewer) scoreAndFilterComments(ctx context.Context, comments []*model.ReviewAIComment, change *model.FileDiff, log logze.Logger) []*model.ReviewAIComment {
	if len(comments) == 0 {
		return comments
	}
	if s.cfg.Scoring.Mode == ScoringModeEverything {
		log.DebugIf(s.cfg.Verbose, "scoring mode is everything, returning all comments")
		return comments
	}

	filteredComments := make([]*model.ReviewAIComment, 0, len(comments))
	for _, comment := range comments {
		log = log.With("issue_type", comment.IssueType,
			"issue_impact", comment.IssueImpact,
			"fix_priority", comment.FixPriority,
			"model_confidence", comment.ModelConfidence,
			"line", comment.Line)

		if !slices.Contains(s.cfg.Scoring.IssueTypes, comment.IssueType) {
			log.DebugIf(s.cfg.Verbose, "skipping: not allowed issue type")
			continue
		}
		if !slices.Contains(s.cfg.Scoring.IssueImpacts, comment.IssueImpact) {
			log.DebugIf(s.cfg.Verbose, "skipping: not allowed issue impact")
			continue
		}
		if !slices.Contains(s.cfg.Scoring.FixPriorities, comment.FixPriority) {
			log.DebugIf(s.cfg.Verbose, "skipping: not allowed fix priority")
			continue
		}
		if !slices.Contains(s.cfg.Scoring.ModelConfidences, comment.ModelConfidence) {
			log.DebugIf(s.cfg.Verbose, "skipping: not allowed model confidence")
			continue
		}

		filteredComments = append(filteredComments, comment)
	}

	return filteredComments
}
