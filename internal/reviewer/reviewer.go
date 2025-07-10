package reviewer

import (
	"context"
	"time"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
	"github.com/maxbolgarin/codry/internal/reviewer/llmcontext"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"github.com/panjf2000/ants/v2"
)

// Reviewer implements the ReviewService interface
type Reviewer struct {
	provider interfaces.CodeProvider
	agent    *agent.Agent
	pool     *ants.Pool
	parser   *astparser.DiffParser

	cfg Config
	log logze.Logger

	// Track processed MRs and reviewed files
	processedMRs *abstract.SafeMapOfMaps[string, string, string]

	// Track reviewed MRs to implement single review mode
	reviewedMRs *abstract.SafeMap[string, reviewTrackingInfo]

	// Context manager for gathering comprehensive MR metadata
	contextBuilder *llmcontext.Builder
}

// reviewTrackingInfo stores information about when an MR was reviewed
type reviewTrackingInfo struct {
	LastReviewedSHA string
	LastReviewedAt  time.Time
	ReviewCount     int
}

// New creates a new reviewer
func New(cfg Config, provider interfaces.CodeProvider, agent *agent.Agent) (*Reviewer, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, errm.Wrap(err, "failed to prepare and validate config")
	}

	pool, err := ants.NewPool(defaultPoolSize)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create ants pool")
	}

	s := &Reviewer{
		provider:       provider,
		agent:          agent,
		cfg:            cfg,
		log:            logze.With("component", "reviewer"),
		pool:           pool,
		parser:         astparser.NewDiffParser(),
		processedMRs:   abstract.NewSafeMapOfMaps[string, string, string](),
		reviewedMRs:    abstract.NewSafeMap[string, reviewTrackingInfo](),
		contextBuilder: llmcontext.NewBuilder(provider),
	}

	return s, nil
}

// HandleWebhook processes incoming webhook events and routes them appropriately
func (s *Reviewer) HandleEvent(ctx context.Context, event *model.CodeEvent) error {
	log := s.log.WithFields(
		"event_type", event.Type,
		"action", event.Action,
		"project_id", event.ProjectID,
		"user", event.User.Username,
	)

	log.Info("processing event")

	switch {
	case s.provider.IsMergeRequestEvent(event):
		return s.pool.Submit(func() {
			// TODO: add error handling
			err := s.ReviewMergeRequest(ctx, event.ProjectID, event.MergeRequest)
			if err != nil {
				log.Error("error processing merge request event", "error", err)
			}
		})

	// case s.provider.IsCommentEvent(event):
	// 	return s.pool.Submit(func() {
	// 		// TODO: add error handling
	// 		s.processCommentEvent(ctx, event, log)
	// 	})

	default:
		log.Debug("unhandled webhook event type")
		return nil
	}
}

func (s *Reviewer) logFlow(msg string, fields ...any) {
	if s.cfg.Verbose {
		s.log.Info(msg, fields...)
	} else {
		s.log.Debug(msg, fields...)
	}
}
