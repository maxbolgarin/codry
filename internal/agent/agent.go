package agent

import (
	"context"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/agent/claude"
	"github.com/maxbolgarin/codry/internal/agent/gemini"
	"github.com/maxbolgarin/codry/internal/agent/openai"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

var _ model.AIAgent = (*Agent)(nil)

type Agent struct {
	cfg    Config
	logger logze.Logger
	pb     model.PromptBuilder
	api    model.AgentAPI
}

func New(ctx context.Context, cfg Config, promptBuilder model.PromptBuilder) (model.AIAgent, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, errm.Wrap(err, "validate config")
	}
	cli, err := cliex.NewWithConfig(cliex.Config{
		BaseURL:        cfg.BaseURL,
		UserAgent:      cfg.UserAgent,
		ProxyAddress:   cfg.ProxyURL,
		RequestTimeout: cfg.Timeout,
	})
	if err != nil {
		return nil, errm.Wrap(err, "failed to create HTTP client")
	}

	agent := &Agent{
		cfg:    cfg,
		logger: logze.Default(),
		pb:     promptBuilder,
	}

	modelCfg := model.ModelConfig{
		APIKey:   cfg.APIKey,
		Model:    cfg.Model,
		URL:      cfg.BaseURL,
		ProxyURL: cfg.ProxyURL,
		IsTest:   cfg.IsTest,
	}

	switch cfg.Type {
	case Gemini:
		agent.api, err = gemini.New(ctx, modelCfg)
	case OpenAI:
		agent.api, err = openai.New(ctx, cli, modelCfg)
	case Claude:
		agent.api, err = claude.New(ctx, cli, modelCfg)
	default:
		return nil, errm.Errorf("unsupported agent type: %s", cfg.Type)
	}
	if err != nil {
		return nil, errm.Wrap(err, "failed to create agent")
	}

	return agent, nil
}

// GenerateDescription generates a description for code changes
func (a *Agent) GenerateDescription(ctx context.Context, diff string) (string, error) {
	prompt := a.pb.BuildDescriptionPrompt(diff)

	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to generate description")
	}
	return response.Content, nil
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, diff string) (string, error) {
	prompt := a.pb.BuildReviewPrompt(filename, diff)

	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to review code")
	}

	return response.Content, nil
}

// SummarizeChanges summarizes the changes in a set of files
func (a *Agent) SummarizeChanges(ctx context.Context, changes []*model.FileDiff) (string, error) {
	prompt := a.pb.BuildSummaryPrompt(changes)

	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to summarize changes")
	}

	return response.Content, nil
}
