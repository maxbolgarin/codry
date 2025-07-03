package review

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"github.com/panjf2000/ants/v2"
)

// Service implements the ReviewService interface
type Service struct {
	provider model.CodeProvider
	agent    model.AIAgent
	config   *config.Config
	logger   logze.Logger
	pool     *ants.Pool

	// Track processed MRs and reviewed files
	processedMRs    map[string]bool
	processedMRsMu  sync.RWMutex
	reviewedFiles   map[string]map[string]string
	reviewedFilesMu sync.RWMutex
}

// NewService creates a new review service
func NewService(provider model.CodeProvider, agent model.AIAgent, cfg *config.Config, logger logze.Logger) (*Service, error) {
	pool, err := ants.NewPool(100)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create ants pool")
	}

	s := &Service{
		provider:      provider,
		agent:         agent,
		config:        cfg,
		logger:        logger,
		pool:          pool,
		processedMRs:  make(map[string]bool),
		reviewedFiles: make(map[string]map[string]string),
	}

	return s, nil
}

// HandleWebhook processes incoming webhook events and routes them appropriately
func (s *Service) HandleEvent(ctx context.Context, event *model.CodeEvent) error {
	log := s.logger.WithFields(
		"event_type", event.Type,
		"action", event.Action,
		"project_id", event.ProjectID,
		"user", event.User.Username,
	)

	log.Info("processing event")

	switch {
	case s.provider.IsMergeRequestEvent(event):
		return s.pool.Submit(func() {
			// TODO: add error handling
			s.processMergeRequestEvent(ctx, event, log)
		})

	case s.provider.IsCommentEvent(event):
		return s.pool.Submit(func() {
			// TODO: add error handling
			s.processCommentEvent(ctx, event, log)
		})

	default:
		log.Debug("unhandled webhook event type")
		return nil
	}
}

// processMergeRequestEvent handles merge request related events
func (s *Service) processMergeRequestEvent(ctx context.Context, event *model.CodeEvent, log logze.Logger) error {
	if event.MergeRequest == nil {
		return errm.New("merge request is nil in event")
	}

	log = log.WithFields(
		"mr_iid", event.MergeRequest.IID,
		"mr_title", event.MergeRequest.Title,
	)

	// Check if MR should be processed
	if !s.ShouldProcessMergeRequest(ctx, event.MergeRequest) {
		log.Debug("merge request should not be processed")
		return nil
	}

	// Get detailed MR information and diffs
	mr, err := s.provider.GetMergeRequest(ctx, event.ProjectID, event.MergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to get merge request details")
	}

	// Get file diffs
	diffs, err := s.provider.GetMergeRequestDiffs(ctx, event.ProjectID, event.MergeRequest.IID)
	if err != nil {
		return errm.Wrap(err, "failed to get merge request diffs")
	}

	// Create review request
	reviewRequest := &model.ReviewRequest{
		ProjectID:    event.ProjectID,
		MergeRequest: mr,
		Changes:      diffs,
		CommitSHA:    mr.SHA,
	}

	// Determine if this is a first-time processing or an update
	var result *model.ReviewResult
	if s.isMRAlreadyProcessed(event.ProjectID, event.MergeRequest.IID, mr.SHA) {
		log.Info("processing merge request update")
		result, err = s.ProcessMergeRequestUpdate(ctx, reviewRequest)
	} else {
		log.Info("processing merge request for the first time")
		result, err = s.ProcessMergeRequest(ctx, reviewRequest)
	}

	if err != nil {
		return errm.Wrap(err, "failed to process merge request")
	}

	// Log results
	s.logProcessingResults(result, log)
	return nil
}

// processCommentEvent handles comment related events
func (s *Service) processCommentEvent(ctx context.Context, event *model.CodeEvent, log logze.Logger) error {
	if event.Comment == nil || event.MergeRequest == nil {
		return errm.New("comment or merge request is nil in event")
	}

	log = log.WithFields(
		"comment_id", event.Comment.ID,
		"mr_iid", event.MergeRequest.IID,
		"comment_author", event.Comment.Author.Username,
	)

	// Check if this is a reply to our bot's comment
	if s.isReplyToBotComment(ctx, event) {
		log.Info("processing reply to bot comment")
		return s.ProcessCommentReply(ctx, event.ProjectID, event.MergeRequest.IID, event.Comment)
	}

	log.Debug("comment event not relevant for processing")
	return nil
}

