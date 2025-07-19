package bitbucket

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/erro"
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
		return nil, erro.New("Bitbucket token is required")
	}
	log := logze.With("provider", "bitbucket", "component", "provider")

	// Set base URL
	baseURL := defaultBaseURL
	if config.BaseURL != "" {
		baseURL = strings.TrimSuffix(config.BaseURL, "/")
	}

	cli, err := cliex.New(cliex.WithBaseURL(baseURL), cliex.WithLogger(log))
	if err != nil {
		return nil, erro.Wrap(err, "failed to create Bitbucket client")
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
		return erro.New("Bitbucket webhook signature verification failed")
	}

	return nil
}

// ParseWebhookEvent parses a Bitbucket webhook event
func (p *Provider) ParseWebhookEvent(payload []byte) (*model.CodeEvent, error) {
	var bitbucketPayload bitbucketPayload
	if err := json.Unmarshal(payload, &bitbucketPayload); err != nil {
		return nil, erro.Wrap(err, "failed to parse Bitbucket webhook payload")
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
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d", workspace, repoSlug, mrIID)

	var pr bitbucketPullRequest
	_, err := p.client.Get(ctx, apiURL, &pr)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get pull request from Bitbucket")
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
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for diff
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/diff", workspace, repoSlug, mrIID)

	resp, err := p.client.Get(ctx, apiURL)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get diff from Bitbucket")
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
		return erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
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
		return erro.Wrap(err, "failed to update pull request description")
	}

	return nil
}

// CreateComment creates a comment on the pull request
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
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
		inlineData := map[string]any{
			"path": comment.FilePath,
			"to":   comment.Line,
		}

		// Handle range comments if this is a review comment
		if (comment.Type == model.CommentTypeReview || comment.Type == model.CommentTypeInline) && p.isRangeComment(comment.Body) {
			startLine, endLine := p.extractLineRange(comment.Body)
			if startLine > 0 && endLine > startLine {
				// Bitbucket supports range comments with from/to
				inlineData["from"] = startLine
				inlineData["to"] = endLine
			}
		}

		commentData["inline"] = inlineData
	}

	_, err := p.client.Post(ctx, apiURL, commentData)
	if err != nil {
		return erro.Wrap(err, "failed to create comment")
	}

	return nil
}

// isRangeComment checks if a comment body indicates it's a range comment
func (p *Provider) isRangeComment(body string) bool {
	return strings.Contains(body, "*(lines ") && strings.Contains(body, "-")
}

// extractLineRange extracts start and end line numbers from comment body
func (p *Provider) extractLineRange(body string) (int, int) {
	// Look for pattern: *(lines 19-32)*
	re := regexp.MustCompile(`\*\(lines (\d+)-(\d+)\)\*`)
	matches := re.FindStringSubmatch(body)

	if len(matches) >= 3 {
		startLine, _ := strconv.Atoi(matches[1])
		endLine, _ := strconv.Atoi(matches[2])
		return startLine, endLine
	}

	return 0, 0
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
		return false
	}

	// Don't process events from the bot itself to avoid loops
	if event.User.Username == p.config.BotUsername {
		return false
	}

	// TODO: bad logic

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
			return false
		}

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

// ListMergeRequests retrieves multiple pull requests based on filter criteria
func (p *Provider) ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
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
		return nil, erro.Wrap(err, "failed to list pull requests")
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

// GetFileContent retrieves the content of a file at a specific commit/SHA
func (p *Provider) GetFileContent(ctx context.Context, projectID, filePath, commitSHA string) (string, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return "", erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for file content at specific commit
	apiURL := fmt.Sprintf("repositories/%s/%s/src/%s/%s", workspace, repoSlug, commitSHA, filePath)

	resp, err := p.client.Get(ctx, apiURL)
	if err != nil {
		return "", erro.Wrap(err, "failed to get file content from Bitbucket")
	}

	return string(resp.Body()), nil
}

