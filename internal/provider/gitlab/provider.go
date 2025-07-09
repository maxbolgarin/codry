package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
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

// New creates a new GitLab provider
func New(config model.ProviderConfig) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("GitLab token is required")
	}
	logger := logze.With("provider", "gitlab", "component", "provider")

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

	// Convert reviewers from webhook payload
	var reviewers []model.User
	for _, reviewer := range gitlabPayload.Reviewers {
		reviewers = append(reviewers, model.User{
			ID:       strconv.Itoa(reviewer.ID),
			Username: reviewer.Username,
			Name:     reviewer.Name,
		})
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
			Reviewers:    reviewers,
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

	// Check if this is a line-specific comment
	if comment.Type == model.CommentTypeInline && comment.FilePath != "" && comment.Line > 0 {
		// Create a positioned discussion for line-specific comments
		baseSHA := "base"
		startSHA := "start"
		headSHA := "head"
		positionType := "text"
		newPath := comment.FilePath
		newLine := comment.Line

		positionOpts := &gitlab.PositionOptions{
			BaseSHA:      &baseSHA,
			StartSHA:     &startSHA,
			HeadSHA:      &headSHA,
			PositionType: &positionType,
			NewPath:      &newPath,
			NewLine:      &newLine,
		}

		// Handle range comments if this is a review comment
		if (comment.Type == model.CommentTypeReview || comment.Type == model.CommentTypeInline) && p.isRangeComment(comment.Body) {
			startLine, endLine := p.extractLineRange(comment.Body)
			if startLine > 0 && endLine > startLine {
				// GitLab doesn't have native range comments, but we can use the start line
				// and include range information in the comment body
				positionOpts.NewLine = &startLine
			}
		}

		discussionOpts := &gitlab.CreateMergeRequestDiscussionOptions{
			Body:     &comment.Body,
			Position: positionOpts,
		}

		discussion, _, err := p.client.Discussions.CreateMergeRequestDiscussion(projectIDInt, mrIID, discussionOpts)
		if err != nil {
			return errm.Wrap(err, "failed to create merge request discussion")
		}

		comment.ID = discussion.ID
		return nil
	}

	// Create regular discussion for general comments
	return p.createRegularComment(projectIDInt, mrIID, comment)
}

// createRegularComment creates a regular (non-positioned) discussion
func (p *Provider) createRegularComment(projectID int, mrIID int, comment *model.Comment) error {
	discussionOpts := &gitlab.CreateMergeRequestDiscussionOptions{
		Body: &comment.Body,
	}

	discussion, _, err := p.client.Discussions.CreateMergeRequestDiscussion(projectID, mrIID, discussionOpts)
	if err != nil {
		return errm.Wrap(err, "failed to create merge request discussion")
	}

	comment.ID = discussion.ID
	return nil
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
		State:        []string{"opened"}, // GitLab uses "opened" instead of "open"
		Limit:        100,                // Reasonable default
	}

	return p.ListMergeRequests(ctx, projectID, filter)
}

// IsMergeRequestEvent determines if a webhook event is a merge request event that should be processed
func (p *Provider) IsMergeRequestEvent(event *model.CodeEvent) bool {
	// Only process merge request events
	if event.Type != "merge_request" {
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
		return false
	}

	// Don't process events from the bot itself to avoid loops
	if event.User.Username == p.config.BotUsername {
		return false
	}

	// For close/merge actions, we might want to do cleanup but not review
	if event.Action == "close" || event.Action == "merge" {
		return false
	}

	// CRITICAL: Only process MRs where the bot is in the reviewers list
	// This implements requirement #1: analyze all webhooks but only review MRs where bot is reviewer
	if !p.isBotInReviewersList(event.MergeRequest) {
		p.logger.Debug("bot not in reviewers list, skipping review",
			"mr_iid", event.MergeRequest.IID,
			"bot_username", p.config.BotUsername,
			"reviewers_count", len(event.MergeRequest.Reviewers))
		return false
	}

	p.logger.Debug("merge request event should be processed",
		"action", event.Action,
		"mr_iid", event.MergeRequest.IID,
		"bot_in_reviewers", true)
	return true
}

// isBotInReviewersList checks if the configured bot username is in the MR reviewers list
func (p *Provider) isBotInReviewersList(mr *model.MergeRequest) bool {
	if mr == nil || p.config.BotUsername == "" {
		return false
	}

	for _, reviewer := range mr.Reviewers {
		if reviewer.Username == p.config.BotUsername {
			return true
		}
	}

	return false
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

// GetFileContent retrieves the content of a file at a specific commit/SHA
func (p *Provider) GetFileContent(ctx context.Context, projectID, filePath, commitSHA string) (string, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return "", errm.Wrap(err, "invalid project ID")
	}

	// Get file content at specific commit
	fileOpts := &gitlab.GetFileOptions{
		Ref: &commitSHA,
	}

	file, resp, err := p.client.RepositoryFiles.GetFile(projectIDInt, filePath, fileOpts)
	if err != nil {
		return "", errm.Wrap(err, "failed to get file content from GitLab")
	}

	if resp.StatusCode != http.StatusOK {
		return "", errm.New("GitLab API returned status %d", resp.StatusCode)
	}

	if file == nil {
		return "", errm.New("file content is nil")
	}

	return file.Content, nil
}

