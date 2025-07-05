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
	IsCommentEvent(event *model.CodeEvent) bool

	// MR/PR operations
	GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*model.MergeRequest, error)
	GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*model.FileDiff, error)
	UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error

	// Multiple MR operations
	ListMergeRequests(ctx context.Context, projectID string, filter *model.MergeRequestFilter) ([]*model.MergeRequest, error)
	GetMergeRequestUpdates(ctx context.Context, projectID string, since time.Time) ([]*model.MergeRequest, error)

	// Comments
	CreateComment(ctx context.Context, projectID string, mrIID int, comment *model.Comment) error
	ReplyToComment(ctx context.Context, projectID string, mrIID int, commentID string, reply string) error
	GetComments(ctx context.Context, projectID string, mrIID int) ([]*model.Comment, error)
	GetComment(ctx context.Context, projectID string, mrIID int, commentID string) (*model.Comment, error)

	// User operations
	GetCurrentUser(ctx context.Context) (*model.User, error)
}

// AIAgent defines the interface for AI code review agents
type AIAgent interface {
	GenerateDescription(ctx context.Context, fullDiff string) (string, error)
	ReviewCode(ctx context.Context, filePath, diff string) (string, error)
	SummarizeChanges(ctx context.Context, changes []*model.FileDiff) (string, error)
	GenerateCommentReply(ctx context.Context, originalComment, replyContext string) (string, error)
}

// AgentAPI defines the interface for calling LLM AI models
type AgentAPI interface {
	CallAPI(ctx context.Context, req model.APIRequest) (model.APIResponse, error)
}

// PromptBuilder defines the interface for building prompts for AI agents
type PromptBuilder interface {
	BuildDescriptionPrompt(diff string) model.Prompt
	BuildReviewPrompt(filename, diff string) model.Prompt
	BuildSummaryPrompt(changes []*model.FileDiff) model.Prompt
	BuildCommentReplyPrompt(originalComment, replyContext string) model.Prompt
}