// GetComments retrieves all comments for a pull request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, mrIID)

	var response struct {
		Values []bitbucketComment `json:"values"`
	}

	_, err := p.client.Get(ctx, apiURL, &response)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get comments from Bitbucket")
	}

	var allComments []*model.Comment

	for _, comment := range response.Values {
		modelComment := &model.Comment{
			ID:   strconv.Itoa(comment.ID),
			Body: comment.Content.Raw,
			Author: model.User{
				ID:       comment.User.UUID,
				Username: comment.User.Username,
				Name:     comment.User.DisplayName,
			},
		}

		// Parse timestamps
		if createdAt, err := time.Parse(time.RFC3339, comment.CreatedOn); err == nil {
			modelComment.CreatedAt = createdAt
		}
		if updatedAt, err := time.Parse(time.RFC3339, comment.UpdatedOn); err == nil {
			modelComment.UpdatedAt = updatedAt
		}

		// Determine comment type based on inline data
		if comment.Inline.Path != "" {
			modelComment.Type = model.CommentTypeInline
			modelComment.FilePath = comment.Inline.Path
			modelComment.Line = comment.Inline.To
		} else {
			modelComment.Type = model.CommentTypeGeneral
		}

		allComments = append(allComments, modelComment)
	}

	return allComments, nil
}

// GetMergeRequestCommits retrieves all commits for a pull request
func (p *Provider) GetMergeRequestCommits(ctx context.Context, projectID string, mrIID int) ([]*model.Commit, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for pull request commits
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/commits", workspace, repoSlug, mrIID)

	var response struct {
		Values []bitbucketCommit `json:"values"`
	}

	_, err := p.client.Get(ctx, apiURL, &response)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get pull request commits from Bitbucket")
	}

	// Convert to our model
	var modelCommits []*model.Commit
	for _, commit := range response.Values {
		modelCommit := p.convertBitbucketCommit(commit)
		modelCommits = append(modelCommits, modelCommit)
	}

	return modelCommits, nil
}

// GetCommitDetails retrieves detailed information about a specific commit
func (p *Provider) GetCommitDetails(ctx context.Context, projectID, commitSHA string) (*model.Commit, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for commit details
	apiURL := fmt.Sprintf("repositories/%s/%s/commit/%s", workspace, repoSlug, commitSHA)

	var commit bitbucketCommit
	_, err := p.client.Get(ctx, apiURL, &commit)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get commit details from Bitbucket")
	}

	return p.convertBitbucketCommit(commit), nil
}

// GetCommitDiffs retrieves file diffs for a specific commit
func (p *Provider) GetCommitDiffs(ctx context.Context, projectID, commitSHA string) ([]*model.FileDiff, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for commit diff
	apiURL := fmt.Sprintf("repositories/%s/%s/diff/%s", workspace, repoSlug, commitSHA)

	resp, err := p.client.Get(ctx, apiURL)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get commit diff from Bitbucket")
	}

	// Parse diff into FileDiff objects
	diffs := p.parseDiffContent(string(resp.Body()))

	return diffs, nil
}

// convertBitbucketCommit converts a Bitbucket commit to our model
func (p *Provider) convertBitbucketCommit(commit bitbucketCommit) *model.Commit {
	modelCommit := &model.Commit{
		SHA: commit.Hash,
		URL: commit.Links.HTML.Href,
	}

	// Parse commit message into subject and body
	if commit.Message != "" {
		p.parseCommitMessage(commit.Message, modelCommit)
	}

	// Set author information
	modelCommit.Author = model.User{
		ID:       commit.Author.UUID,
		Username: commit.Author.Username,
		Name:     commit.Author.DisplayName,
	}

	// Bitbucket doesn't distinguish between author and committer in their API
	modelCommit.Committer = modelCommit.Author

	// Parse timestamp
	if timestamp, err := time.Parse(time.RFC3339, commit.Date); err == nil {
		modelCommit.Timestamp = timestamp
	}

	// Bitbucket doesn't provide commit stats directly in the commit object
	// Would need to fetch diff or use a separate API call
	modelCommit.Stats = model.CommitStats{
		TotalFiles: 0,
		Additions:  0,
		Deletions:  0,
	}

	return modelCommit
}