// GetComments retrieves all comments for a merge request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	// Get all discussions for the merge request
	discussions, _, err := p.client.Discussions.ListMergeRequestDiscussions(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get discussions from GitLab")
	}

	var allComments []*model.Comment

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
				CreatedAt: lang.Deref(note.CreatedAt),
				UpdatedAt: lang.Deref(note.UpdatedAt),
			}

			// Determine comment type based on position
			if note.Position != nil && note.Position.NewPath != "" {
				comment.Type = model.CommentTypeInline
				comment.FilePath = note.Position.NewPath
				if note.Position.NewLine != 0 {
					comment.Line = note.Position.NewLine
				}
			} else {
				comment.Type = model.CommentTypeGeneral
			}

			allComments = append(allComments, comment)
		}
	}

	return allComments, nil
}

// GetMergeRequestCommits retrieves all commits for a merge request
func (p *Provider) GetMergeRequestCommits(ctx context.Context, projectID string, mrIID int) ([]*model.Commit, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	// Get all commits for the merge request
	commits, _, err := p.client.MergeRequests.GetMergeRequestCommits(projectIDInt, mrIID, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get merge request commits from GitLab")
	}

	// Convert to our model
	var modelCommits []*model.Commit
	for _, commit := range commits {
		modelCommit := p.convertGitLabCommit(commit)
		modelCommits = append(modelCommits, modelCommit)
	}

	return modelCommits, nil
}

// GetCommitDetails retrieves detailed information about a specific commit
func (p *Provider) GetCommitDetails(ctx context.Context, projectID, commitSHA string) (*model.Commit, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	// Get commit details
	commit, _, err := p.client.Commits.GetCommit(projectIDInt, commitSHA, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get commit details from GitLab")
	}

	return p.convertGitLabCommit(commit), nil
}

// GetCommitDiffs retrieves file diffs for a specific commit
func (p *Provider) GetCommitDiffs(ctx context.Context, projectID, commitSHA string) ([]*model.FileDiff, error) {
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, errm.Wrap(err, "invalid project ID")
	}

	// Get commit diff
	diffs, _, err := p.client.Commits.GetCommitDiff(projectIDInt, commitSHA, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get commit diff from GitLab")
	}

	// Convert to our model
	var fileDiffs []*model.FileDiff
	for _, diff := range diffs {
		fileDiff := &model.FileDiff{
			OldPath:   diff.OldPath,
			NewPath:   diff.NewPath,
			Diff:      diff.Diff,
			IsNew:     diff.NewFile,
			IsDeleted: diff.DeletedFile,
			IsRenamed: diff.RenamedFile,
			IsBinary:  diff.Diff == "" && !diff.DeletedFile && !diff.NewFile,
		}
		fileDiffs = append(fileDiffs, fileDiff)
	}

	return fileDiffs, nil
}

// convertGitLabCommit converts a GitLab commit to our model
func (p *Provider) convertGitLabCommit(commit *gitlab.Commit) *model.Commit {
	modelCommit := &model.Commit{
		SHA:       commit.ID,
		URL:       commit.WebURL,
		Timestamp: lang.Deref(commit.CommittedDate),
	}

	// Parse commit message into subject and body
	if commit.Message != "" {
		p.parseCommitMessage(commit.Message, modelCommit)
	}

	// Set author information
	modelCommit.Author = model.User{
		Name:  commit.AuthorName,
		Email: commit.AuthorEmail,
	}

	// Set committer information
	modelCommit.Committer = model.User{
		Name:  commit.CommitterName,
		Email: commit.CommitterEmail,
	}

	// Get commit statistics
	if commit.Stats != nil {
		modelCommit.Stats = model.CommitStats{
			Additions:  commit.Stats.Additions,
			Deletions:  commit.Stats.Deletions,
			TotalFiles: 0, // GitLab doesn't provide total files in stats, would need to fetch diffs separately
		}
	}

	return modelCommit
}

// parseCommitMessage parses a commit message into subject and body (shared with GitHub)
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
	projectIDInt, err := strconv.Atoi(projectID)
	if err != nil {
		return errm.Wrap(err, "invalid project ID")
	}

	// Get all discussions to find the one containing this comment
	discussions, _, err := p.client.Discussions.ListMergeRequestDiscussions(projectIDInt, mrIID, nil)
	if err != nil {
		return errm.Wrap(err, "failed to get discussions from GitLab")
	}

	var discussionID string
	var noteID int
	found := false

	// Find the discussion and note ID for the comment
	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			if strconv.Itoa(note.ID) == commentID {
				discussionID = discussion.ID
				noteID = note.ID
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return errm.New("comment not found")
	}

	// Update the note
	updateOpts := &gitlab.UpdateMergeRequestDiscussionNoteOptions{
		Body: &newBody,
	}

	_, _, err = p.client.Discussions.UpdateMergeRequestDiscussionNote(projectIDInt, mrIID, discussionID, noteID, updateOpts)
	if err != nil {
		return errm.Wrap(err, "failed to update comment")
	}

	return nil
}
