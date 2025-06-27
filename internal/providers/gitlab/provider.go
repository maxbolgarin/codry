package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Provider implements the CodeProvider interface for GitLab
type Provider struct {
	client *gitlab.Client
	config models.ProviderConfig
	logger logze.Logger
}

// NewProvider creates a new GitLab provider
func NewProvider(config models.ProviderConfig, logger logze.Logger) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("GitLab token is required")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}

	client, err := gitlab.NewClient(config.Token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, errm.Wrap(err, "failed to create GitLab client")
	}

	return &Provider{
		client: client,
		config: config,
		logger: logger,
	}, nil
}

// ValidateWebhook validates the webhook signature
func (p *Provider) ValidateWebhook(payload []byte, signature string) error {
	if p.config.WebhookSecret == "" {
		return nil // No secret configured, skip validation
	}

	if signature != p.config.WebhookSecret {
		return errm.New("invalid webhook signature")
	}

	return nil
}

// ParseWebhookEvent parses a GitLab webhook event
func (p *Provider) ParseWebhookEvent(payload []byte) (*models.WebhookEvent, error) {
	var gitlabPayload struct {
		ObjectKind string `json:"object_kind"`
		EventType  string `json:"event_type"`
		User       struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"user"`
		Project struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
		ObjectAttributes struct {
			IID          int    `json:"iid"`
			Action       string `json:"action"`
			State        string `json:"state"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			URL          string `json:"url"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			AuthorID     int    `json:"author_id"`
			LastCommit   struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"object_attributes"`
	}

	if err := json.Unmarshal(payload, &gitlabPayload); err != nil {
		return nil, errm.Wrap(err, "failed to parse GitLab webhook payload")
	}

	event := &models.WebhookEvent{
		Type:      gitlabPayload.ObjectKind,
		Action:    gitlabPayload.ObjectAttributes.Action,
		ProjectID: strconv.Itoa(gitlabPayload.Project.ID),
		User: &models.User{
			ID:       strconv.Itoa(gitlabPayload.User.ID),
			Username: gitlabPayload.User.Username,
			Name:     gitlabPayload.User.Name,
			Email:    gitlabPayload.User.Email,
		},
		MergeRequest: &models.MergeRequest{
			ID:           strconv.Itoa(gitlabPayload.ObjectAttributes.IID),
			IID:          gitlabPayload.ObjectAttributes.IID,
			Title:        gitlabPayload.ObjectAttributes.Title,
			Description:  gitlabPayload.ObjectAttributes.Description,
			SourceBranch: gitlabPayload.ObjectAttributes.SourceBranch,
			TargetBranch: gitlabPayload.ObjectAttributes.TargetBranch,
			URL:          gitlabPayload.ObjectAttributes.URL,
			State:        gitlabPayload.ObjectAttributes.State,
			SHA:          gitlabPayload.ObjectAttributes.LastCommit.ID,
			AuthorID:     strconv.Itoa(gitlabPayload.ObjectAttributes.AuthorID),
		},
	}

	return event, nil
}

// GetMergeRequest retrieves detailed information about a merge request
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*models.MergeRequest, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	mr, resp, err := p.client.MergeRequests.GetMergeRequest(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get merge request from GitLab")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errm.New(fmt.Sprintf("GitLab API returned status %d", resp.StatusCode))
	}

	// Convert reviewers
	var reviewers []*models.User
	for _, reviewer := range mr.Reviewers {
		reviewers = append(reviewers, &models.User{
			ID:       strconv.Itoa(reviewer.ID),
			Username: reviewer.Username,
			Name:     reviewer.Name,
		})
	}

	return &models.MergeRequest{
		ID:           strconv.Itoa(mr.ID),
		IID:          mr.IID,
		Title:        mr.Title,
		Description:  mr.Description,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		URL:          mr.WebURL,
		State:        mr.State,
		SHA:          mr.SHA,
		AuthorID:     strconv.Itoa(mr.Author.ID),
		Author: &models.User{
			ID:       strconv.Itoa(mr.Author.ID),
			Username: mr.Author.Username,
			Name:     mr.Author.Name,
		},
		Reviewers: reviewers,
	}, nil
}

// GetMergeRequestDiffs retrieves the file diffs for a merge request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*models.FileDiff, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	var allDiffs []*gitlab.MergeRequestDiff
	page := 1

	// Fetch all pages of diffs
	for {
		opts := &gitlab.ListMergeRequestDiffsOptions{
			ListOptions: gitlab.ListOptions{
				Page: page,
			},
		}

		diffs, resp, err := p.client.MergeRequests.ListMergeRequestDiffs(projectIDInt, mrIID, opts)
		if err != nil {
			return nil, errm.Wrap(err, "failed to list merge request diffs")
		}

		if resp.StatusCode != http.StatusOK {
			return nil, errm.New(fmt.Sprintf("GitLab API returned status %d", resp.StatusCode))
		}

		allDiffs = append(allDiffs, diffs...)

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	// Convert to our models
	var fileDiffs []*models.FileDiff
	for _, diff := range allDiffs {
		fileDiff := &models.FileDiff{
			OldPath:   diff.OldPath,
			NewPath:   diff.NewPath,
			Diff:      diff.Diff,
			IsNew:     diff.NewFile,
			IsDeleted: diff.DeletedFile,
			IsRenamed: diff.RenamedFile,
			IsBinary:  diff.Diff == "" && !diff.DeletedFile && !diff.NewFile, // Heuristic for binary files
		}
		fileDiffs = append(fileDiffs, fileDiff)
	}

	return fileDiffs, nil
}

// UpdateMergeRequestDescription updates the description of a merge request
func (p *Provider) UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return errm.Wrap(err, "invalid project ID")
	}

	updateOpts := &gitlab.UpdateMergeRequestOptions{
		Description: &description,
	}

	_, _, err = p.client.MergeRequests.UpdateMergeRequest(projectIDInt, mrIID, updateOpts)
	if err != nil {
		return errm.Wrap(err, "failed to update merge request description")
	}

	return nil
}

// CreateComment creates a discussion/comment on a merge request
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *models.ReviewComment) error {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return errm.Wrap(err, "invalid project ID")
	}

	discussionOpts := &gitlab.CreateMergeRequestDiscussionOptions{
		Body: &comment.Body,
	}

	discussion, _, err := p.client.Discussions.CreateMergeRequestDiscussion(projectIDInt, mrIID, discussionOpts)
	if err != nil {
		return errm.Wrap(err, "failed to create merge request discussion")
	}

	// Update comment with the created ID
	comment.ID = discussion.ID

	return nil
}

// GetComments retrieves all comments/discussions for a merge request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*models.ReviewComment, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	discussions, _, err := p.client.Discussions.ListMergeRequestDiscussions(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list merge request discussions")
	}

	var comments []*models.ReviewComment
	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			comment := &models.ReviewComment{
				ID:   strconv.Itoa(note.ID),
				Body: note.Body,
				Author: &models.User{
					ID:       strconv.Itoa(note.Author.ID),
					Username: note.Author.Username,
					Name:     note.Author.Name,
				},
			}
			comments = append(comments, comment)
		}
	}

	return comments, nil
}

// GetCurrentUser retrieves information about the current authenticated user
func (p *Provider) GetCurrentUser(ctx context.Context) (*models.User, error) {
	user, _, err := p.client.Users.CurrentUser()
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user")
	}

	return &models.User{
		ID:       strconv.Itoa(user.ID),
		Username: user.Username,
		Name:     user.Name,
		Email:    user.Email,
	}, nil
}
