package provider

import (
	"slices"
	"time"

	"github.com/maxbolgarin/errm"
)

type ProviderType string

// SupportedProviderTypes defines the supported VCS provider types
const (
	GitLab    ProviderType = "gitlab"
	GitHub    ProviderType = "github"
	Bitbucket ProviderType = "bitbucket"
)

var supportedProviderTypes = []ProviderType{GitLab, GitHub, Bitbucket}

// Config represents VCS provider configuration
type Config struct {
	Type          ProviderType  `yaml:"type" env:"PROVIDER_TYPE"`
	BaseURL       string        `yaml:"base_url" env:"PROVIDER_BASE_URL"`
	Token         string        `yaml:"token" env:"PROVIDER_TOKEN"`
	WebhookSecret string        `yaml:"webhook_secret" env:"PROVIDER_WEBHOOK_SECRET"`
	BotUsername   string        `yaml:"bot_username" env:"PROVIDER_BOT_USERNAME"`
	RateLimitWait time.Duration `yaml:"rate_limit_wait" env:"PROVIDER_RATE_LIMIT_WAIT"`
}

func (c *Config) PrepareAndValidate() error {
	if c.Token == "" {
		return errm.New("token is required")
	}

	if c.Type == "" || !slices.Contains(supportedProviderTypes, c.Type) {
		return errm.New("invalid provider type: %s", c.Type)
	}

	return nil
}
