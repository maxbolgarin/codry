package main

import (
	"github.com/alecthomas/kingpin/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/gitlab"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/errs"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/logger"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/metrics"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/panicsafe"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/shutdowner"
)

type Config struct {
	GitLabConfig gitlab.Config        `yaml:"gitlab"`
	AgentConfig  agent.Config         `yaml:"agent"`
	LoggerConfig metrics.LoggerConfig `yaml:"logger"`
}

var (
	Version, Branch, Commit, BuildDate string
)

func main() {
	config := kingpin.Flag("config", "path to config file").Short('c').String()
	kingpin.Parse()

	info := metrics.GetStartInfo("GitLab MR Reviewer", Branch, Commit, BuildDate)
	println(info)

	sd := shutdowner.New()

	var err error
	defer sd.ShutdownWithExit(&err)

	if err := run(sd, *config); err != nil {
		logger.Error(err, "cannot start")
		return
	}

	sd.Wait()
}

func run(ctx shutdowner.Context, configFile string) (err error) {
	defer panicsafe.RecoverWithErrAndStack(&err)

	cfg := Config{}
	if configFile == "" {
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return errs.Wrap(err, "failed to read config")
		}
	} else {
		if err := cleanenv.ReadConfig(configFile, &cfg); err != nil {
			return errs.Wrap(err, "failed to read config")
		}
	}

	if err := metrics.InitLogger(ctx, cfg.LoggerConfig); err != nil {
		return errs.Wrap(err, "failed to init logger")
	}

	gemini, err := agent.NewGemini(ctx, cfg.AgentConfig)
	if err != nil {
		return errs.Wrap(err, "failed to create agent")
	}

	gitlabClient, err := gitlab.New(cfg.GitLabConfig, gemini)
	if err != nil {
		return errs.Wrap(err, "failed to create gitlab client")
	}

	if err := gitlabClient.StartWebhookServer(ctx); err != nil {
		return errs.Wrap(err, "failed to start webhook server")
	}

	return nil
}
