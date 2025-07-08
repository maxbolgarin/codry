package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
)

func (s *Reviewer) generateDescription(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.EnableDescriptionGeneration {
		bundle.log.InfoIf(s.cfg.Verbose, "description generation is disabled, skipping")
		return
	}
	bundle.log.DebugIf(s.cfg.Verbose, "generating description")

	if err := s.createDescription(ctx, bundle.request, bundle.fullDiffString); err != nil {
		msg := "failed to generate description"
		bundle.log.Error(msg, "error", err)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	bundle.log.InfoIf(s.cfg.Verbose, "generated and updated description")

	bundle.result.IsDescriptionCreated = true
}

func (s *Reviewer) createDescription(ctx context.Context, request model.ReviewRequest, fullDiff string) error {
	description, err := s.agent.GenerateDescription(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate description")
	}
	if description == "" {
		return errm.New("empty description")
	}

	// Update description with changes section
	newDescription := s.updateDescriptionWithAISection(request.MergeRequest.Description, description)

	// Update MR description
	err = s.provider.UpdateMergeRequestDescription(ctx, request.ProjectID, request.MergeRequest.IID, newDescription)
	if err != nil {
		return errm.Wrap(err, "failed to update MR description")
	}

	return nil
}

// updateDescriptionWithAISection updates MR description with AI section
func (s *Reviewer) updateDescriptionWithAISection(currentDescription, newAIDescription string) string {
	var (
		startPos = strings.Index(currentDescription, startMarkerDesc)
		endPos   int

		description = strings.Builder{}
	)

	if startPos != -1 {
		endPos = strings.Index(currentDescription, endMarkerDesc) + len(endMarkerDesc)
	}

	// Check if AI section already exists in current description
	if startPos != -1 && endPos != -1 {

		description.Grow(len(currentDescription[:startPos]) + len(currentDescription[endPos:]) + len(newAIDescription) + len(startMarkerDesc) + len(endMarkerDesc) + 20)

		// Build new description with existing content before AI section
		description.WriteString(currentDescription[:startPos])
		description.WriteString(startMarkerDesc)
		description.WriteString("\n")
		description.WriteString(newAIDescription)
		description.WriteString("\n")
		description.WriteString(endMarkerDesc)

		// Add remaining content after AI section if any
		if endPos < len(currentDescription) {
			description.WriteString(currentDescription[endPos:])
		}

		return description.String()
	}

	description.Grow(len(currentDescription) + len(newAIDescription) + len(startMarkerDesc) + len(endMarkerDesc) + 20)

	description.WriteString(startMarkerDesc)
	description.WriteString("\n")
	description.WriteString(newAIDescription)
	description.WriteString("\n")
	description.WriteString(endMarkerDesc)

	if currentDescription == "" {
		return description.String()
	}

	description.WriteString("\n\n---\n\n")
	description.WriteString(currentDescription)

	return description.String()
}
