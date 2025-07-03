package github

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

	"github.com/google/go-github/v57/github"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"golang.org/x/oauth2"
)

var _ model.CodeProvider = (*Provider)(nil)

const (
	defaultBaseURL = "https://github.com"
)

// Provider implements the CodeProvider interface for GitHub
type Provider struct {
	client *github.Client
	config model.ProviderConfig
	logger logze.Logger
}

// NewProvider creates a new GitHub provider
func NewProvider(config model.ProviderConfig) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("GitHub token is required")
	}
	log := logze.With("provider", "github")

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	// Create GitHub client
	client := github.NewClient(tc)

	// Set base URL if provided (for GitHub Enterprise)
	if config.BaseURL != "" && config.BaseURL != defaultBaseURL {
		var err error
		client, err = github.NewClient(tc).WithEnterpriseURLs(config.BaseURL, config.BaseURL)
		if err != nil {
			return nil, errm.Wrap(err, "failed to create Git	Hub Enterprise client")
		}
	}

	return &Provider{
		client: client,
		config: config,
		logger: log,
	}, nil
}

// ValidateWebhook validates the GitHub webhook signature
func (p *Provider) ValidateWebhook(payload []byte, signature string) error {
	if p.config.WebhookSecret == "" {
		return nil // No secret configured, skip validation
	}

	// GitHub signature format: "sha256=<signature>"
	if !strings.HasPrefix(signature, "sha256=") {
		return errm.New("invalid GitHub signature format")
	}

	// Extract the signature
	expectedSignature := strings.TrimPrefix(signature, "sha256=")

	// Calculate HMAC
	mac := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	mac.Write(payload)
	calculatedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(expectedSignature), []byte(calculatedSignature)) {
		return errm.New("GitHub webhook signature verification failed")
	}

	return nil
}

// ParseWebhookEvent parses a GitHub webhook event
func (p *Provider) ParseWebhookEvent(payload []byte) (*model.CodeEvent, error) {
	var githubPayload githubPayload
	if err := json.Unmarshal(payload, &githubPayload); err != nil {
		return nil, errm.Wrap(err, "failed to parse GitHub webhook payload")
	}

	// Convert reviewers
	var reviewers []*model.User
	for _, reviewer := range githubPayload.PullRequest.RequestedReviewers {
		reviewers = append(reviewers, &model.User{
			ID:       strconv.Itoa(reviewer.ID),
			Username: reviewer.Login,
			Name:     reviewer.Name,
		})
	}

	event := &model.CodeEvent{
		Type:      "pull_request",
		Action:    githubPayload.Action,
		ProjectID: githubPayload.Repository.FullName, // GitHub uses "owner/repo" format
		User: &model.User{
			ID:       strconv.Itoa(githubPayload.Sender.ID),
			Username: githubPayload.Sender.Login,
			Name:     githubPayload.Sender.Name,
			Email:    githubPayload.Sender.Email,
		},
		MergeRequest: &model.MergeRequest{
			ID:           strconv.Itoa(githubPayload.PullRequest.ID),
			IID:          githubPayload.PullRequest.Number,
			Title:        githubPayload.PullRequest.Title,
			Description:  githubPayload.PullRequest.Body,
			SourceBranch: githubPayload.PullRequest.Head.Ref,
			TargetBranch: githubPayload.PullRequest.Base.Ref,
			URL:          githubPayload.PullRequest.HTMLURL,
			State:        githubPayload.PullRequest.State,
			SHA:          githubPayload.PullRequest.Head.SHA,
			AuthorID:     strconv.Itoa(githubPayload.PullRequest.User.ID),
			Author: &model.User{
				ID:       strconv.Itoa(githubPayload.PullRequest.User.ID),
				Username: githubPayload.PullRequest.User.Login,
				Name:     githubPayload.PullRequest.User.Name,
				Email:    githubPayload.PullRequest.User.Email,
			},
			Reviewers: reviewers,
		},
	}

	return event, nil
}

// GetMergeRequest retrieves detailed information about a pull request
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*model.MergeRequest, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Get pull request
	pr, _, err := p.client.PullRequests.Get(ctx, owner, repo, mrIID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get pull request from GitHub")
	}

	// Get requested reviewers
	var reviewers []*model.User
	if pr.RequestedReviewers != nil {
		for _, reviewer := range pr.RequestedReviewers {
			reviewers = append(reviewers, &model.User{
				ID:       strconv.FormatInt(*reviewer.ID, 10),
				Username: *reviewer.Login,
				Name:     reviewer.GetName(),
			})
		}
	}

	return &model.MergeRequest{
		ID:           strconv.FormatInt(*pr.ID, 10),
		IID:          *pr.Number,
		Title:        *pr.Title,
		Description:  pr.GetBody(),
		SourceBranch: *pr.Head.Ref,
		TargetBranch: *pr.Base.Ref,
		URL:          *pr.HTMLURL,
		State:        *pr.State,
		SHA:          *pr.Head.SHA,
		AuthorID:     strconv.FormatInt(*pr.User.ID, 10),
		Author: &model.User{
			ID:       strconv.FormatInt(*pr.User.ID, 10),
			Username: *pr.User.Login,
			Name:     pr.User.GetName(),
		},
		Reviewers: reviewers,
		CreatedAt: pr.GetCreatedAt().Time,
		UpdatedAt: pr.GetUpdatedAt().Time,
	}, nil
}

