package provider

import (
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/provider/bitbucket"
	"github.com/maxbolgarin/codry/internal/provider/github"
	"github.com/maxbolgarin/codry/internal/provider/gitlab"
	"github.com/maxbolgarin/erro"
)

// NewProvider creates a new VCS provider based on the configuration
func NewProvider(cfg Config) (interfaces.CodeProvider, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, erro.Wrap(err, "validate config")
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
		provider, err = gitlab.New(cfgForProvider)
	case GitHub:
		provider, err = github.New(cfgForProvider)
	case Bitbucket:
		provider, err = bitbucket.New(cfgForProvider)
	default:
		return nil, erro.New("unsupported provider type: %s", cfg.Type)
	}
	if err != nil {
		return nil, erro.Wrap(err, "failed to create provider")
	}

	return provider, nil
}
