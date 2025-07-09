package interfaces

import (
	"context"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
)

// CodeProvider defines the interface for different VCS providers (GitLab, GitHub, etc.)
type CodeProvider interface {
	// Webhook handling
	ValidateWebhook(payload []byte, authToken string) error
	ParseWebhookEvent(payload []byte) (*model.CodeEvent, error)
	IsMergeRequestEvent(event *model.CodeEvent) bool

	// MR/PR operations
	GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*model.MergeRequest, error)
	GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*model.FileDiff, error)
	UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error

	// Multiple MR operations
	ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error)
	GetMergeRequestUpdates(ctx context.Context, projectID string, since time.Time) ([]*model.MergeRequest, error)

	// Comments
	CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error
	GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error)
	UpdateComment(ctx context.Context, projectID string, mrIID int, commentID string, newBody string) error

	// GetFileContent retrieves the content of a file at a specific commit/SHA
	GetFileContent(ctx context.Context, projectID, filePath, commitSHA string) (string, error)

	// Commit operations
	GetMergeRequestCommits(ctx context.Context, projectID string, mrIID int) ([]*model.Commit, error)
	GetCommitDetails(ctx context.Context, projectID, commitSHA string) (*model.Commit, error)
	GetCommitDiffs(ctx context.Context, projectID, commitSHA string) ([]*model.FileDiff, error)
}

// AgentAPI defines the interface for calling LLM AI models
type AgentAPI interface {
	CallAPI(ctx context.Context, req model.APIRequest) (model.APIResponse, error)
}
