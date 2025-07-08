package provider

import (
	"context"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

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

// Fetcher provides utility methods for fetching merge requests from repositories
type Fetcher struct {
	provider interfaces.CodeProvider
	log      logze.Logger
}

// NewFetcher creates a new MR fetcher instance
func NewFetcher(provider interfaces.CodeProvider) *Fetcher {
	return &Fetcher{
		provider: provider,
		log:      logze.With("component", "fetcher"),
	}
}

// FetchOpenMRs retrieves all open merge requests from a repository
func (f *Fetcher) FetchOpenMRs(ctx context.Context, projectID string) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		State: []string{"open", "opened"}, // Support both GitLab and GitHub terminology
		Limit: 100,                        // Get up to 100 MRs per page
	}
	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// FetchRecentMRs retrieves merge requests updated in the last specified duration
func (f *Fetcher) FetchRecentMRs(ctx context.Context, projectID string, since time.Duration) ([]*model.MergeRequest, error) {
	sinceTime := time.Now().Add(-since)
	return f.provider.GetMergeRequestUpdates(ctx, projectID, sinceTime)
}

// FetchMRsByAuthor retrieves merge requests created by a specific author
func (f *Fetcher) FetchMRsByAuthor(ctx context.Context, projectID, authorID string) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		State:    []string{"open", "opened"},
		AuthorID: authorID,
		Limit:    50,
	}
	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// FetchMRsToReview retrieves merge requests that need review based on various criteria
func (f *Fetcher) FetchMRsToReview(ctx context.Context, projectID string, options FetchOptions) ([]*model.MergeRequest, error) {
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
	return f.provider.ListMergeRequests(ctx, projectID, filter)
}

// PollForUpdates continuously polls for updated merge requests
func (f *Fetcher) PollForUpdates(ctx context.Context, projectID string, interval time.Duration, callback func([]*model.MergeRequest)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastUpdate := time.Now().Add(-24 * time.Hour) // Start with last 24 hours

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			mrs, err := f.provider.GetMergeRequestUpdates(ctx, projectID, lastUpdate)
			if err != nil {
				f.log.Error("failed to fetch MR updates", "error", err)
				continue
			}

			if len(mrs) > 0 {
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
func (f *Fetcher) BatchProcessMRs(ctx context.Context, projectID string, filter *model.MergeRequestFilter, processor func(*model.MergeRequest) error) error {
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

		f.log.Debug("processing MR batch", "count", len(mrs), "page", page)

		for _, mr := range mrs {
			if err := processor(mr); err != nil {
				f.log.Error("failed to process merge request",
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
	return nil
}
