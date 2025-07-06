package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

type Agent struct {
	cfg    Config
	logger logze.Logger
	pb     *prompts.Builder
	api    interfaces.AgentAPI
}

func New(ctx context.Context, cfg Config) (*Agent, error) {
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
func (a *Agent) ReviewCode(ctx context.Context, filename, fullFileContent, cleanDiff string) (*model.FileReviewResult, error) {
	prompt := a.pb.BuildReviewPrompt(filename, fullFileContent, cleanDiff)
	response, err := a.apiCall(ctx, prompt)
	if err != nil {
		return nil, errm.Wrap(err, "failed to call API for enhanced structured review")
	}

	result, err := unmarshal[model.FileReviewResult](response)
	if err != nil {
		fmt.Println(response)
		return nil, errm.Wrap(err, "failed to parse enhanced structured review response as JSON")
	}

	result.FilePath = filename

	return &result, nil
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

func unmarshal[T any](response string) (T, error) {
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```", "")
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "[")
	response = strings.TrimSuffix(response, "]")

	var result T
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return result, errm.Wrap(err, "failed to parse structured review response as JSON")
	}
	return result, nil
}
