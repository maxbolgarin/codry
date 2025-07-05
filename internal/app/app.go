package app

import (
	"context"

	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/provider"
	"github.com/maxbolgarin/codry/internal/reviewer"
	"github.com/maxbolgarin/codry/internal/server"
	"github.com/maxbolgarin/contem"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Codry is the main service that orchestrates all components
type Codry struct {
	provider       interfaces.CodeProvider
	agent          interfaces.AIAgent
	reviewer       *reviewer.Reviewer
	webhookHandler *server.Server
	fetcher        *provider.Fetcher

	cfg config.Config
	log logze.Logger
}

// New creates a new code review service
func New(ctx contem.Context, cfg config.Config) (*Codry, error) {
	service := &Codry{
		cfg: cfg,
		log: logze.With("component", "app"),
	}

	if err := service.init(ctx, cfg); err != nil {
		return nil, errm.Wrap(err, "failed to initialize service")
	}

	return service, nil
}

func (s *Codry) StartWebhook(ctx context.Context) error {
	if err := s.webhookHandler.Start(ctx); err != nil {
		return errm.Wrap(err, "failed to start webhook handler")
	}
	return nil
}

func (s *Codry) RunReview(ctx context.Context, projectID string) error {
	mrs, err := s.fetcher.FetchOpenMRs(ctx, projectID)
	if err != nil {
		return errm.Wrap(err, "failed to fetch recent merge requests")
	}
	for _, mr := range mrs {
		err := s.reviewer.ReviewMergeRequest(ctx, projectID, mr)
		if err != nil {
			return errm.Wrap(err, "failed to review merge request")
		}
	}
	return nil
}

func (s *Codry) init(ctx contem.Context, cfg config.Config) (err error) {

	// Create VCS provider
	s.provider, err = provider.NewProvider(cfg.Provider)
	if err != nil {
		return errm.Wrap(err, "failed to create VCS provider")
	}
	s.fetcher = provider.NewFetcher(s.provider)

	// Create AI agent
	s.agent, err = agent.New(ctx, cfg.Agent)
	if err != nil {
		return errm.Wrap(err, "failed to create AI agent")
	}

	// Create review service - this is the central orchestrator
	s.reviewer, err = reviewer.New(cfg.Reviewer, s.provider, s.agent)
	if err != nil {
		return errm.Wrap(err, "failed to create review service")
	}

	// Create webhook handler - just an event source
	s.webhookHandler, err = server.New(cfg.Server, s.provider, s.reviewer)
	if err != nil {
		return errm.Wrap(err, "failed to create webhook handler")
	}
	ctx.Add(s.webhookHandler.Stop)

	return nil
}
