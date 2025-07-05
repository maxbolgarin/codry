package reviewer

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// reviewCodeChanges reviews individual files and creates comments
func (s *Reviewer) reviewCodeChanges(ctx context.Context, request model.ReviewRequest, changes []*model.FileDiff, log logze.Logger) (int, error) {
	if !s.cfg.EnableCodeReview {
		log.Info("code review is disabled, skipping")
		return 0, nil
	}

	log.Info("reviewing code changes")
	commentsCreated := 0

	errs := errm.NewList()
	for _, change := range changes {
		fileHash := s.getFileHash(change.Diff)
		if oldHash, ok := s.processedMRs.Lookup(request.String(), change.NewPath); ok {
			if oldHash == fileHash {
				log.Debug("skipping already reviewed", "file", change.NewPath)
				continue
			}
		}

		log.Debug("reviewing code change", "file", change.NewPath)

		reviewComment, err := s.agent.ReviewCode(ctx, change.NewPath, change.Diff)
		if err != nil {
			errs.Wrap(err, "failed to review code change", "file", change.NewPath)
			log.Error("failed to review code change", "error", err, "file", change.NewPath)
			continue
		}

		// Skip if no issues found
		if reviewComment == "" || strings.HasPrefix(strings.TrimSpace(reviewComment), "OK") {
			log.Debug("skipping empty or OK comment", "file", change.NewPath)
			s.processedMRs.Set(request.String(), change.NewPath, fileHash)
			continue
		}

		// Create comment
		comment := &model.Comment{
			Body:     reviewComment,
			FilePath: change.NewPath,
		}

		err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
		if err != nil {
			errs.Wrap(err, "failed to create comment", "file", change.NewPath)
			log.Error("failed to create comment", "error", err, "file", change.NewPath)
			continue
		}

		commentsCreated++
		s.processedMRs.Set(request.String(), change.NewPath, fileHash)

		log.Info("file reviewed and commented", "file", change.NewPath)
	}

	return commentsCreated, nil
}

// Helper methods for tracking processed MRs and files

func (s *Reviewer) getFileHash(diff string) string {
	if diff == "" {
		return ""
	}

	var hash uint64
	for i, char := range diff {
		hash = (hash*31 + uint64(char))
		if i >= 100 {
			break
		}
	}

	return fmt.Sprintf("%d:%d", len(diff), hash)
}
