package agent

import (
	"context"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/agent/claude"
	"github.com/maxbolgarin/codry/internal/agent/gemini"
	"github.com/maxbolgarin/codry/internal/agent/openai"
	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

var _ interfaces.AIAgent = (*Agent)(nil)

type Agent struct {
	cfg    Config
	logger logze.Logger
	pb     interfaces.PromptBuilder
	api    interfaces.AgentAPI
}

func New(ctx context.Context, cfg Config) (interfaces.AIAgent, error) {
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
		pb:     prompts.NewBuilder(cfg.Language),
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
	return a.apiCall(ctx, a.pb.BuildDescriptionPrompt(diff))
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, diff string) (string, error) {
	return a.apiCall(ctx, a.pb.BuildReviewPrompt(filename, diff))
}

// SummarizeChanges summarizes the changes in a set of files
func (a *Agent) SummarizeChanges(ctx context.Context, changes []*model.FileDiff) (string, error) {
	return a.apiCall(ctx, a.pb.BuildSummaryPrompt(changes))
}

// GenerateCommentReply generates a reply to a comment
func (a *Agent) GenerateCommentReply(ctx context.Context, originalComment, replyContext string) (string, error) {
	return a.apiCall(ctx, a.pb.BuildCommentReplyPrompt(originalComment, replyContext))
}

func (a *Agent) apiCall(ctx context.Context, prompt model.Prompt) (string, error) {
	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to call API")
	}
	return response.Content, nil
}
