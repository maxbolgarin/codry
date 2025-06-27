package config

import (
	"time"

	"github.com/maxbolgarin/codry/internal/models"
)

// Config represents the main application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Provider ProviderConfig `yaml:"provider"`
	Agent    AgentConfig    `yaml:"agent"`
	Review   ReviewConfig   `yaml:"review"`
}

// ServerConfig represents webhook server configuration
type ServerConfig struct {
	Address  string        `yaml:"address" env:"SERVER_ADDRESS"`
	Endpoint string        `yaml:"endpoint" env:"SERVER_ENDPOINT"`
	Timeout  time.Duration `yaml:"timeout" env:"SERVER_TIMEOUT"`
}

// ProviderConfig represents VCS provider configuration
type ProviderConfig struct {
	Type          string        `yaml:"type" env:"PROVIDER_TYPE"` // gitlab, github, etc.
	BaseURL       string        `yaml:"base_url" env:"PROVIDER_BASE_URL"`
	Token         string        `yaml:"token" env:"PROVIDER_TOKEN"`
	WebhookSecret string        `yaml:"webhook_secret" env:"PROVIDER_WEBHOOK_SECRET"`
	BotUsername   string        `yaml:"bot_username" env:"PROVIDER_BOT_USERNAME"`
	RateLimitWait time.Duration `yaml:"rate_limit_wait" env:"PROVIDER_RATE_LIMIT_WAIT"`
}

// AgentConfig represents AI agent configuration
type AgentConfig struct {
	Type        string        `yaml:"type" env:"AGENT_TYPE"` // gemini, openai, claude, etc.
	APIKey      string        `yaml:"api_key" env:"AGENT_API_KEY"`
	Model       string        `yaml:"model" env:"AGENT_MODEL"`
	BaseURL     string        `yaml:"base_url" env:"AGENT_BASE_URL"` // Custom API endpoint (Azure OpenAI, local models, etc.)
	ProxyURL    string        `yaml:"proxy_url" env:"AGENT_PROXY_URL"`
	MaxRetries  int           `yaml:"max_retries" env:"AGENT_MAX_RETRIES"`
	RetryDelay  time.Duration `yaml:"retry_delay" env:"AGENT_RETRY_DELAY"`
	Temperature float32       `yaml:"temperature" env:"AGENT_TEMPERATURE"`
	MaxTokens   int           `yaml:"max_tokens" env:"AGENT_MAX_TOKENS"`
}

// ReviewConfig represents code review behavior configuration
type ReviewConfig struct {
	FileFilter                  models.FileFilter `yaml:"file_filter"`
	MaxFilesPerMR               int               `yaml:"max_files_per_mr" env:"REVIEW_MAX_FILES_PER_MR"`
	UpdateDescriptionOnMR       bool              `yaml:"update_description_on_mr" env:"REVIEW_UPDATE_DESCRIPTION_ON_MR"`
	EnableDescriptionGeneration bool              `yaml:"enable_description_generation" env:"REVIEW_ENABLE_DESCRIPTION_GENERATION"`
	EnableCodeReview            bool              `yaml:"enable_code_review" env:"REVIEW_ENABLE_CODE_REVIEW"`
	MinFilesForDescription      int               `yaml:"min_files_for_description" env:"REVIEW_MIN_FILES_FOR_DESCRIPTION"`
	ProcessingDelay             time.Duration     `yaml:"processing_delay" env:"REVIEW_PROCESSING_DELAY"`
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Provider.Token == "" {
		return ErrMissingProviderToken
	}
	if c.Provider.Type == "" {
		return ErrMissingProviderType
	}
	if c.Agent.APIKey == "" {
		return ErrMissingAgentAPIKey
	}
	if c.Agent.Type == "" {
		return ErrMissingAgentType
	}
	return nil
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	// Server defaults
	if c.Server.Address == "" {
		c.Server.Address = ":8080"
	}
	if c.Server.Endpoint == "" {
		c.Server.Endpoint = "/webhook"
	}
	if c.Server.Timeout == 0 {
		c.Server.Timeout = 30 * time.Second
	}

	// Provider defaults
	if c.Provider.RateLimitWait == 0 {
		c.Provider.RateLimitWait = 1 * time.Minute
	}

	// Agent defaults
	if c.Agent.MaxRetries == 0 {
		c.Agent.MaxRetries = 3
	}
	if c.Agent.RetryDelay == 0 {
		c.Agent.RetryDelay = 5 * time.Second
	}
	if c.Agent.Temperature == 0 {
		c.Agent.Temperature = 0.1
	}
	if c.Agent.MaxTokens == 0 {
		c.Agent.MaxTokens = 4000
	}

	// Review defaults
	if c.Review.MaxFilesPerMR == 0 {
		c.Review.MaxFilesPerMR = 50
	}
	if c.Review.MinFilesForDescription == 0 {
		c.Review.MinFilesForDescription = 3
	}
	if c.Review.ProcessingDelay == 0 {
		c.Review.ProcessingDelay = 5 * time.Second
	}
	if c.Review.FileFilter.MaxFileSize == 0 {
		c.Review.FileFilter.MaxFileSize = 10000
	}
	if len(c.Review.FileFilter.AllowedExtensions) == 0 {
		c.Review.FileFilter.AllowedExtensions = []string{
			".go", ".js", ".ts", ".py", ".java", ".cpp", ".c", ".cs", ".php", ".rb", ".rs", ".kt", ".swift",
			".yaml", ".yml", ".json", ".xml", ".toml", ".ini", ".cfg", ".conf",
			".sql", ".md", ".dockerfile", ".sh", ".bash", ".ps1",
		}
	}
	if len(c.Review.FileFilter.ExcludedPaths) == 0 {
		c.Review.FileFilter.ExcludedPaths = []string{
			"vendor/", "node_modules/", ".git/", "dist/", "build/", "target/",
			"*.min.js", "*.min.css", "*.bundle.js", "*.generated.*",
		}
	}
}

// GetProviderConfig returns provider-specific configuration
func (c *Config) GetProviderConfig() models.ProviderConfig {
	return models.ProviderConfig{
		Type:          c.Provider.Type,
		BaseURL:       c.Provider.BaseURL,
		Token:         c.Provider.Token,
		WebhookSecret: c.Provider.WebhookSecret,
		BotUsername:   c.Provider.BotUsername,
	}
}
