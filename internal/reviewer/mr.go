package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/codry/internal/reviewer/llmcontext"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

// ReviewRequest represents a code review request
type ReviewRequest struct {
	ProjectID string
	Context   *llmcontext.ContextBundle
}

// ReviewResult represents the result of a code review process
type ReviewResult struct {
	ProcessedFiles  int
	CommentsCreated int

	IsSuccess                   bool
	IsDescriptionCreated        bool
	IsChangesOverviewCreated    bool
	IsArchitectureReviewCreated bool
	IsCodeReviewCreated         bool

	Errors []error
}

// ReviewMergeRequest handles merge request related events
func (s *Reviewer) ReviewMergeRequest(ctx context.Context, projectID string, mrIID int) error {

	s.log.Infof("reviewing merge request", "project_id", projectID, "mr_iid", mrIID)

	// Step 1: Gather comprehensive MR context before any generation
	s.logFlow(s.log, "starting gathering context", "mr_iid", mrIID)
	mrContext, err := s.contextBuilder.BuildContext(ctx, projectID, mrIID)
	if err != nil {
		return erro.Wrap(err, "failed to gather MR context")
	}

	// Step 2: Process the review with rich context
	s.processMergeRequestReview(ctx, ReviewRequest{
		ProjectID: projectID,
		Context:   mrContext,
	})

	return nil
}

type reviewBundle struct {
	request        ReviewRequest
	fullDiffString string
	result         *ReviewResult

	log   logze.Logger
	timer abstract.Timer
}

// ProcessMergeRequest processes a merge request for the first time
func (s *Reviewer) processMergeRequestReview(ctx context.Context, request ReviewRequest) {
	log := s.log.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.Context.MR.IID,
		"branch_from", request.Context.MR.SourceBranch,
		"branch_to", request.Context.MR.TargetBranch,
		"commit_sha", lang.TruncateString(request.Context.MR.SHA, 8),
	)
	s.logFlow(log, "starting merge request review: "+request.Context.MR.Title)

	reviewBundle := &reviewBundle{
		result:         &ReviewResult{},
		request:        request,
		fullDiffString: buildDiffString(request.Context.FilesForReview, request.Context.TotalDiffLength),
		timer:          abstract.StartTimer(),
		log:            log,
	}

	defer func() {
		s.logProcessingResults(*reviewBundle.result, reviewBundle.timer, s.log)
	}()

	s.generateDescription(ctx, reviewBundle)
	s.generateChangesOverview(ctx, reviewBundle)
	s.generateArchitectureReview(ctx, reviewBundle)
	s.generateCodeReview(ctx, reviewBundle)

	reviewBundle.result.ProcessedFiles = len(request.Context.FilesForReview)
	reviewBundle.result.IsSuccess = len(reviewBundle.result.Errors) == 0
}

func buildDiffString(files []*llmcontext.FileContext, totalDiffLength int64) string {
	var fullDiff strings.Builder
	fullDiff.Grow(int(totalDiffLength) + 30)
	for _, change := range files {
		fullDiff.WriteString("--- a/")
		fullDiff.WriteString(change.Diff.OldPath)
		fullDiff.WriteString("\n+++ b/")
		fullDiff.WriteString(change.Diff.NewPath)
		fullDiff.WriteString("\n")
		fullDiff.WriteString(change.Diff.Diff)
		fullDiff.WriteString("\n\n")
	}
	return fullDiff.String()
}

// logProcessingResults logs the results of MR processing
func (s *Reviewer) logProcessingResults(result ReviewResult, timer abstract.Timer, log logze.Logger) {
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
	// for _, err := range result.Errors {
	// 	log.Err(err, "processing error")
	// }
}

func (r ReviewRequest) String() string {
	return r.ProjectID + ":" + r.Context.MR.SHA + ":" + r.Context.MR.IIDStr
}
