package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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

	s.log.Infof("reviewing merge request: %s", mergeRequest.Title)

	// Check single review mode - only review once unless there are new changes
	if s.cfg.SingleReviewMode && s.hasAlreadyBeenReviewed(projectID, mergeRequest) {
		s.logFlow("MR already reviewed in single review mode, skipping",
			"mr_iid", mergeRequest.IID,
			"project_id", projectID,
			"current_sha", mergeRequest.SHA)
		return nil
	}

	// Step 1: Gather comprehensive MR context before any generation
	s.logFlow("gathering comprehensive MR context", "mr_iid", mergeRequest.IID)
	mrContext, err := s.contextBuilder.BuildContext(ctx, projectID, mergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to gather MR context")
	}

	// s.logFlow("MR context gathered successfully",
	// 	"mr_iid", mergeRequest.IID,
	// 	"total_files", mrContext.TotalFiles,
	// 	"total_commits", mrContext.TotalCommits,
	// 	"linked_issues", len(mrContext.LinkedIssues),
	// 	"linked_tickets", len(mrContext.LinkedTickets),
	// 	"author_comments", len(mrContext.AuthorComments),
	// 	"total_additions", mrContext.TotalAdditions,
	// 	"total_deletions", mrContext.TotalDeletions,
	// )

	res, err := json.MarshalIndent(mrContext, "", "  ")
	if err != nil {
		return errm.Wrap(err, "failed to marshal context bundle")
	}

	if err := os.WriteFile("context_bundle.json", res, 0644); err != nil {
		return errm.Wrap(err, "failed to write context bundle to file")
	}
	os.Exit(0)

	// Step 2: Process the review with rich context
	s.processMergeRequestReview(ctx, model.ReviewRequest{
		ProjectID:    projectID,
		MergeRequest: mergeRequest,
		Changes:      mrContext.MRContext.FileDiffs,
	})

	// Mark MR as reviewed in single review mode
	if s.cfg.SingleReviewMode {
		s.markMRAsReviewed(projectID, mergeRequest)
	}

	return nil
}

// hasAlreadyBeenReviewed checks if this MR has already been reviewed
func (s *Reviewer) hasAlreadyBeenReviewed(projectID string, mr *model.MergeRequest) bool {
	mrKey := fmt.Sprintf("%s:%d", projectID, mr.IID)

	trackingInfo := s.reviewedMRs.Get(mrKey)
	// Check if trackingInfo has a zero value (not found)
	if trackingInfo.LastReviewedSHA == "" {
		return false
	}

	// Check if the TTL has expired
	if s.cfg.TrackingTTL > 0 && time.Since(trackingInfo.LastReviewedAt) > s.cfg.TrackingTTL {
		s.reviewedMRs.Delete(mrKey)
		return false
	}

	// If SHA changed, need to review again
	if trackingInfo.LastReviewedSHA != mr.SHA {
		s.log.Debug("MR SHA changed since last review",
			"mr_iid", mr.IID,
			"project_id", projectID,
			"old_sha", trackingInfo.LastReviewedSHA,
			"new_sha", mr.SHA)
		return false
	}

	s.log.Debug("MR already reviewed",
		"mr_iid", mr.IID,
		"project_id", projectID,
		"sha", mr.SHA,
		"last_reviewed", trackingInfo.LastReviewedAt,
		"review_count", trackingInfo.ReviewCount)

	return true
}

// markMRAsReviewed marks an MR as reviewed
func (s *Reviewer) markMRAsReviewed(projectID string, mr *model.MergeRequest) {
	mrKey := fmt.Sprintf("%s:%d", projectID, mr.IID)

	// Get existing info or create new
	existingInfo := s.reviewedMRs.Get(mrKey)
	reviewCount := 1
	// Check if existing info has a non-zero value (found)
	if existingInfo.LastReviewedSHA != "" {
		reviewCount = existingInfo.ReviewCount + 1
	}

	trackingInfo := reviewTrackingInfo{
		LastReviewedSHA: mr.SHA,
		LastReviewedAt:  time.Now(),
		ReviewCount:     reviewCount,
	}

	s.reviewedMRs.Set(mrKey, trackingInfo)

	s.log.Debug("marked MR as reviewed",
		"mr_iid", mr.IID,
		"project_id", projectID,
		"sha", mr.SHA,
		"review_count", reviewCount)
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
	log.Infof("starting merge request review: '%s'", request.MergeRequest.Title)

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

		if len(file.Diff) > s.cfg.Filter.MaxFileSizeTokens {
			log.DebugIf(s.cfg.Verbose, "skipping due to size", "file", file.NewPath, "size", len(file.Diff), "max_size", s.cfg.Filter.MaxFileSizeTokens)
			continue
		}

		if s.cfg.isExcludedPath(file.NewPath) {
			log.DebugIf(s.cfg.Verbose, "skipping excluded", "file", file.NewPath)
			continue
		}

		if !s.cfg.isAllowedExtension(file.NewPath) {
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
		if len(filtered) >= s.cfg.Filter.MaxFiles {
			log.Warn("reached maximum files limit", "limit", s.cfg.Filter.MaxFiles)
			break
		}
	}

	if len(filtered) == 0 {
		s.logFlow("no files to review after filtering")
		return nil, 0
	}

	s.logFlow("found files to review",
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
