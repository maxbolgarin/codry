package main

import (
	"github.com/alecthomas/kingpin/v2"
	"github.com/maxbolgarin/codry/internal/app"
	"github.com/maxbolgarin/contem"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/logze/v2"
)

var (
	Version, Branch, Commit, BuildDate string
)

var (
	configPath = kingpin.Flag("config", "path to config file").Short('c').String()
)

func main() {
	kingpin.Parse()
	//contem.Start(run, logze.DefaultPtr())
	var err error
	ctx := contem.New(contem.WithLogger(logze.DefaultPtr()), contem.Exit(&err))
	defer ctx.Shutdown()
	err = run(ctx)
	if err != nil {
		logze.DefaultPtr().Error("cannot run", "error", err)
	}
}

func run(ctx contem.Context) error {
	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		return erro.Wrap(err, "load config")
	}
	logze.Init(logze.C().WithConsole().WithLevel(logze.LevelDebug))

	codry, err := app.New(ctx, cfg)
	if err != nil {
		return erro.Wrap(err, "new provider")
	}

	if err := codry.RunReview(ctx, "maxbolgarin/codry"); err != nil {
		return erro.Wrap(err, "run review")
	}

	return nil
}
