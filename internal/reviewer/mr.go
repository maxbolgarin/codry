package reviewer

import (
	"context"
	"os"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

// GetAndReviewMergeRequest gets a merge request by ID and reviews it
func (s *Reviewer) GetAndReviewMergeRequest(ctx context.Context, projectID string, mrIID int) error {
	mr, err := s.provider.GetMergeRequest(ctx, projectID, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get merge request")
	}
	return s.ReviewMergeRequest(ctx, projectID, mr)
}

// ReviewMergeRequest handles merge request related events
func (s *Reviewer) ReviewMergeRequest(ctx context.Context, projectID string, mergeRequest *model.MergeRequest) error {
	if mergeRequest == nil {
		return errm.New("merge request is nil")
	}

	diffs, err := s.provider.GetMergeRequestDiffs(ctx, projectID, mergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to get merge request diffs")
	}

	// Create review request
	reviewRequest := model.ReviewRequest{
		ProjectID:    projectID,
		MergeRequest: mergeRequest,
		Changes:      diffs,
	}

	result, err := s.processMergeRequestReview(ctx, reviewRequest)
	if err != nil {
		return errm.Wrap(err, "failed to process merge request")
	}

	s.logProcessingResults(result, s.log)

	return nil
}

// ProcessMergeRequest processes a merge request for the first time
func (s *Reviewer) processMergeRequestReview(ctx context.Context, request model.ReviewRequest) (model.ReviewResult, error) {
	log := s.log.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.MergeRequest.IID,
		"mr_from", request.MergeRequest.SourceBranch,
		"mr_to", request.MergeRequest.TargetBranch,
		"mr_author", request.MergeRequest.Author.Username,
		"commit_sha", lang.TruncateString(request.MergeRequest.SHA, 8),
	)

	log.Info("starting merge request review")

	result := model.ReviewResult{}

	// Filter files for review
	filesToReview, totalSize := s.filterFilesForReview(request, log)
	if len(filesToReview) == 0 {
		result.Success = true
		return result, nil
	}

	var fullDiff strings.Builder
	fullDiff.Grow(int(totalSize) + 30)
	for _, change := range filesToReview {
		fullDiff.WriteString("--- a/")
		fullDiff.WriteString(change.OldPath)
		fullDiff.WriteString("\n+++ b/")
		fullDiff.WriteString(change.NewPath)
		fullDiff.WriteString("\n")
		fullDiff.WriteString(change.Diff)
		fullDiff.WriteString("\n\n")
	}

	err := s.createDescription(ctx, request, fullDiff.String(), log)
	if err != nil {
		log.Error("failed to generate description", "error", err)
		result.Errors = append(result.Errors, errm.Wrap(err, "failed to generate description"))
	} else {
		result.DescriptionUpdated = true
	}

	err = s.createChangesOverview(ctx, request, fullDiff.String(), log)
	if err != nil {
		log.Error("failed to generate changes overview", "error", err)
		result.Errors = append(result.Errors, errm.Wrap(err, "failed to generate changes overview"))
	} else {
		result.ChangesOverviewUpdated = true
	}

	os.Exit(1)

	commentsCreated, err := s.reviewCodeChanges(ctx, request, filesToReview, log)
	if err != nil {
		result.Errors = append(result.Errors, errm.Wrap(err, "failed to review code changes"))
	} else {
		result.CommentsCreated = commentsCreated
	}

	log.Info("processing completed")

	result.ProcessedFiles = len(filesToReview)
	result.Success = len(result.Errors) == 0

	return result, nil
}

func (s *Reviewer) filterFilesForReview(request model.ReviewRequest, log logze.Logger) ([]*model.FileDiff, int64) {
	var filtered []*model.FileDiff

	var totalSize int64

	for _, file := range request.Changes {
		if file.IsDeleted || file.IsBinary {
			continue
		}

		if len(file.Diff) == 0 || len(file.Diff) > s.cfg.FileFilter.MaxFileSize {
			log.Debug("skipping due to size", "file", file.NewPath, "size", len(file.Diff))
			continue
		}

		if !s.isCodeFile(file.NewPath) {
			log.Debug("skipping non-code", "file", file.NewPath)
			continue
		}

		if s.isExcludedPath(file.NewPath) {
			log.Debug("skipping excluded", "file", file.NewPath)
			continue
		}

		log.Debug("adding to review", "file", file.NewPath)
		filtered = append(filtered, file)
		totalSize += int64(len(file.Diff))
		totalSize += int64(len(file.OldPath))
		totalSize += int64(len(file.NewPath))

		// Limit number of files per MR
		if len(filtered) >= s.cfg.MaxFilesPerMR {
			log.Warn("reached maximum files limit", "limit", s.cfg.MaxFilesPerMR)
			break
		}
	}

	if len(filtered) == 0 {
		log.Info("no files to review after filtering")
		return nil, 0
	}

	log.Info("found files to review",
		"total_files", len(filtered),
		"total_size", totalSize,
	)

	return filtered, totalSize
}

// logProcessingResults logs the results of MR processing
func (s *Reviewer) logProcessingResults(result model.ReviewResult, log logze.Logger) {
	if result.Success {
		log.Info("successfully processed merge request",
			"processed_files", result.ProcessedFiles,
			"comments_created", result.CommentsCreated,
			"description_updated", result.DescriptionUpdated,
		)
	} else {
		log.Error("merge request processing completed with errors",
			"error_count", len(result.Errors),
			"processed_files", result.ProcessedFiles,
		)
		for _, err := range result.Errors {
			log.Error("processing error", "error", err.Error())
		}
	}
}