// ProcessMergeRequest processes a merge request for the first time
func (s *Service) ProcessMergeRequest(ctx context.Context, request *model.ReviewRequest) (*model.ReviewResult, error) {
	log := s.logger.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.MergeRequest.IID,
		"commit_sha", request.CommitSHA[:8],
	)

	result := &model.ReviewResult{}

	// Check if already processed
	if s.isMRAlreadyProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA) {
		log.Debug("MR already processed for this commit")
		result.Success = true
		return result, nil
	}

	// Add processing delay to let the provider process commits
	time.Sleep(s.config.Review.ProcessingDelay)

	// Filter files for review
	filesToReview := s.filterFilesForReview(request.Changes, log)
	if len(filesToReview) == 0 {
		log.Info("no files to review after filtering")
		s.markMRAsProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA)
		result.Success = true
		return result, nil
	}

	log.Info("found files to review", "total_files", len(filesToReview))

	// Generate description if enabled and appropriate
	if s.config.Review.EnableDescriptionGeneration && s.shouldGenerateDescription(request.MergeRequest, filesToReview) {
		err := s.generateAndUpdateDescription(ctx, request.ProjectID, request.MergeRequest.IID, filesToReview, log)
		if err != nil {
			result.Errors = append(result.Errors, errm.Wrap(err, "failed to generate description"))
		} else {
			result.DescriptionUpdated = true
		}
	}

	// Review code changes if enabled
	if s.config.Review.EnableCodeReview {
		commentsCreated, err := s.reviewCodeChanges(ctx, request.ProjectID, request.MergeRequest.IID, filesToReview, request.CommitSHA, log)
		if err != nil {
			result.Errors = append(result.Errors, errm.Wrap(err, "failed to review code changes"))
		} else {
			result.CommentsCreated = commentsCreated
		}
	}

	result.ProcessedFiles = len(filesToReview)
	result.Success = len(result.Errors) == 0

	// Mark as processed
	s.markMRAsProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA)

	return result, nil
}

// ProcessMergeRequestUpdate processes updates to an existing merge request
func (s *Service) ProcessMergeRequestUpdate(ctx context.Context, request *model.ReviewRequest) (*model.ReviewResult, error) {
	log := s.logger.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.MergeRequest.IID,
		"commit_sha", request.CommitSHA[:8],
	)

	result := &model.ReviewResult{}

	// Filter files for review
	filesToReview := s.filterFilesForReview(request.Changes, log)
	if len(filesToReview) == 0 {
		log.Info("no files to review after filtering")
		result.Success = true
		return result, nil
	}

	// Determine new and changed files only
	newAndChangedFiles := s.getNewAndChangedFiles(request.ProjectID, request.MergeRequest.IID, request.CommitSHA, filesToReview, log)
	if len(newAndChangedFiles) == 0 {
		log.Info("no new or changed files to review")
		result.Success = true
		return result, nil
	}

	log.Info("found files to review in update",
		"total_files", len(filesToReview),
		"new_or_changed_files", len(newAndChangedFiles),
	)

	// Update description if enabled and there are significant changes
	if s.config.Review.EnableDescriptionGeneration && len(newAndChangedFiles) >= s.config.Review.MinFilesForDescription {
		err := s.updateDescriptionForChanges(ctx, request.ProjectID, request.MergeRequest.IID, newAndChangedFiles, log)
		if err != nil {
			result.Errors = append(result.Errors, errm.Wrap(err, "failed to update description"))
		} else {
			result.DescriptionUpdated = true
		}
	}

	// Review only new and changed files
	if s.config.Review.EnableCodeReview {
		commentsCreated, err := s.reviewCodeChanges(ctx, request.ProjectID, request.MergeRequest.IID, newAndChangedFiles, request.CommitSHA, log)
		if err != nil {
			result.Errors = append(result.Errors, errm.Wrap(err, "failed to review code changes"))
		} else {
			result.CommentsCreated = commentsCreated
		}
	}

	result.ProcessedFiles = len(newAndChangedFiles)
	result.Success = len(result.Errors) == 0

	// Mark as processed with new commit SHA
	s.markMRAsProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA)

	return result, nil
}

