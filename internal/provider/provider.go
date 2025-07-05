package provider

import (
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/provider/bitbucket"
	"github.com/maxbolgarin/codry/internal/provider/github"
	"github.com/maxbolgarin/codry/internal/provider/gitlab"
	"github.com/maxbolgarin/errm"
)

// NewProvider creates a new VCS provider based on the configuration
func NewProvider(cfg Config) (interfaces.CodeProvider, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, errm.Wrap(err, "validate config")
	}

	cfgForProvider := model.ProviderConfig{
		BaseURL:       cfg.BaseURL,
		Token:         cfg.Token,
		WebhookSecret: cfg.WebhookSecret,
		BotUsername:   cfg.BotUsername,
	}

	var provider interfaces.CodeProvider
	var err error

	switch cfg.Type {
	case GitLab:
		provider, err = gitlab.NewProvider(cfgForProvider)
	case GitHub:
		provider, err = github.NewProvider(cfgForProvider)
	case Bitbucket:
		provider, err = bitbucket.New(cfgForProvider)
	default:
		return nil, errm.Errorf("unsupported provider type: %s", cfg.Type)
	}
	if err != nil {
		return nil, errm.Wrap(err, "failed to create provider")
	}

	return provider, nil
}
