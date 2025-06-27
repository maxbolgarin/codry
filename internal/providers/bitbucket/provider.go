package bitbucket

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Provider implements the CodeProvider interface for Bitbucket
type Provider struct {
	client     *http.Client
	config     models.ProviderConfig
	logger     logze.Logger
	baseURL    string
	apiVersion string
}

// Bitbucket API structures
type BitbucketUser struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	Links       struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		Avatar struct {
			Href string `json:"href"`
		} `json:"avatar"`
	} `json:"links"`
}

type BitbucketPullRequest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	Source      struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"destination"`
	Author    BitbucketUser `json:"author"`
	Reviewers []struct {
		User BitbucketUser `json:"user"`
	} `json:"reviewers"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
		Diff struct {
			Href string `json:"href"`
		} `json:"diff"`
		Comments struct {
			Href string `json:"href"`
		} `json:"comments"`
	} `json:"links"`
}

type BitbucketRepository struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Workspace struct {
		Slug string `json:"slug"`
	} `json:"workspace"`
}

// NewProvider creates a new Bitbucket provider
func NewProvider(config models.ProviderConfig, logger logze.Logger) (*Provider, error) {
	if config.Token == "" {
		return nil, errm.New("Bitbucket token is required")
	}

	// Set base URL
	baseURL := "https://api.bitbucket.org"
	if config.BaseURL != "" {
		baseURL = strings.TrimSuffix(config.BaseURL, "/")
	}

	return &Provider{
		client:     &http.Client{},
		config:     config,
		logger:     logger,
		baseURL:    baseURL,
		apiVersion: "2.0",
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
func (p *Provider) ParseWebhookEvent(payload []byte) (*models.WebhookEvent, error) {
	var bitbucketPayload struct {
		Repository  BitbucketRepository  `json:"repository"`
		PullRequest BitbucketPullRequest `json:"pullrequest"`
		Actor       BitbucketUser        `json:"actor"`
		EventKey    string               `json:"eventKey,omitempty"` // Alternative field name
		Type        string               `json:"type,omitempty"`     // Some events use this
		Changes     json.RawMessage      `json:"changes,omitempty"`  // For update events
		Approval    json.RawMessage      `json:"approval,omitempty"` // For approval events
		Comment     json.RawMessage      `json:"comment,omitempty"`  // For comment events
	}

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
	var reviewers []*models.User
	for _, reviewer := range bitbucketPayload.PullRequest.Reviewers {
		reviewers = append(reviewers, &models.User{
			ID:       reviewer.User.UUID,
			Username: reviewer.User.Username,
			Name:     reviewer.User.DisplayName,
		})
	}

	event := &models.WebhookEvent{
		Type:      eventType,
		Action:    action,
		ProjectID: bitbucketPayload.Repository.FullName, // Format: workspace/repo_slug
		User: &models.User{
			ID:       bitbucketPayload.Actor.UUID,
			Username: bitbucketPayload.Actor.Username,
			Name:     bitbucketPayload.Actor.DisplayName,
		},
		MergeRequest: &models.MergeRequest{
			ID:           strconv.Itoa(bitbucketPayload.PullRequest.ID),
			IID:          bitbucketPayload.PullRequest.ID,
			Title:        bitbucketPayload.PullRequest.Title,
			Description:  bitbucketPayload.PullRequest.Description,
			SourceBranch: bitbucketPayload.PullRequest.Source.Branch.Name,
			TargetBranch: bitbucketPayload.PullRequest.Destination.Branch.Name,
			URL:          bitbucketPayload.PullRequest.Links.HTML.Href,
			State:        strings.ToLower(bitbucketPayload.PullRequest.State),
			SHA:          bitbucketPayload.PullRequest.Source.Commit.Hash,
			AuthorID:     bitbucketPayload.PullRequest.Author.UUID,
			Author: &models.User{
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
func (p *Provider) GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*models.MergeRequest, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("%s/%s/repositories/%s/%s/pullrequests/%d",
		p.baseURL, p.apiVersion, workspace, repoSlug, mrIID)

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get pull request from Bitbucket")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	var pr BitbucketPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, errm.Wrap(err, "failed to decode pull request response")
	}

	// Convert reviewers
	var reviewers []*models.User
	for _, reviewer := range pr.Reviewers {
		reviewers = append(reviewers, &models.User{
			ID:       reviewer.User.UUID,
			Username: reviewer.User.Username,
			Name:     reviewer.User.DisplayName,
		})
	}

	mergeRequest := &models.MergeRequest{
		ID:           strconv.Itoa(pr.ID),
		IID:          pr.ID,
		Title:        pr.Title,
		Description:  pr.Description,
		SourceBranch: pr.Source.Branch.Name,
		TargetBranch: pr.Destination.Branch.Name,
		URL:          pr.Links.HTML.Href,
		State:        strings.ToLower(pr.State),
		SHA:          pr.Source.Commit.Hash,
		AuthorID:     pr.Author.UUID,
		Author: &models.User{
			ID:       pr.Author.UUID,
			Username: pr.Author.Username,
			Name:     pr.Author.DisplayName,
		},
		Reviewers: reviewers,
	}

	return mergeRequest, nil
}

// GetMergeRequestDiffs retrieves the diff for a pull request
func (p *Provider) GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*models.FileDiff, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL for diff
	apiURL := fmt.Sprintf("%s/%s/repositories/%s/%s/pullrequests/%d/diff",
		p.baseURL, p.apiVersion, workspace, repoSlug, mrIID)

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get diff from Bitbucket")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	// Read the diff content
	diffContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errm.Wrap(err, "failed to read diff content")
	}

	// Parse diff into FileDiff objects
	diffs := p.parseDiffContent(string(diffContent))

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
	apiURL := fmt.Sprintf("%s/%s/repositories/%s/%s/pullrequests/%d",
		p.baseURL, p.apiVersion, workspace, repoSlug, mrIID)

	// Prepare request body
	updateData := map[string]interface{}{
		"description": description,
	}

	jsonData, err := json.Marshal(updateData)
	if err != nil {
		return errm.Wrap(err, "failed to marshal update data")
	}

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return errm.Wrap(err, "failed to update pull request description")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	return nil
}

// CreateComment creates a comment on the pull request
func (p *Provider) CreateComment(ctx context.Context, projectID string, mrIID int, comment *models.ReviewComment) error {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("%s/%s/repositories/%s/%s/pullrequests/%d/comments",
		p.baseURL, p.apiVersion, workspace, repoSlug, mrIID)

	// Prepare comment data
	commentData := map[string]interface{}{
		"content": map[string]interface{}{
			"raw": comment.Body,
		},
	}

	// Add inline comment data if file path and line are specified
	if comment.FilePath != "" && comment.Line > 0 {
		commentData["inline"] = map[string]interface{}{
			"path": comment.FilePath,
			"to":   comment.Line,
		}
	}

	jsonData, err := json.Marshal(commentData)
	if err != nil {
		return errm.Wrap(err, "failed to marshal comment data")
	}

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return errm.Wrap(err, "failed to create comment")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	return nil
}

// GetComments retrieves comments from a pull request
func (p *Provider) GetComments(ctx context.Context, projectID string, mrIID int) ([]*models.ReviewComment, error) {
	// Parse workspace/repo_slug from projectID
	parts := strings.Split(projectID, "/")
	if len(parts) != 2 {
		return nil, errm.New("invalid Bitbucket project ID format, expected 'workspace/repo_slug'")
	}
	workspace, repoSlug := parts[0], parts[1]

	// Build API URL
	apiURL := fmt.Sprintf("%s/%s/repositories/%s/%s/pullrequests/%d/comments",
		p.baseURL, p.apiVersion, workspace, repoSlug, mrIID)

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get comments from Bitbucket")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	var response struct {
		Values []struct {
			ID      int `json:"id"`
			Content struct {
				Raw string `json:"raw"`
			} `json:"content"`
			User   BitbucketUser `json:"user"`
			Inline struct {
				Path string `json:"path"`
				To   int    `json:"to"`
			} `json:"inline"`
		} `json:"values"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errm.Wrap(err, "failed to decode comments response")
	}

	var comments []*models.ReviewComment
	for _, comment := range response.Values {
		reviewComment := &models.ReviewComment{
			ID:       strconv.Itoa(comment.ID),
			Body:     comment.Content.Raw,
			FilePath: comment.Inline.Path,
			Line:     comment.Inline.To,
			Author: &models.User{
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
func (p *Provider) GetCurrentUser(ctx context.Context) (*models.User, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/%s/user", p.baseURL, p.apiVersion)

	// Make API request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create request")
	}

	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get current user from Bitbucket")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errm.New(fmt.Sprintf("Bitbucket API error: %d - %s", resp.StatusCode, string(body)))
	}

	var user BitbucketUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errm.Wrap(err, "failed to decode user response")
	}

	return &models.User{
		ID:       user.UUID,
		Username: user.Username,
		Name:     user.DisplayName,
	}, nil
}

// setAuthHeaders sets authentication headers for API requests
func (p *Provider) setAuthHeaders(req *http.Request) {
	// Bitbucket uses HTTP Basic Auth with username:app_password
	// The username can be the actual username or "x-token-auth" for app passwords
	req.SetBasicAuth("x-token-auth", p.config.Token)
}

// parseDiffContent parses unified diff content into FileDiff objects
func (p *Provider) parseDiffContent(diffContent string) []*models.FileDiff {
	var diffs []*models.FileDiff
	lines := strings.Split(diffContent, "\n")

	var currentDiff *models.FileDiff
	var diffLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Save previous diff if exists
			if currentDiff != nil {
				currentDiff.Diff = strings.Join(diffLines, "\n")
				diffs = append(diffs, currentDiff)
			}

			// Start new diff
			currentDiff = &models.FileDiff{}
			diffLines = []string{line}
		} else if strings.HasPrefix(line, "--- ") && currentDiff != nil {
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
		} else if strings.HasPrefix(line, "+++ ") && currentDiff != nil {
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
		} else if currentDiff != nil {
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