// ProcessCommentReply handles replies to bot comments
func (s *Service) ProcessCommentReply(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error {
	log := s.logger.WithFields(
		"project_id", projectID,
		"mr_iid", mrIID,
		"comment_id", comment.ID,
		"comment_author", comment.Author.Username,
	)

	// Get the original comment being replied to
	originalComment, err := s.provider.GetComment(ctx, projectID, mrIID, comment.ParentID)
	if err != nil {
		return errm.Wrap(err, "failed to get original comment")
	}

	// Generate a contextual reply
	replyContext := fmt.Sprintf("Original comment: %s\nUser reply: %s", originalComment.Body, comment.Body)
	reply, err := s.generateWithRetry(ctx, func() (string, error) {
		return s.agent.GenerateCommentReply(ctx, originalComment.Body, replyContext)
	}, log)
	if err != nil {
		return errm.Wrap(err, "failed to generate reply")
	}

	if reply == "" {
		log.Debug("agent returned empty reply")
		return nil
	}

	// Post the reply
	err = s.provider.ReplyToComment(ctx, projectID, mrIID, comment.ID, reply)
	if err != nil {
		return errm.Wrap(err, "failed to post reply")
	}

	log.Info("successfully posted reply to comment")
	return nil
}

// ShouldProcessMergeRequest determines if an MR should be processed
func (s *Service) ShouldProcessMergeRequest(ctx context.Context, mr *model.MergeRequest) bool {
	// Always process if bot is a reviewer (GitHub review_requested trigger)
	if slices.ContainsFunc(mr.Reviewers, func(u *model.User) bool {
		return u.Username == s.config.Provider.BotUsername
	}) {
		return true
	}

	// Check MR state for other triggers
	if mr.State != "opened" && mr.State != "open" {
		return false
	}

	return true
}

// isReplyToBotComment checks if a comment is a reply to the bot's comment
func (s *Service) isReplyToBotComment(ctx context.Context, event *model.CodeEvent) bool {
	if event.Comment.ParentID == "" {
		return false
	}

	// Get the parent comment
	parentComment, err := s.provider.GetComment(ctx, event.ProjectID, event.MergeRequest.IID, event.Comment.ParentID)
	if err != nil {
		s.logger.Err(err, "failed to get parent comment for reply check")
		return false
	}

	// Check if parent comment is from our bot
	return parentComment.Author.Username == s.config.Provider.BotUsername
}

// updateDescriptionForChanges updates description with information about new changes
func (s *Service) updateDescriptionForChanges(ctx context.Context, projectID string, mrIID int, changes []*model.FileDiff, log logze.Logger) error {
	// Build diff for new changes only
	var changesDiff strings.Builder
	changesDiff.WriteString("## Recent Changes:\n\n")
	for _, change := range changes {
		changesDiff.WriteString(fmt.Sprintf("### %s\n", change.NewPath))
		changesDiff.WriteString(fmt.Sprintf("```diff\n%s\n```\n\n", change.Diff))
	}

	// Generate description for changes
	changeDescription, err := s.generateWithRetry(ctx, func() (string, error) {
		return s.agent.SummarizeChanges(ctx, changes)
	}, log)
	if err != nil {
		return errm.Wrap(err, "failed to generate changes description")
	}

	if changeDescription == "" {
		log.Warn("agent returned empty changes description")
		return nil
	}

	// Get current MR to update description
	currentMR, err := s.provider.GetMergeRequest(ctx, projectID, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get current MR")
	}

	// Update description with changes section
	newDescription := s.updateDescriptionWithChangesSection(currentMR.Description, changeDescription)

	// Update MR description
	err = s.provider.UpdateMergeRequestDescription(ctx, projectID, mrIID, newDescription)
	if err != nil {
		return errm.Wrap(err, "failed to update MR description")
	}

	log.Info("successfully updated MR description with changes")
	return nil
}

// updateDescriptionWithChangesSection updates description with changes section
func (s *Service) updateDescriptionWithChangesSection(currentDescription, changesDescription string) string {
	const (
		changesStartMarker = "<!-- ai-changes-start -->"
		changesEndMarker   = "<!-- ai-changes-end -->"
	)

	changesSection := fmt.Sprintf("%s\n### 🔄 Recent Changes\n\n%s\n%s", changesStartMarker, changesDescription, changesEndMarker)

	// Remove existing changes section if present
	startIndex := strings.Index(currentDescription, changesStartMarker)
	if startIndex != -1 {
		endIndex := strings.Index(currentDescription, changesEndMarker)
		if endIndex != -1 {
			endIndex += len(changesEndMarker)
			beforeChanges := strings.TrimRight(currentDescription[:startIndex], "\n\r \t")
			afterChanges := strings.TrimLeft(currentDescription[endIndex:], "\n\r \t")

			if afterChanges == "" {
				return fmt.Sprintf("%s\n\n%s", beforeChanges, changesSection)
			}
			return fmt.Sprintf("%s\n\n%s\n\n%s", beforeChanges, changesSection, afterChanges)
		}
	}

	// Add changes section at the top
	if currentDescription == "" {
		return changesSection
	}
	return fmt.Sprintf("%s\n\n---\n\n%s", changesSection, currentDescription)
}

// logProcessingResults logs the results of MR processing
func (s *Service) logProcessingResults(result *model.ReviewResult, log logze.Logger) {
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

// filterFilesForReview filters files based on configuration
func (s *Service) filterFilesForReview(changes []*model.FileDiff, log logze.Logger) []*model.FileDiff {
	var filtered []*model.FileDiff

	for _, change := range changes {
		if change.IsDeleted {
			continue
		}

		if change.IsBinary {
			continue
		}

		if len(change.Diff) == 0 || len(change.Diff) > s.config.Review.FileFilter.MaxFileSize {
			log.Debug("skipping file due to size", "file", change.NewPath, "size", len(change.Diff))
			continue
		}

		if !s.isCodeFile(change.NewPath) {
			continue
		}

		if s.isExcludedPath(change.NewPath) {
			log.Debug("skipping excluded file", "file", change.NewPath)
			continue
		}

		log.Debug("adding file to review", "file", change.NewPath)
		filtered = append(filtered, change)

		// Limit number of files per MR
		if len(filtered) >= s.config.Review.MaxFilesPerMR {
			log.Warn("reached maximum files per MR limit", "limit", s.config.Review.MaxFilesPerMR)
			break
		}
	}

	return filtered
}

// isCodeFile checks if a file should be reviewed based on extension
func (s *Service) isCodeFile(filePath string) bool {
	if s.config.Review.FileFilter.IncludeOnlyCode {
		ext := strings.ToLower(filepath.Ext(filePath))
		return slices.Contains(s.config.Review.FileFilter.AllowedExtensions, ext)
	}
	return true
}

// isExcludedPath checks if a path should be excluded
func (s *Service) isExcludedPath(filePath string) bool {
	for _, pattern := range s.config.Review.FileFilter.ExcludedPaths {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}

// shouldGenerateDescription determines if description should be generated
func (s *Service) shouldGenerateDescription(mr *model.MergeRequest, changes []*model.FileDiff) bool {
	// Check if already has AI description
	if strings.Contains(mr.Description, "<!-- ai-desc-start -->") {
		return false
	}

	// Check minimum files threshold
	return len(changes) >= s.config.Review.MinFilesForDescription
}

// generateAndUpdateDescription generates and updates MR description
func (s *Service) generateAndUpdateDescription(ctx context.Context, projectID string, mrIID int, changes []*model.FileDiff, log logze.Logger) error {
	// Build full diff
	var fullDiff strings.Builder
	for _, change := range changes {
		fullDiff.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", change.OldPath, change.NewPath))
		fullDiff.WriteString(change.Diff)
		fullDiff.WriteString("\n\n")
	}

	// Generate description with retry
	description, err := s.generateWithRetry(ctx, func() (string, error) {
		return s.agent.GenerateDescription(ctx, fullDiff.String())
	}, log)
	if err != nil {
		return errm.Wrap(err, "failed to generate description")
	}

	if description == "" {
		log.Warn("agent returned empty description")
		return nil
	}

	// Get current MR to preserve existing description
	currentMR, err := s.provider.GetMergeRequest(ctx, projectID, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get current MR")
	}

	// Update description with AI section
	newDescription := s.updateDescriptionWithAISection(currentMR.Description, description)

	// Update MR description
	err = s.provider.UpdateMergeRequestDescription(ctx, projectID, mrIID, newDescription)
	if err != nil {
		return errm.Wrap(err, "failed to update MR description")
	}

	log.Info("successfully updated MR description")
	return nil
}

// reviewCodeChanges reviews individual files and creates comments
func (s *Service) reviewCodeChanges(ctx context.Context, projectID string, mrIID int, changes []*model.FileDiff, commitSHA string, log logze.Logger) (int, error) {
	commentsCreated := 0

	for _, change := range changes {
		reviewComment, err := s.generateWithRetry(ctx, func() (string, error) {
			return s.agent.ReviewCode(ctx, change.NewPath, change.Diff)
		}, log)
		if err != nil {
			log.Err(err, "failed to review code for file", "file", change.NewPath)
			continue
		}

		// Skip if no issues found
		if reviewComment == "" || strings.HasPrefix(strings.TrimSpace(reviewComment), "OK") {
			log.Debug("no issues found for file", "file", change.NewPath)
			s.markFileAsReviewed(projectID, mrIID, commitSHA, change.NewPath, s.getFileHash(change.Diff))
			continue
		}

		// Create comment
		comment := &model.ReviewComment{
			Body:     fmt.Sprintf("### 🤖 Ревью для файла `%s`\n\n%s", change.NewPath, reviewComment),
			FilePath: change.NewPath,
		}

		err = s.provider.CreateComment(ctx, projectID, mrIID, comment)
		if err != nil {
			log.Err(err, "failed to create comment for file", "file", change.NewPath)
		} else {
			log.Info("successfully created comment for file", "file", change.NewPath)
			commentsCreated++
			s.markFileAsReviewed(projectID, mrIID, commitSHA, change.NewPath, s.getFileHash(change.Diff))
		}
	}

	return commentsCreated, nil
}

// generateWithRetry wraps AI generation calls with retry logic
func (s *Service) generateWithRetry(ctx context.Context, generateFn func() (string, error), log logze.Logger) (string, error) {
	for attempt := 1; attempt <= s.config.Agent.MaxRetries; attempt++ {
		result, err := generateFn()
		if err == nil {
			return result, nil
		}

		// Check if it's a rate limit error and we have retries left
		if strings.Contains(err.Error(), "429") && attempt < s.config.Agent.MaxRetries {
			log.Warn("rate limit exceeded, waiting before retry", "attempt", attempt, "delay", s.config.Agent.RetryDelay)
			time.Sleep(s.config.Agent.RetryDelay)
			continue
		}

		return "", err
	}

	return "", errm.New("max retries exceeded")
}

// updateDescriptionWithAISection updates MR description with AI section
func (s *Service) updateDescriptionWithAISection(currentDescription, newAIDescription string) string {
	const (
		startMarker = "<!-- ai-desc-start -->"
		endMarker   = "<!-- ai-desc-end -->"
	)

	aiSection := fmt.Sprintf("%s\n### 🤖 Описание изменений\n\n%s\n%s", startMarker, newAIDescription, endMarker)

	startIndex := strings.Index(currentDescription, startMarker)
	if startIndex == -1 {
		if currentDescription == "" {
			return aiSection
		}
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, currentDescription)
	}

	endIndex := strings.Index(currentDescription, endMarker)
	if endIndex == -1 {
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, currentDescription[startIndex:])
	}

	endIndex += len(endMarker)
	beforeAI := strings.TrimRight(currentDescription[:startIndex], "\n\r \t")
	afterAI := strings.TrimLeft(currentDescription[endIndex:], "\n\r \t")

	switch {
	case beforeAI == "" && afterAI == "":
		return aiSection
	case beforeAI == "":
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, afterAI)
	case afterAI == "":
		return fmt.Sprintf("%s\n\n---\n\n%s", beforeAI, aiSection)
	default:
		return fmt.Sprintf("%s\n\n---\n\n%s\n\n---\n\n%s", beforeAI, aiSection, afterAI)
	}
}

