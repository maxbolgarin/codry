package agent

import (
	"slices"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/lang"
)

const (
	defaultTemperature = 0.5
	defaultMaxTokens   = 10000
	defaultTimeout     = 30 * time.Second
	defaultMaxRetries  = 5
	defaultRetryDelay  = 5 * time.Second
	defaultUserAgent   = "codry/0.1.0 (https://github.com/maxbolgarin/codry)"
)

// AgentType represents the type of AI agent
type AgentType string

// SupportedAgentTypes defines the supported AI agent types
const (
	Gemini AgentType = "gemini"
	OpenAI AgentType = "openai"
	Claude AgentType = "claude"
)

var supportedAgentTypes = []AgentType{Gemini, OpenAI, Claude}

// Config represents AI agent configuration
type Config struct {
	Type        AgentType `yaml:"type" env:"AGENT_TYPE"` // gemini, openai, claude, etc.
	APIKey      string    `yaml:"api_key" env:"AGENT_API_KEY"`
	Model       string    `yaml:"model" env:"AGENT_MODEL"`
	Temperature float32   `yaml:"temperature" env:"AGENT_TEMPERATURE"`
	MaxTokens   int       `yaml:"max_tokens" env:"AGENT_MAX_TOKENS"`

	BaseURL    string        `yaml:"base_url" env:"AGENT_BASE_URL"` // Custom API endpoint (Azure OpenAI, local models, etc.)
	ProxyURL   string        `yaml:"proxy_url" env:"AGENT_PROXY_URL"`
	MaxRetries int           `yaml:"max_retries" env:"AGENT_MAX_RETRIES"`
	RetryDelay time.Duration `yaml:"retry_delay" env:"AGENT_RETRY_DELAY"`
	Timeout    time.Duration `yaml:"timeout" env:"AGENT_TIMEOUT"`
	UserAgent  string        `yaml:"user_agent" env:"AGENT_USER_AGENT"`
	IsTest     bool          `yaml:"is_test" env:"AGENT_IS_TEST"`

	Language model.Language `yaml:"language" env:"AGENT_LANGUAGE"`
}

func (c *Config) PrepareAndValidate() error {
	if c.APIKey == "" {
		return erro.New("api key is required")
	}
	if c.Type == "" || !slices.Contains(supportedAgentTypes, c.Type) {
		return erro.New("invalid agent type: %s", c.Type)
	}

	c.Temperature = lang.Check(c.Temperature, defaultTemperature)
	c.MaxTokens = lang.Check(c.MaxTokens, defaultMaxTokens)
	c.Timeout = lang.Check(c.Timeout, defaultTimeout)
	c.MaxRetries = lang.Check(c.MaxRetries, defaultMaxRetries)
	c.RetryDelay = lang.Check(c.RetryDelay, defaultRetryDelay)
	c.UserAgent = lang.Check(c.UserAgent, defaultUserAgent)

	return nil
}
