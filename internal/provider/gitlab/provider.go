package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	defaultBaseURL = "https://gitlab.com"
)

var _ interfaces.CodeProvider = (*Provider)(nil)

// Provider implements the CodeProvider interface for GitLab
type Provider struct {
	client *gitlab.Client
	config model.ProviderConfig
	logger logze.Logger
}

// NewProvider creates a new GitLab provider
func NewProvider(config model.ProviderConfig) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("GitLab token is required")
	}
	logger := logze.With("provider", "gitlab")

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
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
func (p *Provider) ParseWebhookEvent(payload []byte) (*model.CodeEvent, error) {
	var gitlabPayload gitlabPayload
	if err := json.Unmarshal(payload, &gitlabPayload); err != nil {
		return nil, errm.Wrap(err, "failed to parse GitLab webhook payload")
	}

	event := &model.CodeEvent{
		Type:      gitlabPayload.ObjectKind,
		Action:    gitlabPayload.ObjectAttributes.Action,
		ProjectID: strconv.Itoa(gitlabPayload.Project.ID),
		User: &model.User{
			ID:       strconv.Itoa(gitlabPayload.User.ID),
			Username: gitlabPayload.User.Username,
			Name:     gitlabPayload.User.Name,
		},
		MergeRequest: &model.MergeRequest{
			ID:           strconv.Itoa(gitlabPayload.ObjectAttributes.IID),
			IID:          gitlabPayload.ObjectAttributes.IID,
			Title:        gitlabPayload.ObjectAttributes.Title,
			Description:  gitlabPayload.ObjectAttributes.Description,
			SourceBranch: gitlabPayload.ObjectAttributes.SourceBranch,
			TargetBranch: gitlabPayload.ObjectAttributes.TargetBranch,
			URL:          gitlabPayload.ObjectAttributes.URL,
			State:        gitlabPayload.ObjectAttributes.State,
			SHA:          gitlabPayload.ObjectAttributes.LastCommit.ID,
		},
	}

	return event, nil
}

// GetMergeRequest retrieves detailed information about a merge request
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*model.MergeRequest, error) {
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
	var reviewers []model.User
	for _, reviewer := range mr.Reviewers {
		reviewers = append(reviewers, model.User{
			ID:       strconv.Itoa(reviewer.ID),
			Username: reviewer.Username,
			Name:     reviewer.Name,
		})
	}

	return &model.MergeRequest{
		ID:           strconv.Itoa(mr.ID),
		IID:          mr.IID,
		Title:        mr.Title,
		Description:  mr.Description,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		URL:          mr.WebURL,
		State:        mr.State,
		SHA:          mr.SHA,
		Author: model.User{
			ID:       strconv.Itoa(mr.Author.ID),
			Username: mr.Author.Username,
			Name:     mr.Author.Name,
		},
		Reviewers: reviewers,
		CreatedAt: lang.Deref(mr.CreatedAt),
		UpdatedAt: lang.Deref(mr.UpdatedAt),
	}, nil
}

