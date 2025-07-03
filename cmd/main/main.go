package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/maxbolgarin/codry/internal/app"
	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

var (
	Version, Branch, Commit, BuildDate string
)

func main() {
	configPath := kingpin.Flag("config", "path to config file").Short('c').String()
	kingpin.Parse()

	// Print version info
	logze.Info("starting codry",
		"version", Version,
		"branch", Branch,
		"commit", Commit,
		"build_date", BuildDate,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logze.Info("received shutdown signal")
		cancel()
	}()

	if err := run(ctx, *configPath); err != nil {
		logze.Fatal(err, "application failed")
	}
}

func run(ctx context.Context, configFile string) error {
	// Load configuration
	cfg := &config.Config{}
	if configFile == "" {
		if err := cleanenv.ReadEnv(cfg); err != nil {
			return errm.Wrap(err, "failed to read config from environment")
		}
	} else {
		if err := cleanenv.ReadConfig(configFile, cfg); err != nil {
			return errm.Wrap(err, "failed to read config file")
		}
	}

	// Create logger
	logger := logze.With("service", "codry")

	// Create and initialize service
	codeReviewService, err := app.NewCodeReviewService(cfg, logger)
	if err != nil {
		return errm.Wrap(err, "failed to create code review service")
	}

	if err := codeReviewService.Initialize(ctx); err != nil {
		return errm.Wrap(err, "failed to initialize code review service")
	}

	// Start service (this will block until context is cancelled)
	if err := codeReviewService.Start(ctx); err != nil {
		return errm.Wrap(err, "failed to start code review service")
	}

	return nil
}
