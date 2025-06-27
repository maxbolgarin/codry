package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"golang.org/x/oauth2"
)

// Provider implements the CodeProvider interface for GitHub
type Provider struct {
	client *github.Client
	config models.ProviderConfig
	logger logze.Logger
}

// NewProvider creates a new GitHub provider
func NewProvider(config models.ProviderConfig, logger logze.Logger) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("GitHub token is required")
	}

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	// Create GitHub client
	client := github.NewClient(tc)

	// Set base URL if provided (for GitHub Enterprise)
	if config.BaseURL != "" && config.BaseURL != "https://github.com" {
		var err error
		client, err = github.NewEnterpriseClient(config.BaseURL, config.BaseURL, tc)
		if err != nil {
			return nil, errm.Wrap(err, "failed to create GitHub Enterprise client")
		}
	}

	return &Provider{
		client: client,
		config: config,
		logger: logger,
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
func (p *Provider) ParseWebhookEvent(payload []byte) (*models.WebhookEvent, error) {
	var githubPayload struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			ID     int    `json:"id"`
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			State  string `json:"state"`
			Head   struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
			HTMLURL string `json:"html_url"`
			User    struct {
				ID    int    `json:"id"`
				Login string `json:"login"`
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"user"`
			RequestedReviewers []struct {
				ID    int    `json:"id"`
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requested_reviewers"`
		} `json:"pull_request"`
		Repository struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repository"`
		Sender struct {
			ID    int    `json:"id"`
			Login string `json:"login"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"sender"`
	}

	if err := json.Unmarshal(payload, &githubPayload); err != nil {
		return nil, errm.Wrap(err, "failed to parse GitHub webhook payload")
	}

	// Convert reviewers
	var reviewers []*models.User
	for _, reviewer := range githubPayload.PullRequest.RequestedReviewers {
		reviewers = append(reviewers, &models.User{
			ID:       strconv.Itoa(reviewer.ID),
			Username: reviewer.Login,
			Name:     reviewer.Name,
		})
	}

	event := &models.WebhookEvent{
		Type:      "pull_request",
		Action:    githubPayload.Action,
		ProjectID: githubPayload.Repository.FullName, // GitHub uses "owner/repo" format
		User: &models.User{
			ID:       strconv.Itoa(githubPayload.Sender.ID),
			Username: githubPayload.Sender.Login,
			Name:     githubPayload.Sender.Name,
			Email:    githubPayload.Sender.Email,
		},
		MergeRequest: &models.MergeRequest{
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
			Author: &models.User{
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
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*models.MergeRequest, error) {
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
	var reviewers []*models.User
	if pr.RequestedReviewers != nil {
		for _, reviewer := range pr.RequestedReviewers {
			reviewers = append(reviewers, &models.User{
				ID:       strconv.FormatInt(*reviewer.ID, 10),
				Username: *reviewer.Login,
				Name:     reviewer.GetName(),
			})
		}
	}

	return &models.MergeRequest{
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
		Author: &models.User{
			ID:       strconv.FormatInt(*pr.User.ID, 10),
			Username: *pr.User.Login,
			Name:     pr.User.GetName(),
		},
		Reviewers: reviewers,
	}, nil
}

// GetMergeRequestDiffs retrieves the file diffs for a pull request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*models.FileDiff, error) {
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
	var fileDiffs []*models.FileDiff
	for _, file := range allFiles {
		fileDiff := &models.FileDiff{
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
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *models.ReviewComment) error {
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
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*models.ReviewComment, error) {
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
	var reviewComments []*models.ReviewComment
	for _, comment := range allComments {
		reviewComment := &models.ReviewComment{
			ID:   strconv.FormatInt(*comment.ID, 10),
			Body: *comment.Body,
			Author: &models.User{
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
func (p *Provider) GetCurrentUser(ctx context.Context) (*models.User, error) {
	user, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user")
	}

	return &models.User{
		ID:       strconv.FormatInt(*user.ID, 10),
		Username: *user.Login,
		Name:     user.GetName(),
		Email:    user.GetEmail(),
	}, nil
}
