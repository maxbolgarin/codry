package model

import (
	"context"
	"time"
)

// ReviewService defines the interface for the core review service
type ReviewService interface {
	HandleEvent(ctx context.Context, event *CodeEvent) error

	ProcessMergeRequest(ctx context.Context, request *ReviewRequest) (*ReviewResult, error)
	ProcessMergeRequestUpdate(ctx context.Context, request *ReviewRequest) (*ReviewResult, error)
	ProcessCommentReply(ctx context.Context, projectID string, mrIID int, comment *Comment) error
}

// CodeProvider defines the interface for different VCS providers (GitLab, GitHub, etc.)
type CodeProvider interface {
	// Webhook handling
	ValidateWebhook(payload []byte, authToken string) error
	ParseWebhookEvent(payload []byte) (*CodeEvent, error)
	IsMergeRequestEvent(event *CodeEvent) bool
	IsCommentEvent(event *CodeEvent) bool

	// MR/PR operations
	GetMergeRequest(ctx context.Context, projectID string, mrIID int) (*MergeRequest, error)
	GetMergeRequestDiffs(ctx context.Context, projectID string, mrIID int) ([]*FileDiff, error)
	UpdateMergeRequestDescription(ctx context.Context, projectID string, mrIID int, description string) error

	// Multiple MR operations
	ListMergeRequests(ctx context.Context, projectID string, filter *MergeRequestFilter) ([]*MergeRequest, error)
	GetMergeRequestUpdates(ctx context.Context, projectID string, since time.Time) ([]*MergeRequest, error)

	// Comments
	CreateComment(ctx context.Context, projectID string, mrIID int, comment *ReviewComment) error
	ReplyToComment(ctx context.Context, projectID string, mrIID int, commentID string, reply string) error
	GetComments(ctx context.Context, projectID string, mrIID int) ([]*ReviewComment, error)
	GetComment(ctx context.Context, projectID string, mrIID int, commentID string) (*Comment, error)

	// User operations
	GetCurrentUser(ctx context.Context) (*User, error)
}