// GetMergeRequestDiffs retrieves the file diffs for a merge request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*model.FileDiff, error) {
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
	var fileDiffs []*model.FileDiff
	for _, diff := range allDiffs {
		fileDiff := &model.FileDiff{
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
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error {
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
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	discussions, _, err := p.client.Discussions.ListMergeRequestDiscussions(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list merge request discussions")
	}

	var comments []*model.Comment
	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			comment := &model.Comment{
				ID:   strconv.Itoa(note.ID),
				Body: note.Body,
				Author: model.User{
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
func (p *Provider) GetCurrentUser(ctx context.Context) (*model.User, error) {
	user, _, err := p.client.Users.CurrentUser()
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user")
	}

	return &model.User{
		ID:       strconv.Itoa(user.ID),
		Username: user.Username,
		Name:     user.Name,
	}, nil
}

// IsCommentEvent determines if a webhook event is a comment event that should be processed
func (p *Provider) IsCommentEvent(event *model.CodeEvent) bool {
	return event.Type == "note" || event.Type == "comment"
}

// ReplyToComment replies to an existing comment
func (p *Provider) ReplyToComment(ctx context.Context, projectID string, mrIID int, commentID string, reply string) error {
	// For now, create a new discussion as a reply
	// TODO: Implement proper thread replies when GitLab client supports it
	comment := &model.Comment{
		Body: fmt.Sprintf("Reply to comment %s: %s", commentID, reply),
	}
	return p.CreateComment(ctx, projectID, mrIID, comment)
}

// GetComment retrieves a specific comment
func (p *Provider) GetComment(ctx context.Context, projectID string, mrIID int, commentID string) (*model.Comment, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	// Get all discussions and find the specific comment
	discussions, _, err := p.client.Discussions.ListMergeRequestDiscussions(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list discussions")
	}

	for _, discussion := range discussions {
		if discussion.ID == commentID {
			if len(discussion.Notes) > 0 {
				note := discussion.Notes[0]
				return &model.Comment{
					ID:   strconv.Itoa(note.ID),
					Body: note.Body,
					Author: model.User{
						ID:       strconv.Itoa(note.Author.ID),
						Username: note.Author.Username,
						Name:     note.Author.Name,
					},
					CreatedAt: lang.Deref(note.CreatedAt),
					UpdatedAt: lang.Deref(note.UpdatedAt),
				}, nil
			}
		}
		// Also check individual notes within discussions
		for _, note := range discussion.Notes {
			if strconv.Itoa(note.ID) == commentID {
				return &model.Comment{
					ID:   strconv.Itoa(note.ID),
					Body: note.Body,
					Author: model.User{
						ID:       strconv.Itoa(note.Author.ID),
						Username: note.Author.Username,
						Name:     note.Author.Name,
					},
					CreatedAt: lang.Deref(note.CreatedAt),
					UpdatedAt: lang.Deref(note.UpdatedAt),
				}, nil
			}
		}
	}

	return nil, errm.New("comment not found")
}

// ListMergeRequests retrieves multiple merge requests based on filter criteria
func (p *Provider) ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	opts := &gitlab.ListProjectMergeRequestsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    filter.Page + 1, // GitLab uses 1-based pagination
			PerPage: filter.Limit,
		},
	}

	// Apply filters
	if len(filter.State) > 0 {
		// GitLab typically uses "opened", "closed", "merged"
		opts.State = &filter.State[0]
	}

	if filter.TargetBranch != "" {
		opts.TargetBranch = &filter.TargetBranch
	}

	if filter.SourceBranch != "" {
		opts.SourceBranch = &filter.SourceBranch
	}

	if filter.AuthorID != "" {
		authorIDInt, err := strconv.Atoi(filter.AuthorID)
		if err == nil {
			opts.AuthorID = &authorIDInt
		}
	}

	if filter.UpdatedAfter != nil {
		opts.UpdatedAfter = filter.UpdatedAfter
	}

	if filter.CreatedAfter != nil {
		opts.CreatedAfter = filter.CreatedAfter
	}

	mrs, _, err := p.client.MergeRequests.ListProjectMergeRequests(projectIDInt, opts)
	if err != nil {
		return nil, errm.Wrap(err, "failed to list merge requests")
	}

	var result []*model.MergeRequest
	for _, mr := range mrs {
		// Convert reviewers
		var reviewers []model.User
		for _, reviewer := range mr.Reviewers {
			reviewers = append(reviewers, model.User{
				ID:       strconv.Itoa(reviewer.ID),
				Username: reviewer.Username,
				Name:     reviewer.Name,
			})
		}

		modelMR := &model.MergeRequest{
			ID:           strconv.Itoa(mr.ID),
			IID:          mr.IID,
			Title:        mr.Title,
			Description:  mr.Description,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			URL:          mr.WebURL,
			State:        mr.State,
			SHA:          mr.SHA,
			Author: model.User{
				ID:       strconv.Itoa(mr.Author.ID),
				Username: mr.Author.Username,
				Name:     mr.Author.Name,
			},
			Reviewers: reviewers,
			CreatedAt: lang.Deref(mr.CreatedAt),
			UpdatedAt: lang.Deref(mr.UpdatedAt),
		}
		result = append(result, modelMR)
	}

	return result, nil
}

// GetMergeRequestUpdates retrieves merge requests updated since a specific time
func (p *Provider) GetMergeRequestUpdates(ctx context.Context, projectID string, since time.Time) ([]*model.MergeRequest, error) {
	filter := &model.MergeRequestFilter{
		UpdatedAfter: &since,
		State:        []string{"opened"}, // Only get open MRs for updates
		Limit:        100,                // Reasonable default
	}

	return p.ListMergeRequests(ctx, projectID, filter)
}

// IsMergeRequestEvent determines if a webhook event is a merge request event that should be processed
func (p *Provider) IsMergeRequestEvent(event *model.CodeEvent) bool {
	// Only process merge request events
	if event.Type != "merge_request" {
		p.logger.Debug("ignoring non-merge request event", "event_type", event.Type)
		return false
	}

	// Check for relevant actions
	relevantActions := []string{
		"open",   // When MR is opened
		"reopen", // When MR is reopened
		"update", // When MR is updated
		"merge",  // When MR is merged (for cleanup)
		"close",  // When MR is closed (for cleanup)
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

	// For close/merge actions, we might want to do cleanup but not review
	if event.Action == "close" || event.Action == "merge" {
		p.logger.Debug("merge request closed/merged, skipping review", "action", event.Action)
		return false
	}

	p.logger.Debug("merge request event should be processed", "action", event.Action)
	return true
}