// Helper methods for tracking processed MRs and files

func (s *Service) getMRKey(projectID string, mrIID int, commitSHA string) string {
	return fmt.Sprintf("%s:%d:%s", projectID, mrIID, commitSHA)
}

func (s *Service) isMRAlreadyProcessed(projectID string, mrIID int, commitSHA string) bool {
	key := s.getMRKey(projectID, mrIID, commitSHA)
	s.processedMRsMu.RLock()
	defer s.processedMRsMu.RUnlock()
	_, exists := s.processedMRs[key]
	return exists
}

func (s *Service) markMRAsProcessed(projectID string, mrIID int, commitSHA string) {
	key := s.getMRKey(projectID, mrIID, commitSHA)
	s.processedMRsMu.Lock()
	defer s.processedMRsMu.Unlock()
	s.processedMRs[key] = true
}

func (s *Service) getFileHash(diff string) string {
	if diff == "" {
		return ""
	}
	if len(diff) > 100 {
		return fmt.Sprintf("%d:%s", len(diff), diff[:100])
	}
	return fmt.Sprintf("%d:%s", len(diff), diff)
}

func (s *Service) markFileAsReviewed(projectID string, mrIID int, commitSHA, filePath, fileHash string) {
	key := s.getMRKey(projectID, mrIID, commitSHA)
	s.reviewedFilesMu.Lock()
	defer s.reviewedFilesMu.Unlock()

	if s.reviewedFiles[key] == nil {
		s.reviewedFiles[key] = make(map[string]string)
	}
	s.reviewedFiles[key][filePath] = fileHash
}

