package agent

import (
	"context"
	"os"
	"path"

	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/agent/claude"
	"github.com/maxbolgarin/codry/internal/agent/gemini"
	"github.com/maxbolgarin/codry/internal/agent/openai"
	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
	"github.com/maxbolgarin/codry/internal/reviewer/llmcontext"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Agent struct {
	cfg Config
	log logze.Logger
	pb  *prompts.Builder
	api interfaces.AgentAPI
}

func New(ctx context.Context, cfg Config) (*Agent, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, erro.Wrap(err, "validate config")
	}
	cli, err := cliex.NewWithConfig(cliex.Config{
		BaseURL:        cfg.BaseURL,
		UserAgent:      cfg.UserAgent,
		ProxyAddress:   cfg.ProxyURL,
		RequestTimeout: cfg.Timeout,
	})
	if err != nil {
		return nil, erro.Wrap(err, "failed to create HTTP client")
	}

	agent := &Agent{
		cfg: cfg,
		log: logze.With("llm", cfg.Type, "component", "agent"),
		pb:  prompts.NewBuilder(cfg.Language),
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
		return nil, erro.New("unsupported agent type: %s", cfg.Type)
	}
	if err != nil {
		return nil, erro.Wrap(err, "failed to create agent")
	}

	return agent, nil
}

// GenerateDescription generates a description for code changes
func (a *Agent) GenerateDescription(ctx context.Context, diff string) (string, error) {
	response, err := a.apiCall(ctx, a.pb.BuildDescriptionPrompt(diff), false)
	if err != nil {
		return "", erro.Wrap(err, "failed to call API for description")
	}

	a.log.Debug("description generated",
		"input_tokens", response.PromptTokens,
		"output_tokens", response.CompletionTokens,
		"total_tokens", response.TotalTokens,
	)

	return response.Content, nil
}

// GenerateChangesOverview generates an overview of code changes§
func (a *Agent) GenerateChangesOverview(ctx context.Context, diff string) ([]model.FileChangeInfo, error) {
	prompt := a.pb.BuildChangesOverviewPrompt(diff)
	response, err := a.apiCall(ctx, prompt, true)
	if err != nil {
		return nil, erro.Wrap(err, "failed to call API for changes overview")
	}

	a.log.Debug("changes overview generated",
		"input_tokens", response.PromptTokens,
		"output_tokens", response.CompletionTokens,
		"total_tokens", response.TotalTokens,
	)

	var result []model.FileChangeInfo
	err = json.Unmarshal([]byte(response.Content), &result)
	if err != nil {
		fmt.Println("cannot unmarshal changes overview response:", err.Error())
		fmt.Println(response.Content)
		return nil, erro.Wrap(err, "failed to parse changes overview response as JSON")
	}

	return result, nil
}

// GenerateArchitectureReview generates an architecture review for all code changes
func (a *Agent) GenerateArchitectureReview(ctx context.Context, diff string) (string, error) {
	response, err := a.apiCall(ctx, a.pb.BuildArchitectureReviewPrompt(diff), false)
	if err != nil {
		return "", erro.Wrap(err, "failed to call API for architecture review")
	}

	a.log.Debug("architecture review generated",
		"input_tokens", response.PromptTokens,
		"output_tokens", response.CompletionTokens,
		"total_tokens", response.TotalTokens,
	)

	return response.Content, nil
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, fileDiffString, fullFileContent string) (*model.FileReviewResult, error) {
	prompt := a.pb.BuildReviewPrompWithFullFileContent(filename, fileDiffString, fullFileContent)
	response, err := a.apiCall(ctx, prompt, true)
	if err != nil {
		return nil, erro.Wrap(err, "failed to call API for simple review")
	}

	a.log.Debug("simple code review generated",
		"input_tokens", response.PromptTokens,
		"output_tokens", response.CompletionTokens,
		"total_tokens", response.TotalTokens,
		"filename", filename,
	)

	result, err := unmarshal[model.FileReviewResult](response.Content)
	if err != nil {
		fmt.Println("cannot unmarshal review response:", err.Error())
		fmt.Println(response.Content)
		return nil, erro.Wrap(err, "failed to parse simple review response as JSON")
	}

	result.File = filename

	return &result, nil
}

// ReviewCodeWithContext performs a context-aware code review on the given file with rich contextual information
func (a *Agent) ReviewCodeWithContext(ctx context.Context, filename, fileDiffString string, fileContext *astparser.FileContext, mrContext *llmcontext.MRContext) (*model.FileReviewResult, error) {
	prompt := a.pb.BuildReviewPromptWithContext(filename, fileDiffString, fileContext, mrContext)

	err := os.WriteFile("fileContext"+path.Base(filename)+".json", []byte(prompt.UserPrompt), 0644)
	if err != nil {
		return nil, erro.Wrap(err, "failed to write file context to file")
	}
	return nil, nil

	response, err := a.apiCall(ctx, prompt, true)
	if err != nil {
		return nil, erro.Wrap(err, "failed to call API for context-aware review")
	}

	a.log.Debug("context-aware code review generated",
		"input_tokens", response.PromptTokens,
		"output_tokens", response.CompletionTokens,
		"total_tokens", response.TotalTokens,
		"filename", filename,
		"context_symbols", len(fileContext.AffectedSymbols),
	)

	result, err := unmarshal[model.FileReviewResult](response.Content)
	if err != nil {
		fmt.Println("cannot unmarshal context-aware review response:", err.Error())
		fmt.Println(response.Content)
		return nil, erro.Wrap(err, "failed to parse context-aware review response as JSON")
	}

	result.File = filename

	return &result, nil
}

func (a *Agent) apiCall(ctx context.Context, prompt model.Prompt, isJSON bool) (model.APIResponse, error) {
	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
		ResponseType: lang.If(isJSON, "application/json", "text/plain"),
	})
	if err != nil {
		return model.APIResponse{}, erro.Wrap(err, "failed to call API")
	}

	if response.Content == "" {
		return model.APIResponse{}, erro.New("empty response from API")
	}

	return response, nil
}

func unmarshal[T any](response string) (T, error) {
	var result T

	// Clean up response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimPrefix(response, "json")
	response = strings.TrimSuffix(response, "```")

	// Find JSON boundaries
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start == -1 || end == -1 || end <= start {
		return result, erro.New("no valid JSON found in response")
	}

	jsonStr := response[start : end+1]

	// Attempt to fix common JSON issues
	jsonStr = fixCommonJSONIssues(jsonStr)

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// Log the problematic JSON for debugging
		fmt.Printf("Failed to parse JSON: %s", jsonStr)
		return result, erro.Wrap(err, "failed to parse JSON response")
	}

	return result, nil
}

func fixCommonJSONIssues(jsonStr string) string {
	// Fix truncated strings by ensuring proper closure
	if !strings.HasSuffix(strings.TrimSpace(jsonStr), "}") {
		// Try to recover by finding the last complete field
		lastComma := strings.LastIndex(jsonStr, ",")
		lastQuote := strings.LastIndex(jsonStr, "\"")
		if lastComma > lastQuote {
			// Remove incomplete field and close JSON
			jsonStr = jsonStr[:lastComma] + "\n    }\n  ]\n}"
		}
	}
	return jsonStr
}
