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
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Service implements the ReviewService interface
type Service struct {
	provider models.CodeProvider
	agent    models.AIAgent
	config   *config.Config
	logger   logze.Logger

	// Track processed MRs and reviewed files
	processedMRs    map[string]bool
	processedMRsMu  sync.RWMutex
	reviewedFiles   map[string]map[string]string
	reviewedFilesMu sync.RWMutex
}

// NewService creates a new review service
func NewService(provider models.CodeProvider, agent models.AIAgent, cfg *config.Config, logger logze.Logger) *Service {
	return &Service{
		provider:      provider,
		agent:         agent,
		config:        cfg,
		logger:        logger,
		processedMRs:  make(map[string]bool),
		reviewedFiles: make(map[string]map[string]string),
	}
}

// ProcessMergeRequest processes a merge request for code review
func (s *Service) ProcessMergeRequest(ctx context.Context, request *models.ReviewRequest) (*models.ReviewResult, error) {
	log := s.logger.WithFields(
		"project_id", request.ProjectID,
		"mr_iid", request.MergeRequest.IID,
		"commit_sha", request.CommitSHA[:8],
	)

	result := &models.ReviewResult{}

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

	// Determine new and changed files
	newAndChangedFiles := s.getNewAndChangedFiles(request.ProjectID, request.MergeRequest.IID, request.CommitSHA, filesToReview, log)
	if len(newAndChangedFiles) == 0 {
		log.Info("no new or changed files to review")
		s.markMRAsProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA)
		result.Success = true
		return result, nil
	}

	log.Info("found files to review",
		"total_files", len(filesToReview),
		"new_or_changed_files", len(newAndChangedFiles),
	)

	// Generate description if enabled and appropriate
	if s.config.Review.EnableDescriptionGeneration && s.shouldGenerateDescription(request.MergeRequest, newAndChangedFiles) {
		err := s.generateAndUpdateDescription(ctx, request.ProjectID, request.MergeRequest.IID, newAndChangedFiles, log)
		if err != nil {
			result.Errors = append(result.Errors, errm.Wrap(err, "failed to generate description"))
		} else {
			result.DescriptionUpdated = true
		}
	}

	// Review code changes if enabled
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

	// Mark as processed
	s.markMRAsProcessed(request.ProjectID, request.MergeRequest.IID, request.CommitSHA)

	return result, nil
}

// ShouldProcessMergeRequest determines if an MR should be processed
func (s *Service) ShouldProcessMergeRequest(ctx context.Context, mr *models.MergeRequest) bool {
	// Always process if bot is a reviewer (GitHub review_requested trigger)
	if slices.ContainsFunc(mr.Reviewers, func(u *models.User) bool {
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

// filterFilesForReview filters files based on configuration
func (s *Service) filterFilesForReview(changes []*models.FileDiff, log logze.Logger) []*models.FileDiff {
	var filtered []*models.FileDiff

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
func (s *Service) shouldGenerateDescription(mr *models.MergeRequest, changes []*models.FileDiff) bool {
	// Check if already has AI description
	if strings.Contains(mr.Description, "<!-- ai-desc-start -->") {
		return false
	}

	// Check minimum files threshold
	return len(changes) >= s.config.Review.MinFilesForDescription
}

// generateAndUpdateDescription generates and updates MR description
func (s *Service) generateAndUpdateDescription(ctx context.Context, projectID string, mrIID int, changes []*models.FileDiff, log logze.Logger) error {
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
func (s *Service) reviewCodeChanges(ctx context.Context, projectID string, mrIID int, changes []*models.FileDiff, commitSHA string, log logze.Logger) (int, error) {
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
		comment := &models.ReviewComment{
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

func (s *Service) getNewAndChangedFiles(projectID string, mrIID int, currentCommitSHA string, diffs []*models.FileDiff, log logze.Logger) []*models.FileDiff {
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

	var newAndChangedFiles []*models.FileDiff
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
