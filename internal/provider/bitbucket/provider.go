package bitbucket

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

var _ interfaces.CodeProvider = (*Provider)(nil)

const (
	defaultBaseURL = "https://api.bitbucket.org/2.0"
)

// Provider implements the CodeProvider interface for Bitbucket
type Provider struct {
	config model.ProviderConfig
	logger logze.Logger
	client *cliex.HTTP
}

// New creates a new Bitbucket provider
func New(config model.ProviderConfig) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("Bitbucket token is required")
	}
	log := logze.With("provider", "bitbucket")

	// Set base URL
	baseURL := defaultBaseURL
	if config.BaseURL != "" {
		baseURL = strings.TrimSuffix(config.BaseURL, "/")
	}

	cli, err := cliex.New(cliex.WithBaseURL(baseURL), cliex.WithLogger(log))
	if err != nil {
		return nil, errm.Wrap(err, "failed to create Bitbucket client")
	}
	cli.C().SetBasicAuth("x-auth-token", config.Token)

	return &Provider{
		client: cli,
		config: config,
		logger: log,
	}, nil
}

// ValidateWebhook validates the Bitbucket webhook signature
func (p *Provider) ValidateWebhook(payload []byte, signature string) error {
	if p.config.WebhookSecret == "" {
		return nil // No secret configured, skip validation
	}

	// Bitbucket uses HMAC-SHA256 with the full payload
	mac := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Bitbucket might send signature with or without prefix
	cleanSignature := strings.TrimPrefix(signature, "sha256=")

	if !hmac.Equal([]byte(expectedSignature), []byte(cleanSignature)) {
		return errm.New("Bitbucket webhook signature verification failed")
	}

	return nil
}

// ParseWebhookEvent parses a Bitbucket webhook event
func (p *Provider) ParseWebhookEvent(payload []byte) (*model.CodeEvent, error) {
	var bitbucketPayload bitbucketPayload
	if err := json.Unmarshal(payload, &bitbucketPayload); err != nil {
		return nil, errm.Wrap(err, "failed to parse Bitbucket webhook payload")
	}

	// Detect event type from headers or payload
	eventType := "pullrequest"
	action := "unknown"

	// Try to determine action from the payload structure
	if len(bitbucketPayload.Changes) > 0 {
		action = "updated"
	} else if bitbucketPayload.PullRequest.State == "OPEN" {
		action = "opened"
	} else if bitbucketPayload.PullRequest.State == "MERGED" {
		action = "merged"
	} else if bitbucketPayload.PullRequest.State == "DECLINED" {
		action = "declined"
	}

	// Map Bitbucket reviewers to our format
	var reviewers []model.User
	for _, reviewer := range bitbucketPayload.PullRequest.Reviewers {
		reviewers = append(reviewers, model.User{
			ID:       reviewer.User.UUID,
			Username: reviewer.User.Username,
			Name:     reviewer.User.DisplayName,
		})
	}

	event := &model.CodeEvent{
		Type:      eventType,
		Action:    action,
		ProjectID: bitbucketPayload.Repository.FullName, // Format: workspace/repo_slug
		User: &model.User{
			ID:       bitbucketPayload.Actor.UUID,
			Username: bitbucketPayload.Actor.Username,
			Name:     bitbucketPayload.Actor.DisplayName,
		},
		MergeRequest: &model.MergeRequest{
			ID:           strconv.Itoa(bitbucketPayload.PullRequest.ID),
			IID:          bitbucketPayload.PullRequest.ID,
			Title:        bitbucketPayload.PullRequest.Title,
			Description:  bitbucketPayload.PullRequest.Description,
			SourceBranch: bitbucketPayload.PullRequest.Source.Branch.Name,
			TargetBranch: bitbucketPayload.PullRequest.Destination.Branch.Name,
			URL:          bitbucketPayload.PullRequest.Links.HTML.Href,
			State:        strings.ToLower(bitbucketPayload.PullRequest.State),
			SHA:          bitbucketPayload.PullRequest.Source.Commit.Hash,
			Author: model.User{
				ID:       bitbucketPayload.PullRequest.Author.UUID,
				Username: bitbucketPayload.PullRequest.Author.Username,
				Name:     bitbucketPayload.PullRequest.Author.DisplayName,
			},
			Reviewers: reviewers,
		},
	}

	return event, nil
}

