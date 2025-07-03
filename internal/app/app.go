package app

import (
	"context"

	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/prompts"
	"github.com/maxbolgarin/codry/internal/provider"
	"github.com/maxbolgarin/codry/internal/review"
	"github.com/maxbolgarin/codry/internal/server"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Codry is the main service that orchestrates all components
type Codry struct {
	config         *config.Config
	logger         logze.Logger
	provider       model.CodeProvider
	agent          model.AIAgent
	reviewService  model.ReviewService
	webhookHandler *server.Server
}

// NewCodry creates a new code review service
func NewCodry(cfg *config.Config, logger logze.Logger) (*Codry, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, errm.Wrap(err, "invalid configuration")
	}

	// Set defaults
	cfg.SetDefaults()

	// Validate provider and agent types
	if err := provider.ValidateProviderType(cfg.Provider.Type); err != nil {
		return nil, errm.Wrap(err, "invalid provider configuration")
	}

	if err := ValidateAgentType(string(cfg.Agent.Type)); err != nil {
		return nil, errm.Wrap(err, "invalid agent configuration")
	}

	service := &Codry{
		config: cfg,
		logger: logger,
	}

	return service, nil
}

// ValidateAgentType checks if the given agent type is supported
func ValidateAgentType(agentType string) error {
	supportedTypes := []string{
		string(agent.Gemini),
		string(agent.OpenAI),
		string(agent.Claude),
	}
	for _, supportedType := range supportedTypes {
		if agentType == supportedType {
			return nil
		}
	}
	return errm.Errorf("unsupported agent type: %s, supported types: %v", agentType, supportedTypes)
}

// Initialize initializes all service components
func (s *Codry) Initialize(ctx context.Context) error {
	s.logger.Info("initializing code review service",
		"provider_type", s.config.Provider.Type,
		"agent_type", s.config.Agent.Type,
	)

	// Create VCS provider
	var err error
	s.provider, err = provider.NewProvider(s.config.GetProviderConfig(), s.logger)
	if err != nil {
		return errm.Wrap(err, "failed to create VCS provider")
	}

	// Create prompt builder
	promptBuilder := prompts.NewBuilder(model.LanguageEnglish)

	// Create AI agent
	s.agent, err = agent.New(ctx, s.config.Agent, promptBuilder)
	if err != nil {
		return errm.Wrap(err, "failed to create AI agent")
	}

	// Create review service - this is the central orchestrator
	s.reviewService = review.NewService(s.provider, s.agent, s.config, s.logger)

	// Create webhook handler - just an event source
	s.webhookHandler, err = server.New(s.config.GetWebhookConfig(), s.provider, s.reviewService)
	if err != nil {
		return errm.Wrap(err, "failed to create webhook handler")
	}

	s.logger.Info("code review service initialized successfully")
	return nil
}

// Start starts the code review service
func (s *Codry) Start(ctx context.Context) error {
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
func (s *Codry) GetProvider() model.CodeProvider {
	return s.provider
}

// GetAgent returns the AI agent instance
func (s *Codry) GetAgent() model.AIAgent {
	return s.agent
}

// GetReviewService returns the review service instance
func (s *Codry) GetReviewService() model.ReviewService {
	return s.reviewService
}

// GetConfig returns the service configuration
func (s *Codry) GetConfig() *config.Config {
	return s.config
}

// HealthCheck performs a health check on all service components
func (s *Codry) HealthCheck(ctx context.Context) error {
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
