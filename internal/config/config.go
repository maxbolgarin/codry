package config

import (
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/provider"
	"github.com/maxbolgarin/codry/internal/server"
	"github.com/maxbolgarin/errm"
)

// Config represents the main application configuration
type Config struct {
	Server   server.Config   `yaml:"server"`
	Provider provider.Config `yaml:"provider"`
	Agent    agent.Config    `yaml:"agent"`
	Review   ReviewConfig    `yaml:"review"`
}

// ReviewConfig represents code review behavior configuration
type ReviewConfig struct {
	FileFilter                  model.FileFilter `yaml:"file_filter"`
	MaxFilesPerMR               int              `yaml:"max_files_per_mr" env:"REVIEW_MAX_FILES_PER_MR"`
	UpdateDescriptionOnMR       bool             `yaml:"update_description_on_mr" env:"REVIEW_UPDATE_DESCRIPTION_ON_MR"`
	EnableDescriptionGeneration bool             `yaml:"enable_description_generation" env:"REVIEW_ENABLE_DESCRIPTION_GENERATION"`
	EnableCodeReview            bool             `yaml:"enable_code_review" env:"REVIEW_ENABLE_CODE_REVIEW"`
	MinFilesForDescription      int              `yaml:"min_files_for_description" env:"REVIEW_MIN_FILES_FOR_DESCRIPTION"`
	ProcessingDelay             time.Duration    `yaml:"processing_delay" env:"REVIEW_PROCESSING_DELAY"`
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}

	if path == "" {
		if err := cleanenv.ReadEnv(cfg); err != nil {
			return nil, errm.Wrap(err, "failed to load config from env")
		}
		return cfg, nil
	}

	if err := cleanenv.ReadConfig(path, cfg); err != nil {
		return nil, errm.Wrap(err, "failed to load config")
	}

	return cfg, nil
}
