package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"golang.org/x/oauth2"
)

var _ interfaces.CodeProvider = (*Provider)(nil)

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
	var reviewers []model.User
	for _, reviewer := range githubPayload.PullRequest.RequestedReviewers {
		reviewers = append(reviewers, model.User{
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
			Author: model.User{
				ID:       strconv.Itoa(githubPayload.PullRequest.User.ID),
				Username: githubPayload.PullRequest.User.Login,
				Name:     githubPayload.PullRequest.User.Name,
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
	var reviewers []model.User
	if pr.RequestedReviewers != nil {
		for _, reviewer := range pr.RequestedReviewers {
			reviewers = append(reviewers, model.User{
				ID:       strconv.FormatInt(reviewer.GetID(), 10),
				Username: reviewer.GetLogin(),
				Name:     reviewer.GetName(),
			})
		}
	}

	return &model.MergeRequest{
		ID:           strconv.FormatInt(pr.GetID(), 10),
		IID:          pr.GetNumber(),
		Title:        pr.GetTitle(),
		Description:  pr.GetBody(),
		SourceBranch: pr.GetHead().GetRef(),
		TargetBranch: pr.GetBase().GetRef(),
		URL:          pr.GetHTMLURL(),
		State:        pr.GetState(),
		SHA:          pr.GetHead().GetSHA(),
		Author: model.User{
			ID:       strconv.FormatInt(pr.User.GetID(), 10),
			Username: pr.User.GetLogin(),
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
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Check if this is a line-specific comment
	if comment.Type == model.CommentTypeInline && comment.FilePath != "" && comment.Line > 0 {
		p.logger.Debug("creating positioned comment",
			"file", comment.FilePath,
			"line", comment.Line,
			"type", comment.Type)
		return p.createPositionedComment(ctx, owner, repo, mrIID, comment)
	}

	// Create regular issue comment for general comments
	p.logger.Debug("creating regular comment", "type", comment.Type, "has_file", comment.FilePath != "", "has_line", comment.Line > 0)
	return p.createRegularComment(ctx, owner, repo, mrIID, comment)
}

func (p *Provider) createPositionedComment(ctx context.Context, owner, repo string, mrIID int, comment *model.Comment) error {
	// Get the pull request to obtain the commit SHA
	pr, _, err := p.client.PullRequests.Get(ctx, owner, repo, mrIID)
	if err != nil {
		return errm.Wrap(err, "failed to get pull request for commit SHA")
	}

	head := pr.GetHead()
	if head == nil {
		return errm.New("head is nil")
	}

	commitID := head.GetSHA()
	if commitID == "" {
		return errm.New("commit SHA is empty")
	}

	// Create pull request review comment with proper GitHub API format
	reviewComment := &github.PullRequestComment{
		Body:     &comment.Body,
		Path:     &comment.FilePath,
		CommitID: &commitID,
	}

	// Handle range comments vs single line comments
	if comment.Type == model.CommentTypeReview || comment.Type == model.CommentTypeInline {
		// Check if this is a range comment by parsing the comment body
		if p.isRangeComment(comment.Body) {
			startLine, endLine := p.extractLineRange(comment.Body)
			if startLine > 0 && endLine > startLine {
				// GitHub range comment format
				side := "RIGHT" // Comments on new lines are on the RIGHT side
				reviewComment.StartLine = &startLine
				reviewComment.Line = &endLine
				reviewComment.Side = &side

				p.logger.Debug("creating GitHub range comment",
					"file", comment.FilePath,
					"start_line", startLine,
					"end_line", endLine,
					"commit_id", commitID[:8])
			} else {
				// Fall back to single line
				p.setSingleLineComment(reviewComment, comment)
			}
		} else {
			// Single line comment
			p.setSingleLineComment(reviewComment, comment)
		}
	} else {
		// Regular single line comment
		p.setSingleLineComment(reviewComment, comment)
	}

	_, _, err = p.client.PullRequests.CreateComment(ctx, owner, repo, mrIID, reviewComment)
	if err != nil {
		return errm.Wrap(err, "failed to create positioned comment")
	}

	return nil
}

// setSingleLineComment sets up a single line comment
func (p *Provider) setSingleLineComment(reviewComment *github.PullRequestComment, comment *model.Comment) {
	if comment.Line > 0 {
		line := comment.Line
		side := "RIGHT" // Comments on new lines are on the RIGHT side
		reviewComment.Line = &line
		reviewComment.Side = &side
	}

	// Use position as fallback
	if comment.Position > 0 {
		reviewComment.Position = &comment.Position
	}
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
		p.logger.Debug("extracted line range from comment",
			"start_line", startLine,
			"end_line", endLine,
			"pattern_found", true)
		return startLine, endLine
	}

	p.logger.Debug("no line range found in comment body", "pattern_found", false)
	return 0, 0
}

// createRegularComment creates a regular (non-positioned) issue comment
func (p *Provider) createRegularComment(ctx context.Context, owner, repo string, mrIID int, comment *model.Comment) error {
	// Create issue comment (GitHub treats PR comments as issue comments)
	githubComment := &github.IssueComment{
		Body: &comment.Body,
	}

	_, _, err := p.client.Issues.CreateComment(ctx, owner, repo, mrIID, githubComment)
	if err != nil {
		return errm.Wrap(err, "failed to create pull request comment")
	}

	return nil
}

// GetComments retrieves all comments for a pull request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error) {
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
	var reviewComments []*model.Comment
	for _, comment := range allComments {
		reviewComment := &model.Comment{
			ID:   strconv.FormatInt(comment.GetID(), 10),
			Body: comment.GetBody(),
			Author: model.User{
				ID:       strconv.FormatInt(comment.User.GetID(), 10),
				Username: comment.User.GetLogin(),
				Name:     comment.User.GetName(),
			},
			CreatedAt: comment.GetCreatedAt().Time,
			UpdatedAt: comment.GetUpdatedAt().Time,
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
		ID:       strconv.FormatInt(user.GetID(), 10),
		Username: user.GetLogin(),
		Name:     user.GetName(),
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
	comment := &model.Comment{
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
		ID:   strconv.FormatInt(comment.GetID(), 10),
		Body: comment.GetBody(),
		Author: model.User{
			ID:       strconv.FormatInt(comment.User.GetID(), 10),
			Username: comment.User.GetLogin(),
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
			authorID := strconv.FormatInt(pr.User.GetID(), 10)
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
		var reviewers []model.User
		if pr.RequestedReviewers != nil {
			for _, reviewer := range pr.RequestedReviewers {
				reviewers = append(reviewers, model.User{
					ID:       strconv.FormatInt(reviewer.GetID(), 10),
					Username: reviewer.GetLogin(),
					Name:     reviewer.GetName(),
				})
			}
		}

		modelMR := &model.MergeRequest{
			ID:           strconv.FormatInt(pr.GetID(), 10),
			IID:          pr.GetNumber(),
			Title:        pr.GetTitle(),
			Description:  pr.GetBody(),
			SourceBranch: pr.GetHead().GetRef(),
			TargetBranch: pr.GetBase().GetRef(),
			URL:          pr.GetHTMLURL(),
			State:        pr.GetState(),
			SHA:          pr.GetHead().GetSHA(),
			Author: model.User{
				ID:       strconv.FormatInt(pr.User.GetID(), 10),
				Username: pr.User.GetLogin(),
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

// GetFileContent retrieves the content of a file at a specific commit/SHA
func (p *Provider) GetFileContent(ctx context.Context, projectID, filePath, commitSHA string) (string, error) {
	// Parse owner/repo from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return "", errm.New("invalid GitHub project ID format, expected 'owner/repo'")
	}
	owner, repo := parts[0], parts[1]

	// Get file content at specific commit
	fileContent, _, resp, err := p.client.Repositories.GetContents(ctx, owner, repo, filePath, &github.RepositoryContentGetOptions{
		Ref: commitSHA,
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to get file content from GitHub")
	}

	if resp.StatusCode != 200 {
		return "", errm.New("GitHub API returned status %d", resp.StatusCode)
	}

	if fileContent == nil {
		return "", errm.New("file content is nil")
	}

	// Decode content (GitHub returns base64 encoded content)
	content, err := fileContent.GetContent()
	if err != nil {
		return "", errm.Wrap(err, "failed to decode file content")
	}

	return content, nil
}
