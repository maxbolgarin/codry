package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/abstract"
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

	s.processMergeRequestReview(ctx, model.ReviewRequest{
		ProjectID:    projectID,
		MergeRequest: mergeRequest,
		Changes:      diffs,
	})

	return nil
}

// ProcessMergeRequest processes a merge request for the first time
func (s *Reviewer) processMergeRequestReview(ctx context.Context, request model.ReviewRequest) {
	log := s.log.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.MergeRequest.IID,
		"branch_from", request.MergeRequest.SourceBranch,
		"branch_to", request.MergeRequest.TargetBranch,
		"commit_sha", lang.TruncateString(request.MergeRequest.SHA, 8),
	)
	log.Infof("starting merge request review: %s", request.MergeRequest.Title)

	reviewBundle := &reviewBundle{
		result:  &model.ReviewResult{},
		request: request,
		timer:   abstract.StartTimer(),
	}

	defer func() {
		s.logProcessingResults(*reviewBundle.result, reviewBundle.timer, s.log)
	}()

	// Filter files for review
	filesToReview, totalDiffLength := s.filterFilesForReview(request, log)
	if len(filesToReview) == 0 {
		reviewBundle.result.IsSuccess = true
		return
	}

	reviewBundle.filesToReview = filesToReview
	reviewBundle.fullDiffString = buildDiffString(filesToReview, totalDiffLength)

	s.generateDescription(ctx, reviewBundle)
	s.generateChangesOverview(ctx, reviewBundle)
	s.generateArchitectureReview(ctx, reviewBundle)
	s.generateCodeReview(ctx, reviewBundle)

	reviewBundle.result.ProcessedFiles = len(filesToReview)
	reviewBundle.result.IsSuccess = len(reviewBundle.result.Errors) == 0
}

type reviewBundle struct {
	result         *model.ReviewResult
	request        model.ReviewRequest
	filesToReview  []*model.FileDiff
	fullDiffString string
	log            logze.Logger
	timer          abstract.Timer
}

func (s *Reviewer) filterFilesForReview(request model.ReviewRequest, log logze.Logger) ([]*model.FileDiff, int64) {
	var filtered []*model.FileDiff

	var totalDiffLength int64

	for _, file := range request.Changes {
		if file.IsDeleted || file.IsBinary {
			log.DebugIf(s.cfg.Verbose, "skipping deleted or binary file", "file", file.NewPath)
			continue
		}

		if len(file.Diff) == 0 {
			log.DebugIf(s.cfg.Verbose, "skipping empty file", "file", file.NewPath)
			continue
		}

		if len(file.Diff) > s.cfg.FileFilter.MaxFileSize {
			log.DebugIf(s.cfg.Verbose, "skipping due to size", "file", file.NewPath, "size", len(file.Diff), "max_size", s.cfg.FileFilter.MaxFileSize)
			continue
		}

		if s.isExcludedPath(file.NewPath) {
			log.DebugIf(s.cfg.Verbose, "skipping excluded", "file", file.NewPath)
			continue
		}

		if !s.isCodeFile(file.NewPath) {
			log.DebugIf(s.cfg.Verbose, "skipping non-code", "file", file.NewPath)
			continue
		}

		log.DebugIf(s.cfg.Verbose, "adding to review", "file", file.NewPath)
		filtered = append(filtered, file)

		// Count diff string total size
		totalDiffLength += int64(len(file.Diff))
		totalDiffLength += int64(len(file.OldPath))
		totalDiffLength += int64(len(file.NewPath))

		// Limit number of files per MR
		if len(filtered) >= s.cfg.MaxFilesPerMR {
			log.Warn("reached maximum files limit", "limit", s.cfg.MaxFilesPerMR)
			break
		}
	}

	if len(filtered) == 0 {
		log.InfoIf(s.cfg.Verbose, "no files to review after filtering")
		return nil, 0
	}

	log.InfoIf(s.cfg.Verbose, "found files to review",
		"total_files", len(filtered),
		"diff_length", totalDiffLength,
	)

	return filtered, totalDiffLength
}

func buildDiffString(files []*model.FileDiff, totalDiffLength int64) string {
	var fullDiff strings.Builder
	fullDiff.Grow(int(totalDiffLength) + 30)
	for _, change := range files {
		fullDiff.WriteString("--- a/")
		fullDiff.WriteString(change.OldPath)
		fullDiff.WriteString("\n+++ b/")
		fullDiff.WriteString(change.NewPath)
		fullDiff.WriteString("\n")
		fullDiff.WriteString(change.Diff)
		fullDiff.WriteString("\n\n")
	}
	return fullDiff.String()
}

// logProcessingResults logs the results of MR processing
func (s *Reviewer) logProcessingResults(result model.ReviewResult, timer abstract.Timer, log logze.Logger) {
	log = log.WithFields(
		"description", result.IsDescriptionCreated,
		"changes_overview", result.IsChangesOverviewCreated,
		"architecture_review", result.IsArchitectureReviewCreated,
		"code_review", result.IsCodeReviewCreated,
		"processed_files", result.ProcessedFiles,
		"comments_created", result.CommentsCreated,
		"elapsed_time", timer.ElapsedTime().String(),
	)
	if result.IsSuccess {
		log.Info("successfully reviewed")
		return
	}

	log.Error("review completed with errors", "error_count", len(result.Errors))
	for _, err := range result.Errors {
		log.Err(err, "processing error")
	}
}
