package reviewer

import (
	"context"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
)

func (s *Reviewer) generateArchitectureReview(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.EnableArchitectureReview {
		bundle.log.InfoIf(s.cfg.Verbose, "architecture review is disabled, skipping")
		return
	}

	bundle.log.DebugIf(s.cfg.Verbose, "generating architecture review")

	err := s.createArchitectureReview(ctx, bundle.request, bundle.fullDiffString)
	if err != nil {
		msg := "failed to generate architecture review"
		bundle.log.Error(msg, "error", err)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	bundle.log.InfoIf(s.cfg.Verbose, "generated and created architecture review comment")

	bundle.result.IsArchitectureReviewCreated = true
}

func (s *Reviewer) createArchitectureReview(ctx context.Context, request model.ReviewRequest, fullDiff string) error {
	architectureResult, err := s.agent.GenerateArchitectureReview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate architecture review")
	}

	comment := &model.Comment{
		Body: architectureResult,
		Type: model.CommentTypeGeneral,
	}

	// Create comment on MR
	err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
	if err != nil {
		return errm.Wrap(err, "failed to create architecture review comment")
	}

	return nil
}
