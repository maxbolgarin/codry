package service

import (
	"context"

	"github.com/maxbolgarin/codry/internal/agents"
	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/codry/internal/providers"
	"github.com/maxbolgarin/codry/internal/review"
	"github.com/maxbolgarin/codry/internal/webhook"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// CodeReviewService is the main service that orchestrates all components
type CodeReviewService struct {
	config         *config.Config
	logger         logze.Logger
	provider       models.CodeProvider
	agent          models.AIAgent
	reviewService  models.ReviewService
	webhookHandler *webhook.Handler
}

// NewCodeReviewService creates a new code review service
func NewCodeReviewService(cfg *config.Config, logger logze.Logger) (*CodeReviewService, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, errm.Wrap(err, "invalid configuration")
	}

	// Set defaults
	cfg.SetDefaults()

	// Validate provider and agent types
	if err := providers.ValidateProviderType(cfg.Provider.Type); err != nil {
		return nil, errm.Wrap(err, "invalid provider configuration")
	}

	if err := agents.ValidateAgentType(cfg.Agent.Type); err != nil {
		return nil, errm.Wrap(err, "invalid agent configuration")
	}

	service := &CodeReviewService{
		config: cfg,
		logger: logger,
	}

	return service, nil
}

// Initialize initializes all service components
func (s *CodeReviewService) Initialize(ctx context.Context) error {
	s.logger.Info("initializing code review service",
		"provider_type", s.config.Provider.Type,
		"agent_type", s.config.Agent.Type,
	)

	// Create VCS provider
	var err error
	s.provider, err = providers.NewProvider(s.config.GetProviderConfig(), s.logger)
	if err != nil {
		return errm.Wrap(err, "failed to create VCS provider")
	}

	// Create AI agent
	s.agent, err = agents.NewAgent(ctx, s.config.Agent)
	if err != nil {
		return errm.Wrap(err, "failed to create AI agent")
	}

	// Create review service
	s.reviewService = review.NewService(s.provider, s.agent, s.config, s.logger)

	// Create webhook handler
	s.webhookHandler = webhook.NewHandler(s.provider, s.reviewService, s.config, s.logger)

	s.logger.Info("code review service initialized successfully")
	return nil
}

// Start starts the code review service
func (s *CodeReviewService) Start(ctx context.Context) error {
	s.logger.Info("starting code review service")

	// Start webhook server
	if err := s.webhookHandler.Start(ctx); err != nil {
		return errm.Wrap(err, "failed to start webhook handler")
	}

	s.logger.Info("code review service started successfully")

	// Wait for context cancellation
	<-ctx.Done()
	s.logger.Info("code review service stopped")

	return nil
}

// GetProvider returns the VCS provider instance
func (s *CodeReviewService) GetProvider() models.CodeProvider {
	return s.provider
}

// GetAgent returns the AI agent instance
func (s *CodeReviewService) GetAgent() models.AIAgent {
	return s.agent
}

// GetReviewService returns the review service instance
func (s *CodeReviewService) GetReviewService() models.ReviewService {
	return s.reviewService
}

// GetConfig returns the service configuration
func (s *CodeReviewService) GetConfig() *config.Config {
	return s.config
}

// HealthCheck performs a health check on all service components
func (s *CodeReviewService) HealthCheck(ctx context.Context) error {
	s.logger.Debug("performing health check")

	// Check provider connectivity
	if s.provider != nil {
		_, err := s.provider.GetCurrentUser(ctx)
		if err != nil {
			return errm.Wrap(err, "provider health check failed")
		}
	}

	// Note: AI agent health check could be added here if needed
	// but it might consume API quota unnecessarily

	s.logger.Debug("health check passed")
	return nil
}