// parseCommitMessage parses a commit message into subject and body (shared method)
func (p *Provider) parseCommitMessage(message string, commit *model.Commit) {
	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return
	}

	// First line is the subject
	commit.Subject = strings.TrimSpace(lines[0])

	// Rest is the body (skip empty line after subject if present)
	if len(lines) > 1 {
		bodyLines := lines[1:]
		// Skip the first line if it's empty (common convention)
		if len(bodyLines) > 0 && strings.TrimSpace(bodyLines[0]) == "" {
			bodyLines = bodyLines[1:]
		}

		if len(bodyLines) > 0 {
			commit.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		}
	}
}

// UpdateComment updates an existing comment
func (p *Provider) UpdateComment(ctx context.Context, projectID string, mrIID int, commentID string, newBody string) error {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("repositories/%s/%s/pullrequests/%d/comments/%s", workspace, repoSlug, mrIID, commentID)

	// Prepare update data
	updateData := map[string]any{
		"content": map[string]any{
			"raw": newBody,
		},
	}

	_, err := p.client.Put(ctx, apiURL, updateData)
	if err != nil {
		return erro.Wrap(err, "failed to update comment")
	}

	return nil
}

// GetRepositoryInfo retrieves comprehensive repository information
func (p *Provider) GetRepositoryInfo(ctx context.Context, projectID string) (*model.RepositoryInfo, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Get repository details
	repoURL := fmt.Sprintf("repositories/%s/%s", workspace, repoSlug)
	var repository bitbucketRepository
	_, err := p.client.Get(ctx, repoURL, &repository)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get repository from Bitbucket")
	}

	// Get branches
	branchesURL := fmt.Sprintf("repositories/%s/%s/refs/branches", workspace, repoSlug)
	var branchesResponse struct {
		Values []bitbucketBranch `json:"values"`
	}
	_, err = p.client.Get(ctx, branchesURL, &branchesResponse)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get branches from Bitbucket")
	}

	// Convert branches
	var branchInfos []model.BranchInfo
	for _, branch := range branchesResponse.Values {
		branchInfo := model.BranchInfo{
			Name:      branch.Name,
			SHA:       branch.Target.Hash,
			Protected: false, // Bitbucket doesn't provide this info in branch listing
			Default:   branch.Name == repository.MainBranch.Name,
		}

		// Parse timestamp if available
		if timestamp, err := time.Parse(time.RFC3339, branch.Target.Date); err == nil {
			branchInfo.UpdatedAt = timestamp
		}

		branchInfos = append(branchInfos, branchInfo)
	}

	// Get repository languages (if available)
	languageLines := make(map[string]int)
	// Note: Bitbucket's API doesn't provide language statistics like GitHub/GitLab
	// We would need to analyze the file tree to determine languages

	// Parse timestamps
	createdAt, _ := time.Parse(time.RFC3339, repository.CreatedOn)
	updatedAt, _ := time.Parse(time.RFC3339, repository.UpdatedOn)

	// Create repository info
	repoInfo := &model.RepositoryInfo{
		ID:            repository.UUID,
		Name:          repository.Name,
		FullName:      repository.FullName,
		Description:   repository.Description,
		URL:           repository.Links.HTML.Href,
		DefaultBranch: repository.MainBranch.Name,
		Size:          int64(repository.Size / 1024), // Convert bytes to KB
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Branches:      branchInfos,
		Languages:     languageLines,
	}

	return repoInfo, nil
}