// GetMergeRequestDiffs retrieves the file diffs for a pull request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*model.FileDiff, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Get pull request files
	opts := &github.ListOptions{PerPage: 100}
	var allFiles []*github.CommitFile

	for {
		files, resp, err := p.client.PullRequests.ListFiles(ctx, owner, repo, mrIID, opts)
		if err != nil {
			return nil, errm.Wrap(err, "failed to list pull request files")
		}

		allFiles = append(allFiles, files...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Convert to our models
	var fileDiffs []*model.FileDiff
	for _, file := range allFiles {
		fileDiff := &model.FileDiff{
			OldPath:   file.GetPreviousFilename(),
			NewPath:   file.GetFilename(),
			Diff:      file.GetPatch(),
			IsNew:     file.GetStatus() == "added",
			IsDeleted: file.GetStatus() == "removed",
			IsRenamed: file.GetStatus() == "renamed",
			IsBinary:  file.GetPatch() == "" && file.GetStatus() != "removed" && file.GetStatus() != "added",
		}

		// Handle renamed files
		if fileDiff.IsRenamed && fileDiff.OldPath == "" {
			fileDiff.OldPath = fileDiff.NewPath
		}

		fileDiffs = append(fileDiffs, fileDiff)
	}

	return fileDiffs, nil
}

// UpdateMergeRequestDescription updates the description of a pull request
func (p *Provider) UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Update pull request
	updateRequest := &github.PullRequest{
		Body: &description,
	}

	_, _, err := p.client.PullRequests.Edit(ctx, owner, repo, mrIID, updateRequest)
	if err != nil {
		return errm.Wrap(err, "failed to update pull request description")
	}

	return nil
}

// CreateComment creates a comment on a pull request
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.ReviewComment) error {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Create issue comment (GitHub treats PR comments as issue comments)
	githubComment := &github.IssueComment{
		Body: &comment.Body,
	}

	createdComment, _, err := p.client.Issues.CreateComment(ctx, owner, repo, mrIID, githubComment)
	if err != nil {
		return errm.Wrap(err, "failed to create pull request comment")
	}

	// Update comment with the created ID
	comment.ID = strconv.FormatInt(*createdComment.ID, 10)

	return nil
}

// GetComments retrieves all comments for a pull request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.ReviewComment, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Get issue comments (GitHub treats PR comments as issue comments)
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allComments []*github.IssueComment
	for {
		comments, resp, err := p.client.Issues.ListComments(ctx, owner, repo, mrIID, opts)
		if err != nil {
			return nil, errm.Wrap(err, "failed to list pull request comments")
		}

		allComments = append(allComments, comments...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Convert to our models
	var reviewComments []*model.ReviewComment
	for _, comment := range allComments {
		reviewComment := &model.ReviewComment{
			ID:   strconv.FormatInt(*comment.ID, 10),
			Body: *comment.Body,
			Author: &model.User{
				ID:       strconv.FormatInt(*comment.User.ID, 10),
				Username: *comment.User.Login,
				Name:     comment.User.GetName(),
			},
		}
		reviewComments = append(reviewComments, reviewComment)
	}

	return reviewComments, nil
}

// GetCurrentUser retrieves information about the current authenticated user
func (p *Provider) GetCurrentUser(ctx context.Context) (*model.User, error) {
	user, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user")
	}

	return &model.User{
		ID:       strconv.FormatInt(*user.ID, 10),
		Username: *user.Login,
		Name:     user.GetName(),
		Email:    user.GetEmail(),
	}, nil
}

