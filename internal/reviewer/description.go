package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

const (
	startMarker = "<!-- ai-desc-start -->"
	endMarker   = "<!-- ai-desc-end -->"
)

func (s *Reviewer) createDescription(ctx context.Context, request model.ReviewRequest, changes []*model.FileDiff, totalSize int64, log logze.Logger) error {
	if !s.cfg.EnableDescriptionGeneration {
		log.Info("description generation is disabled, skipping")
		return nil
	}

	log.Info("generating description")

	var fullDiff strings.Builder
	fullDiff.Grow(int(totalSize) + 30)
	for _, change := range changes {
		fullDiff.WriteString("--- a/")
		fullDiff.WriteString(change.OldPath)
		fullDiff.WriteString("\n+++ b/")
		fullDiff.WriteString(change.NewPath)
		fullDiff.WriteString("\n")
		fullDiff.WriteString(change.Diff)
		fullDiff.WriteString("\n\n")
	}

	description, err := s.agent.GenerateDescription(ctx, fullDiff.String())
	if err != nil {
		return errm.Wrap(err, "failed to generate description")
	}
	if description == "" {
		return errEmptyDescription
	}

	// Update description with changes section
	newDescription := s.updateDescriptionWithAISection(request.MergeRequest.Description, description)

	// Update MR description
	err = s.provider.UpdateMergeRequestDescription(ctx, request.ProjectID, request.MergeRequest.IID, newDescription)
	if err != nil {
		return errm.Wrap(err, "failed to update MR description")
	}

	log.Info("description created and updated")

	return nil
}

// updateDescriptionWithAISection updates MR description with AI section
func (s *Reviewer) updateDescriptionWithAISection(currentDescription, newAIDescription string) string {
	var (
		startPos = strings.Index(currentDescription, startMarker)
		endPos   int

		description = strings.Builder{}
	)

	if startPos != -1 {
		endPos = strings.Index(currentDescription, endMarker) + len(endMarker)
	}

	// Check if AI section already exists in current description
	if startPos != -1 && endPos != -1 {

		description.Grow(len(currentDescription[:startPos]) + len(currentDescription[endPos:]) + len(newAIDescription) + len(startMarker) + len(endMarker) + 20)

		// Build new description with existing content before AI section
		description.WriteString(currentDescription[:startPos])
		description.WriteString(startMarker)
		description.WriteString("\n")
		description.WriteString(newAIDescription)
		description.WriteString("\n")
		description.WriteString(endMarker)

		// Add remaining content after AI section if any
		if endPos < len(currentDescription) {
			description.WriteString(currentDescription[endPos:])
		}

		return description.String()
	}

	description.Grow(len(currentDescription) + len(newAIDescription) + len(startMarker) + len(endMarker) + 20)

	description.WriteString(startMarker)
	description.WriteString("\n")
	description.WriteString(newAIDescription)
	description.WriteString("\n")
	description.WriteString(endMarker)

	if currentDescription == "" {
		return description.String()
	}

	description.WriteString("\n\n---\n\n")
	description.WriteString(currentDescription)

	return description.String()
}
