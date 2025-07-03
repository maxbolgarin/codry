package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/provider"
)

func main() {
	// Example: Configure GitLab provider
	providerConfig := provider.Config{
		Type:    provider.GitLab,
		BaseURL: "https://gitlab.com",
		Token:   "your-gitlab-token-here",
	}

	// Create provider
	vcsProvider, err := provider.NewProvider(providerConfig)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	// Create MR fetcher
	fetcher := provider.NewMRFetcher(vcsProvider)

	ctx := context.Background()
	projectID := "your-group/your-project" // GitLab project ID format

	// Example 1: Fetch all open merge requests
	fmt.Println("=== Fetching Open Merge Requests ===")
	openMRs, err := fetcher.FetchOpenMRs(ctx, projectID)
	if err != nil {
		log.Printf("Error fetching open MRs: %v", err)
	} else {
		fmt.Printf("Found %d open merge requests:\n", len(openMRs))
		for _, mr := range openMRs {
			fmt.Printf("- %s (#%d) by %s\n", mr.Title, mr.IID, mr.Author.Username)
		}
	}

	// Example 2: Fetch recent merge requests (last 24 hours)
	fmt.Println("\n=== Fetching Recent Merge Requests ===")
	recentMRs, err := fetcher.FetchRecentMRs(ctx, projectID, 24*time.Hour)
	if err != nil {
		log.Printf("Error fetching recent MRs: %v", err)
	} else {
		fmt.Printf("Found %d recently updated merge requests:\n", len(recentMRs))
		for _, mr := range recentMRs {
			fmt.Printf("- %s (#%d) updated at %s\n", mr.Title, mr.IID, mr.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
	}

	// Example 3: Fetch merge requests by specific author
	fmt.Println("\n=== Fetching Merge Requests by Author ===")
	authorID := "author-user-id" // Replace with actual author ID
	authorMRs, err := fetcher.FetchMRsByAuthor(ctx, projectID, authorID)
	if err != nil {
		log.Printf("Error fetching MRs by author: %v", err)
	} else {
		fmt.Printf("Found %d merge requests by author %s:\n", len(authorMRs), authorID)
		for _, mr := range authorMRs {
			fmt.Printf("- %s (#%d)\n", mr.Title, mr.IID)
		}
	}

	// Example 4: Fetch merge requests targeting main branch created in last week
	fmt.Println("\n=== Fetching Merge Requests to Review ===")
	oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
	options := provider.FetchOptions{
		TargetBranch: "main",
		CreatedSince: &oneWeekAgo,
		Limit:        20,
	}
	options.SetDefaults()

	reviewMRs, err := fetcher.FetchMRsToReview(ctx, projectID, options)
	if err != nil {
		log.Printf("Error fetching MRs to review: %v", err)
	} else {
		fmt.Printf("Found %d merge requests targeting main branch:\n", len(reviewMRs))
		for _, mr := range reviewMRs {
			needsReview := provider.FilterNeedsReview(mr, "your-bot-username")
			status := "✓"
			if !needsReview {
				status = "⏭"
			}
			fmt.Printf("%s %s (#%d) by %s\n", status, mr.Title, mr.IID, mr.Author.Username)
		}
	}

	// Example 5: Batch process merge requests
	fmt.Println("\n=== Batch Processing Merge Requests ===")
	filter := &model.MergeRequestFilter{
		State: []string{"open"},
		Limit: 10,
	}

	processor := func(mr *model.MergeRequest) error {
		fmt.Printf("Processing MR: %s (#%d)\n", mr.Title, mr.IID)

		// Here you could:
		// - Trigger AI review
		// - Check if review is needed
		// - Update status
		// - Send notifications

		return nil
	}

	err = fetcher.BatchProcessMRs(ctx, projectID, filter, processor)
	if err != nil {
		log.Printf("Error in batch processing: %v", err)
	}

	// Example 6: Polling for updates (run for 30 seconds)
	fmt.Println("\n=== Starting Polling for Updates ===")
	pollingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	callback := func(updatedMRs []*model.MergeRequest) {
		fmt.Printf("📥 Found %d updated merge requests:\n", len(updatedMRs))
		for _, mr := range updatedMRs {
			fmt.Printf("  - %s (#%d) updated at %s\n",
				mr.Title, mr.IID, mr.UpdatedAt.Format("15:04:05"))
		}
	}

	err = fetcher.PollForUpdates(pollingCtx, projectID, 10*time.Second, callback)
	if err != nil && err != context.DeadlineExceeded {
		log.Printf("Polling error: %v", err)
	}

	fmt.Println("\n=== Example completed ===")
}

// Example of creating a review processor
func exampleReviewProcessor() {
	// This would typically be injected from your main application
	var reviewService model.ReviewService // = your review service instance

	processor := provider.CreateReviewProcessor(reviewService)

	// Example usage
	mr := &model.MergeRequest{
		ID:    "123",
		IID:   456,
		Title: "Example MR",
		// ... other fields
	}

	if err := processor(mr); err != nil {
		log.Printf("Failed to process review for MR %s: %v", mr.ID, err)
	}
}

// Example of custom filtering
func exampleCustomFiltering(mrs []*model.MergeRequest) []*model.MergeRequest {
	var filtered []*model.MergeRequest

	for _, mr := range mrs {
		// Custom filtering logic
		if shouldIncludeMR(mr) {
			filtered = append(filtered, mr)
		}
	}

	return filtered
}

func shouldIncludeMR(mr *model.MergeRequest) bool {
	// Example criteria:
	// - Skip draft MRs
	// - Skip WIP MRs
	// - Only include MRs with recent activity
	// - Skip MRs that already have enough reviewers

	// Skip if title contains [WIP] or Draft:
	if containsWIP(mr.Title) {
		return false
	}

	// Only include if updated in last 7 days
	if time.Since(mr.UpdatedAt) > 7*24*time.Hour {
		return false
	}

	// Skip if already has 2+ reviewers
	if len(mr.Reviewers) >= 2 {
		return false
	}

	return true
}

func containsWIP(title string) bool {
	wipIndicators := []string{"[WIP]", "WIP:", "Draft:", "[Draft]", "🚧"}
	for _, indicator := range wipIndicators {
		if len(title) > len(indicator) && title[:len(indicator)] == indicator {
			return true
		}
	}
	return false
}