// GetRepositorySnapshot retrieves all files in the repository at a specific commit
func (p *Provider) GetRepositorySnapshot(ctx context.Context, projectID, commitSHA string) (*model.RepositorySnapshot, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, erro.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Get commit details for timestamp
	commitURL := fmt.Sprintf("repositories/%s/%s/commit/%s", workspace, repoSlug, commitSHA)
	var commit bitbucketCommit
	_, err := p.client.Get(ctx, commitURL, &commit)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get commit details")
	}

	// Use a recursive approach to get all files
	files, err := p.getRepositoryFilesRecursive(ctx, workspace, repoSlug, commitSHA, "", "/")
	if err != nil {
		return nil, erro.Wrap(err, "failed to get repository files")
	}

	// Calculate total size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}

	// Parse timestamp
	timestamp, _ := time.Parse(time.RFC3339, commit.Date)

	snapshot := &model.RepositorySnapshot{
		CommitSHA: commitSHA,
		Timestamp: timestamp,
		Files:     files,
		TotalSize: totalSize,
	}

	return snapshot, nil
}

// getRepositoryFilesRecursive recursively fetches all files in the repository
func (p *Provider) getRepositoryFilesRecursive(ctx context.Context, workspace, repoSlug, commitSHA, currentPath, parentPath string) ([]*model.RepositoryFile, error) {
	var files []*model.RepositoryFile

	// Build API URL for directory listing
	apiURL := fmt.Sprintf("repositories/%s/%s/src/%s%s", workspace, repoSlug, commitSHA, currentPath)

	var response struct {
		Values []bitbucketFileNode `json:"values"`
	}

	_, err := p.client.Get(ctx, apiURL, &response)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get directory listing")
	}

	for _, node := range response.Values {
		fullPath := currentPath
		if fullPath != "/" {
			fullPath = strings.TrimSuffix(fullPath, "/") + "/" + node.Path
		} else {
			fullPath = "/" + node.Path
		}
		fullPath = strings.TrimPrefix(fullPath, "/")

		if node.Type == "commit_file" {
			// This is a file, get its content
			fileURL := fmt.Sprintf("repositories/%s/%s/src/%s/%s", workspace, repoSlug, commitSHA, fullPath)
			resp, err := p.client.Get(ctx, fileURL)
			if err != nil {
				p.logger.Warn("failed to get file content", "path", fullPath, "error", err)
				continue
			}

			content := string(resp.Body())
			isBinary := p.isBinaryContent(content)

			file := &model.RepositoryFile{
				Path:        fullPath,
				Content:     content,
				Size:        int64(node.Size),
				Mode:        "100644", // Default file mode for Bitbucket
				IsBinary:    isBinary,
				ContentType: p.getContentType(fullPath),
			}

			files = append(files, file)

		} else if node.Type == "commit_directory" {
			// This is a directory, recurse into it
			subFiles, err := p.getRepositoryFilesRecursive(ctx, workspace, repoSlug, commitSHA, fullPath+"/", fullPath)
			if err != nil {
				p.logger.Warn("failed to get subdirectory files", "path", fullPath, "error", err)
				continue
			}
			files = append(files, subFiles...)
		}
	}

	return files, nil
}

// isBinaryContent heuristic to determine if content is binary
func (p *Provider) isBinaryContent(content string) bool {
	// Simple heuristic: if content contains null bytes, it's likely binary
	return strings.Contains(content, "\x00")
}

// getContentType determines content type based on file extension
func (p *Provider) getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".go":
		return "text/x-go"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".py":
		return "text/x-python"
	case ".java":
		return "text/x-java"
	case ".c", ".cpp", ".cc":
		return "text/x-c"
	case ".h", ".hpp":
		return "text/x-c-header"
	case ".css":
		return "text/css"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "text/xml"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".md":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".sh":
		return "text/x-shellscript"
	case ".sql":
		return "text/x-sql"
	default:
		return "text/plain"
	}
}