// GetMergeRequest retrieves detailed information about a pull request
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*model.MergeRequest, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d", workspace, repoSlug, mrIID)

	var pr bitbucketPullRequest
	_, err := p.client.Get(ctx, apiURL, &pr)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get pull request from Bitbucket")
	}

	// Convert reviewers
	var reviewers []model.User
	for _, reviewer := range pr.Reviewers {
		reviewers = append(reviewers, model.User{
			ID:       reviewer.User.UUID,
			Username: reviewer.User.Username,
			Name:     reviewer.User.DisplayName,
		})
	}

	// Parse timestamps
	createdAt, _ := time.Parse(time.RFC3339, pr.CreatedOn)
	updatedAt, _ := time.Parse(time.RFC3339, pr.UpdatedOn)

	mergeRequest := &model.MergeRequest{
		ID:           strconv.Itoa(pr.ID),
		IID:          pr.ID,
		Title:        pr.Title,
		Description:  pr.Description,
		SourceBranch: pr.Source.Branch.Name,
		TargetBranch: pr.Destination.Branch.Name,
		URL:          pr.Links.HTML.Href,
		State:        strings.ToLower(pr.State),
		SHA:          pr.Source.Commit.Hash,
		Author: model.User{
			ID:       pr.Author.UUID,
			Username: pr.Author.Username,
			Name:     pr.Author.DisplayName,
		},
		Reviewers: reviewers,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	return mergeRequest, nil
}

// GetMergeRequestDiffs retrieves the diff for a pull request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*model.FileDiff, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for diff
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/diff", workspace, repoSlug, mrIID)

	resp, err := p.client.Get(ctx, apiURL)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get diff from Bitbucket")
	}

	// Parse diff into FileDiff objects
	diffs := p.parseDiffContent(string(resp.Body()))

	return diffs, nil
}

// UpdateMergeRequestDescription updates the pull request description
func (p *Provider) UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d", workspace, repoSlug, mrIID)

	// Prepare request body
	updateData := map[string]any{
		"description": description,
	}

	_, err := p.client.Put(ctx, apiURL, updateData)
	if err != nil {
		return errm.Wrap(err, "failed to update pull request description")
	}

	return nil
}

// CreateComment creates a comment on the pull request
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, mrIID)

	// Prepare comment data
	commentData := map[string]any{
		"content": map[string]any{
			"raw": comment.Body,
		},
	}

	// Add inline comment data if file path and line are specified
	if comment.FilePath != "" && comment.Line > 0 {
		commentData["inline"] = map[string]any{
			"path": comment.FilePath,
			"to":   comment.Line,
		}
	}

	_, err := p.client.Post(ctx, apiURL, commentData)
	if err != nil {
		return errm.Wrap(err, "failed to create comment")
	}

	return nil
}

// GetComments retrieves comments from a pull request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, mrIID)

	var response bitbucketCommentsResponse
	_, err := p.client.Get(ctx, apiURL, &response)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comments from Bitbucket")
	}

	var comments []*model.Comment
	for _, comment := range response.Values {
		reviewComment := &model.Comment{
			ID:       strconv.Itoa(comment.ID),
			Body:     comment.Content.Raw,
			FilePath: comment.Inline.Path,
			Line:     comment.Inline.To,
			Author: model.User{
				ID:       comment.User.UUID,
				Username: comment.User.Username,
				Name:     comment.User.DisplayName,
			},
		}
		comments = append(comments, reviewComment)
	}

	return comments, nil
}

// GetCurrentUser retrieves the current authenticated user
func (p *Provider) GetCurrentUser(ctx context.Context) (*model.User, error) {
	// Build API URL
	apiURL := "user"

	var user bitbucketUser
	_, err := p.client.Get(ctx, apiURL, &user)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user from Bitbucket")
	}

	return &model.User{
		ID:       user.UUID,
		Username: user.Username,
		Name:     user.DisplayName,
	}, nil
}

