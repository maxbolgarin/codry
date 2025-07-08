package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
)

func (s *Reviewer) generateArchitectureReview(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.EnableArchitectureReview {
		bundle.log.InfoIf(s.cfg.Verbose, "architecture review is disabled, skipping")
		return
	}

	bundle.log.Debug("generating architecture review")

	err := s.createOrUpdateArchitectureReview(ctx, bundle.request, bundle.fullDiffString)
	if err != nil {
		msg := "failed to generate architecture review"
		bundle.log.Err(err, msg)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	bundle.log.InfoIf(s.cfg.Verbose, "generated and updated architecture review comment")

	bundle.result.IsArchitectureReviewCreated = true
}

func (s *Reviewer) createOrUpdateArchitectureReview(ctx context.Context, request model.ReviewRequest, fullDiff string) error {
	architectureResult, err := s.agent.GenerateArchitectureReview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate architecture review")
	}

	// Wrap the architecture result with markers
	wrappedContent := s.wrapArchitectureContent(architectureResult)

	// Check for existing architecture review comment
	existingComment, err := s.findExistingArchitectureComment(ctx, request.ProjectID, request.MergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to check for existing architecture comment")
	}

	if existingComment != nil {
		// Update existing comment
		err = s.provider.UpdateComment(ctx, request.ProjectID, request.MergeRequest.IID, existingComment.ID, wrappedContent)
		if err != nil {
			return errm.Wrap(err, "failed to update existing architecture review comment")
		}
	} else {
		// Create new comment
		comment := &model.Comment{
			Body: wrappedContent,
			Type: model.CommentTypeGeneral,
		}

		err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
		if err != nil {
			return errm.Wrap(err, "failed to create architecture review comment")
		}
	}

	return nil
}

// wrapArchitectureContent wraps the architecture review content with markers
func (s *Reviewer) wrapArchitectureContent(content string) string {
	var result strings.Builder
	result.Grow(len(content) + len(startMarkerArchitecture) + len(endMarkerArchitecture) + 4)

	result.WriteString(startMarkerArchitecture)
	result.WriteString("\n")
	result.WriteString(content)
	result.WriteString("\n")
	result.WriteString(endMarkerArchitecture)

	return result.String()
}

// findExistingArchitectureComment finds an existing architecture review comment by the bot
func (s *Reviewer) findExistingArchitectureComment(ctx context.Context, projectID string, mrIID int) (*model.Comment, error) {
	comments, err := s.provider.GetComments(ctx, projectID, mrIID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comments")
	}

	for _, comment := range comments {
		if s.isArchitectureReviewComment(comment.Body) {
			return comment, nil
		}
	}

	return nil, nil
}

// isArchitectureReviewComment checks if a comment body contains architecture markers
func (s *Reviewer) isArchitectureReviewComment(body string) bool {
	return strings.Contains(body, startMarkerArchitecture) && strings.Contains(body, endMarkerArchitecture)
}