// IsMergeRequestEvent determines if a webhook event is a merge request event that should be processed
func (p *Provider) IsMergeRequestEvent(event *model.CodeEvent) bool {
	// Only process pull request events
	if event.Type != "pull_request" {
		p.logger.Debug("ignoring non-pull request event", "event_type", event.Type)
		return false
	}

	// Check for relevant actions
	relevantActions := []string{
		"opened",           // When PR is opened
		"reopened",         // When PR is reopened
		"synchronize",      // When PR is updated with new commits
		"review_requested", // When reviewer is added
		"ready_for_review", // When PR is marked ready for review
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
	if event.Action == "review_requested" {
		// Check if the bot was added as a reviewer
		botIsReviewer := false
		for _, reviewer := range event.MergeRequest.Reviewers {
			if reviewer.Username == p.config.BotUsername {
				botIsReviewer = true
				break
			}
		}

		if !botIsReviewer {
			p.logger.Debug("bot not in reviewers list for review_requested action")
			return false
		}

		p.logger.Info("bot was added as reviewer, triggering review")
		return true
	}

	p.logger.Debug("pull request event should be processed", "action", event.Action)
	return true
}

// IsCommentEvent determines if a webhook event is a comment event that should be processed
func (p *Provider) IsCommentEvent(event *model.CodeEvent) bool {
	return event.Type == "issue_comment" || event.Type == "pull_request_review_comment"
}

// ReplyToComment replies to an existing comment
func (p *Provider) ReplyToComment(ctx context.Context, projectID string, mrIID int, commentID string, reply string) error {
	// GitHub doesn't have threaded comments, so we create a new comment with reference
	comment := &model.ReviewComment{
		Body: fmt.Sprintf("Reply to comment %s: %s", commentID, reply),
	}
	return p.CreateComment(ctx, projectID, mrIID, comment)
}

// GetComment retrieves a specific comment
func (p *Provider) GetComment(ctx context.Context, projectID string, mrIID int, commentID string) (*model.Comment, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	commentIDInt, err := strconv.ParseInt(commentID, 10, 64)
	if err != nil {
		return nil, errm.Wrap(err, "invalid comment ID")
	}

	comment, _, err := p.client.Issues.GetComment(ctx, owner, repo, commentIDInt)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comment")
	}

	return &model.Comment{
		ID:       strconv.FormatInt(*comment.ID, 10),
		Body:     *comment.Body,
		AuthorID: strconv.FormatInt(*comment.User.ID, 10),
		Author: &model.User{
			ID:       strconv.FormatInt(*comment.User.ID, 10),
			Username: *comment.User.Login,
			Name:     comment.User.GetName(),
		},
		CreatedAt: comment.GetCreatedAt().Time,
		UpdatedAt: comment.GetUpdatedAt().Time,
	}, nil
}

// ListMergeRequests retrieves multiple pull requests based on filter criteria
func (p *Provider) ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	opts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{
			Page:    filter.Page + 1, // GitHub uses 1-based pagination
			PerPage: filter.Limit,
		},
	}

	// Apply filters
	if len(filter.State) > 0 {
		// GitHub uses "open", "closed", "all"
		opts.State = filter.State[0]
	}

	if filter.TargetBranch != "" {
		opts.Base = filter.TargetBranch
	}

	if filter.SourceBranch != "" {
		opts.Head = filter.SourceBranch
	}

	// GitHub doesn't support author filter in list API, so we'll filter afterward
	prs, _, err := p.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list pull requests")
	}

	var result []*model.MergeRequest
	for _, pr := range prs {
		// Apply author filter if specified
		if filter.AuthorID != "" {
			authorID := strconv.FormatInt(*pr.User.ID, 10)
			if authorID != filter.AuthorID {
				continue
			}
		}

		// Apply time filters
		if filter.UpdatedAfter != nil && pr.GetUpdatedAt().Time.Before(*filter.UpdatedAfter) {
			continue
		}

		if filter.CreatedAfter != nil && pr.GetCreatedAt().Time.Before(*filter.CreatedAfter) {
			continue
		}

		// Get requested reviewers
		var reviewers []*model.User
		if pr.RequestedReviewers != nil {
			for _, reviewer := range pr.RequestedReviewers {
				reviewers = append(reviewers, &model.User{
					ID:       strconv.FormatInt(*reviewer.ID, 10),
					Username: *reviewer.Login,
					Name:     reviewer.GetName(),
				})
			}
		}

		modelMR := &model.MergeRequest{
			ID:           strconv.FormatInt(*pr.ID, 10),
			IID:          *pr.Number,
			Title:        *pr.Title,
			Description:  pr.GetBody(),
			SourceBranch: *pr.Head.Ref,
			TargetBranch: *pr.Base.Ref,
			URL:          *pr.HTMLURL,
			State:        *pr.State,
			SHA:          *pr.Head.SHA,
			AuthorID:     strconv.FormatInt(*pr.User.ID, 10),
			Author: &model.User{
				ID:       strconv.FormatInt(*pr.User.ID, 10),
				Username: *pr.User.Login,
				Name:     pr.User.GetName(),
			},
			Reviewers: reviewers,
			CreatedAt: pr.GetCreatedAt().Time,
			UpdatedAt: pr.GetUpdatedAt().Time,
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