// parseDiffContent parses unified diff content into FileDiff objects
func (p *Provider) parseDiffContent(diffContent string) []*model.FileDiff {
	var diffs []*model.FileDiff
	lines := strings.Split(diffContent, "\n")

	var currentDiff *model.FileDiff
	var diffLines []string

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			// Save previous diff if exists
			if currentDiff != nil {
				currentDiff.Diff = strings.Join(diffLines, "\n")
				diffs = append(diffs, currentDiff)
			}

			// Start new diff
			currentDiff = &model.FileDiff{}
			diffLines = []string{line}

		case strings.HasPrefix(line, "--- ") && currentDiff != nil:
			// Old file path
			if strings.Contains(line, "/dev/null") {
				currentDiff.IsNew = true
			} else {
				path := strings.TrimPrefix(line, "--- a/")
				if path != "" {
					currentDiff.OldPath = path
				}
			}
			diffLines = append(diffLines, line)

		case strings.HasPrefix(line, "+++ ") && currentDiff != nil:
			// New file path
			if strings.Contains(line, "/dev/null") {
				currentDiff.IsDeleted = true
			} else {
				path := strings.TrimPrefix(line, "+++ b/")
				if path != "" {
					currentDiff.NewPath = path
				}
			}
			diffLines = append(diffLines, line)

		case currentDiff != nil:
			diffLines = append(diffLines, line)
		}
	}

	// Add the last diff
	if currentDiff != nil {
		currentDiff.Diff = strings.Join(diffLines, "\n")
		diffs = append(diffs, currentDiff)
	}

	// Set default paths and detect renames
	for _, diff := range diffs {
		if diff.NewPath == "" && diff.OldPath != "" {
			diff.NewPath = diff.OldPath
		}
		if diff.OldPath == "" && diff.NewPath != "" {
			diff.OldPath = diff.NewPath
		}
		if diff.OldPath != "" && diff.NewPath != "" && diff.OldPath != diff.NewPath {
			diff.IsRenamed = true
		}
	}

	return diffs
}

// IsMergeRequestEvent determines if a webhook event is a pull request event that should be processed
func (p *Provider) IsMergeRequestEvent(event *model.CodeEvent) bool {
	// Only process pull request events (Bitbucket calls them pullrequest)
	if event.Type != "pullrequest" {
		p.logger.Debug("ignoring non-pull request event", "event_type", event.Type)
		return false
	}

	// Check for relevant actions
	relevantActions := []string{
		"opened",         // When PR is opened
		"updated",        // When PR is updated
		"created",        // Alternative for opened
		"reviewer_added", // When reviewer is added
	}

	isRelevantAction := slices.Contains(relevantActions, event.Action)

	if !isRelevantAction {
		p.logger.Debug("ignoring irrelevant action", "action", event.Action)
		return false
	}

	// Don't process events from the bot itself to avoid loops
	if event.User.Username == p.config.BotUsername {
		p.logger.Debug("ignoring event from bot user")
		return false
	}

	// Special handling for reviewer-based triggers
	if event.Action == "reviewer_added" {
		// Check if the bot was added as a reviewer
		botIsReviewer := false
		for _, reviewer := range event.MergeRequest.Reviewers {
			if reviewer.Username == p.config.BotUsername {
				botIsReviewer = true
				break
			}
		}

		if !botIsReviewer {
			p.logger.Debug("bot not in reviewers list for reviewer_added action")
			return false
		}

		p.logger.Info("bot was added as reviewer, triggering review")
		return true
	}

	// Skip closed/merged/declined pull requests for review
	if event.Action == "merged" || event.Action == "declined" {
		p.logger.Debug("pull request closed/merged/declined, skipping review", "action", event.Action)
		return false
	}

	p.logger.Debug("pull request event should be processed", "action", event.Action)
	return true
}

// IsCommentEvent determines if a webhook event is a comment event that should be processed
func (p *Provider) IsCommentEvent(event *model.CodeEvent) bool {
	return event.Type == "comment" || event.Type == "pullrequest:comment_created"
}

// ReplyToComment replies to an existing comment
func (p *Provider) ReplyToComment(ctx context.Context, projectID string, mrIID int, commentID string, reply string) error {
	// Bitbucket doesn't have threaded comments, so we create a new comment with reference
	comment := &model.Comment{
		Body: fmt.Sprintf("Reply to comment %s: %s", commentID, reply),
	}
	return p.CreateComment(ctx, projectID, mrIID, comment)
}