func (s *Service) getNewAndChangedFiles(projectID string, mrIID int, currentCommitSHA string, diffs []*model.FileDiff, log logze.Logger) []*model.FileDiff {
	var previouslyReviewedFiles map[string]string

	s.reviewedFilesMu.RLock()
	for keyStr, reviewedFiles := range s.reviewedFiles {
		if strings.HasPrefix(keyStr, fmt.Sprintf("%s:%d:", projectID, mrIID)) && !strings.HasSuffix(keyStr, currentCommitSHA) {
			if previouslyReviewedFiles == nil {
				previouslyReviewedFiles = make(map[string]string)
			}
			for filePath, fileHash := range reviewedFiles {
				previouslyReviewedFiles[filePath] = fileHash
			}
		}
	}
	s.reviewedFilesMu.RUnlock()

	if previouslyReviewedFiles == nil {
		log.Debug("no previously reviewed files found, processing all diffs")
		return diffs
	}

	var newAndChangedFiles []*model.FileDiff
	for _, diff := range diffs {
		fileHash := s.getFileHash(diff.Diff)
		filePath := diff.NewPath

		if previousHash, exists := previouslyReviewedFiles[filePath]; !exists {
			newAndChangedFiles = append(newAndChangedFiles, diff)
		} else if previousHash != fileHash {
			newAndChangedFiles = append(newAndChangedFiles, diff)
		}
	}

	log.Debug("identified files for review",
		"total_files", len(diffs),
		"new_or_changed_files", len(newAndChangedFiles),
		"previously_reviewed_files", len(previouslyReviewedFiles),
	)

	return newAndChangedFiles
}
