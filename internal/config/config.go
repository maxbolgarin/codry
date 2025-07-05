package config

import (
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/provider"
	"github.com/maxbolgarin/codry/internal/reviewer"
	"github.com/maxbolgarin/codry/internal/server"
	"github.com/maxbolgarin/errm"
)

// Config represents the main application configuration
type Config struct {
	Provider provider.Config `yaml:"provider"`
	Agent    agent.Config    `yaml:"agent"`
	Reviewer reviewer.Config `yaml:"review"`

	Server server.Config `yaml:"server"`
}

func Load(path string) (Config, error) {
	cfg := Config{}

	if path == "" {
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return Config{}, errm.Wrap(err, "failed to load config from env")
		}
		return cfg, nil
	}

	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return Config{}, errm.Wrap(err, "failed to load config")
	}

	return cfg, nil
}