// GetComment retrieves a specific comment
func (p *Provider) GetComment(ctx context.Context, projectID string, mrIID int, commentID string) (*model.Comment, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for specific comment
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/comments/%s", workspace, repoSlug, mrIID, commentID)

	var comment bitbucketCommentResponse
	_, err := p.client.Get(ctx, apiURL, &comment)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comment")
	}

	// Parse timestamps
	createdAt, _ := time.Parse(time.RFC3339, comment.CreatedOn)
	updatedAt, _ := time.Parse(time.RFC3339, comment.UpdatedOn)

	return &model.Comment{
		ID:   strconv.Itoa(comment.ID),
		Body: comment.Content.Raw,
		Author: model.User{
			ID:       comment.User.UUID,
			Username: comment.User.Username,
			Name:     comment.User.DisplayName,
		},
		FilePath:  comment.Inline.Path,
		Line:      comment.Inline.To,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// ListMergeRequests retrieves multiple pull requests based on filter criteria
func (p *Provider) ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL with query parameters
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests", workspace, repoSlug)

	// Add query parameters
	params := make(map[string]string)

	if len(filter.State) > 0 {
		// Bitbucket uses "OPEN", "MERGED", "DECLINED"
		state := strings.ToUpper(filter.State[0])
		params["state"] = state
	}

	if filter.Limit > 0 {
		params["pagelen"] = strconv.Itoa(filter.Limit)
	}

	if filter.Page > 0 {
		params["page"] = strconv.Itoa(filter.Page + 1) // Bitbucket uses 1-based pagination
	}

	// Build query string
	var queryParts []string
	for key, value := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", key, value))
	}
	if len(queryParts) > 0 {
		apiURL += "?" + strings.Join(queryParts, "&")
	}

	var response struct {
		Values []bitbucketPullRequest `json:"values"`
	}

	_, err := p.client.Get(ctx, apiURL, &response)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list pull requests")
	}

	var result []*model.MergeRequest
	for _, pr := range response.Values {
		// Apply filters that can't be done at API level
		if filter.AuthorID != "" && pr.Author.UUID != filter.AuthorID {
			continue
		}

		if filter.TargetBranch != "" && pr.Destination.Branch.Name != filter.TargetBranch {
			continue
		}

		if filter.SourceBranch != "" && pr.Source.Branch.Name != filter.SourceBranch {
			continue
		}

		// Parse timestamps for filtering
		updatedAt, _ := time.Parse(time.RFC3339, pr.UpdatedOn)
		createdAt, _ := time.Parse(time.RFC3339, pr.CreatedOn)

		if filter.UpdatedAfter != nil && updatedAt.Before(*filter.UpdatedAfter) {
			continue
		}

		if filter.CreatedAfter != nil && createdAt.Before(*filter.CreatedAfter) {
			continue
		}

		// Convert reviewers
		var reviewers []model.User
		for _, reviewer := range pr.Reviewers {
			reviewers = append(reviewers, model.User{
				ID:       reviewer.User.UUID,
				Username: reviewer.User.Username,
				Name:     reviewer.User.DisplayName,
			})
		}

		modelMR := &model.MergeRequest{
			ID:           strconv.Itoa(pr.ID),
			IID:          pr.ID,
			Title:        pr.Title,
			Description:  pr.Description,
			SourceBranch: pr.Source.Branch.Name,
			TargetBranch: pr.Destination.Branch.Name,
			URL:          pr.Links.HTML.Href,
			State:        strings.ToLower(pr.State),
			SHA:          pr.Source.Commit.Hash,
			Author: model.User{
				ID:       pr.Author.UUID,
				Username: pr.Author.Username,
				Name:     pr.Author.DisplayName,
			},
			Reviewers: reviewers,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		result = append(result, modelMR)
	}

	return result, nil
}

// GetMergeRequestUpdates retrieves pull requests updated since a specific time
func (p *Provider) GetMergeRequestUpdates(ctx context.Context, projectID string, since time.Time) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		UpdatedAfter: &since,
		State:        []string{"open"}, // Only get open PRs for updates
		Limit:        100,              // Reasonable default
	}

	return p.ListMergeRequests(ctx, projectID, filter)
}
