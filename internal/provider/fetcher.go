package provider

import (
	"context"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// MRFetcher provides utility methods for fetching merge requests from repositories
type MRFetcher struct {
	provider model.CodeProvider
	logger   logze.Logger
}

// NewMRFetcher creates a new MR fetcher instance
func NewMRFetcher(provider model.CodeProvider) *MRFetcher {
	return &MRFetcher{
		provider: provider,
		logger:   logze.With("component", "mr_fetcher"),
	}
}

// FetchOpenMRs retrieves all open merge requests from a repository
func (f *MRFetcher) FetchOpenMRs(ctx context.Context, projectID string) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		State: []string{"open", "opened"}, // Support both GitLab and GitHub terminology
		Limit: 100,                        // Get up to 100 MRs per page
	}

	f.logger.Info("fetching open merge requests", "project_id", projectID)
	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// FetchRecentMRs retrieves merge requests updated in the last specified duration
func (f *MRFetcher) FetchRecentMRs(ctx context.Context, projectID string, since time.Duration) ([]*model.MergeRequest, error) {
	sinceTime := time.Now().Add(-since)

	f.logger.Info("fetching recent merge requests",
		"project_id", projectID,
		"since", sinceTime.Format(time.RFC3339))

	return f.provider.GetMergeRequestUpdates(ctx, projectID, sinceTime)
}

// FetchMRsByAuthor retrieves merge requests created by a specific author
func (f *MRFetcher) FetchMRsByAuthor(ctx context.Context, projectID, authorID string) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		State:    []string{"open", "opened"},
		AuthorID: authorID,
		Limit:    50,
	}

	f.logger.Info("fetching merge requests by author",
		"project_id", projectID,
		"author_id", authorID)

	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// FetchMRsToReview retrieves merge requests that need review based on various criteria
func (f *MRFetcher) FetchMRsToReview(ctx context.Context, projectID string, options FetchOptions) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		State:        []string{"open", "opened"},
		TargetBranch: options.TargetBranch,
		Limit:        options.Limit,
	}

	if options.UpdatedSince != nil {
		filter.UpdatedAfter = options.UpdatedSince
	}

	if options.CreatedSince != nil {
		filter.CreatedAfter = options.CreatedSince
	}

	f.logger.Info("fetching merge requests for review",
		"project_id", projectID,
		"target_branch", options.TargetBranch,
		"limit", options.Limit)

	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// PollForUpdates continuously polls for updated merge requests
func (f *MRFetcher) PollForUpdates(ctx context.Context, projectID string, interval time.Duration, callback func([]*model.MergeRequest)) error {
	f.logger.Info("starting MR polling",
		"project_id", projectID,
		"interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastUpdate := time.Now().Add(-24 * time.Hour) // Start with last 24 hours

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("stopping MR polling due to context cancellation")
			return ctx.Err()

		case <-ticker.C:
			mrs, err := f.provider.GetMergeRequestUpdates(ctx, projectID, lastUpdate)
			if err != nil {
				f.logger.Error("failed to fetch MR updates", "error", err)
				continue
			}

			if len(mrs) > 0 {
				f.logger.Info("found updated merge requests", "count", len(mrs))
				callback(mrs)

				// Update the last update time to the most recent MR update
				for _, mr := range mrs {
					if mr.UpdatedAt.After(lastUpdate) {
						lastUpdate = mr.UpdatedAt
					}
				}
			}
		}
	}
}

// BatchProcessMRs processes multiple merge requests with a callback function
func (f *MRFetcher) BatchProcessMRs(ctx context.Context, projectID string, filter *model.MergeRequestFilter, processor func(*model.MergeRequest) error) error {
	f.logger.Info("starting batch MR processing", "project_id", projectID)

	page := 0
	for {
		filter.Page = page
		mrs, err := f.provider.ListMergeRequests(ctx, projectID, filter)
		if err != nil {
			return errm.Wrap(err, "failed to fetch merge requests")
		}

		if len(mrs) == 0 {
			break // No more results
		}

		f.logger.Debug("processing MR batch", "count", len(mrs), "page", page)

		for _, mr := range mrs {
			if err := processor(mr); err != nil {
				f.logger.Error("failed to process merge request",
					"mr_id", mr.ID,
					"error", err)
				// Continue processing other MRs instead of failing
			}
		}

		// If we got fewer results than the limit, we've reached the end
		if len(mrs) < filter.Limit {
			break
		}

		page++
	}

	f.logger.Info("completed batch MR processing", "project_id", projectID)
	return nil
}

// FetchOptions defines options for fetching merge requests
type FetchOptions struct {
	TargetBranch string     // Filter by target branch (e.g., "main", "develop")
	UpdatedSince *time.Time // Only fetch MRs updated after this time
	CreatedSince *time.Time // Only fetch MRs created after this time
	Limit        int        // Maximum number of results (default: 50)
}

// SetDefaults sets default values for fetch options
func (o *FetchOptions) SetDefaults() {
	if o.Limit == 0 {
		o.Limit = 50
	}
}

// MRProcessor defines a function type for processing individual merge requests
type MRProcessor func(*model.MergeRequest) error

// Example usage functions

// CreateReviewProcessor returns a processor that creates review events for merge requests
func CreateReviewProcessor(reviewService model.ReviewService) MRProcessor {
	return func(mr *model.MergeRequest) error {
		// Create a review request for the merge request
		request := &model.ReviewRequest{
			ProjectID:    mr.AuthorID, // This would need proper project ID mapping
			MergeRequest: mr,
			CommitSHA:    mr.SHA,
		}

		ctx := context.Background()
		_, err := reviewService.ProcessMergeRequest(ctx, request)
		return err
	}
}

// FilterNeedsReview filters merge requests that need review based on criteria
func FilterNeedsReview(mr *model.MergeRequest, botUsername string) bool {
	// Skip if bot is already a reviewer
	for _, reviewer := range mr.Reviewers {
		if reviewer.Username == botUsername {
			return false
		}
	}

	// Only review open/opened MRs
	if mr.State != "open" && mr.State != "opened" {
		return false
	}

	// Add more criteria as needed
	return true
}
