package openai

import (
	"context"
	"strings"
	"time"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
)

const (
	defaultModel = "gpt-4o-mini"
	defaultURL   = "https://api.openai.com/v1"
)

var _ interfaces.AgentAPI = (*Agent)(nil)

// Agent implements the AIAgent interface using OpenAI API
type Agent struct {
	cli *cliex.HTTP
	cfg model.ModelConfig
}

// NewAgent creates a new OpenAI agent
func New(ctx context.Context, cli *cliex.HTTP, config model.ModelConfig) (*Agent, error) {
	if config.APIKey == "" {
		return nil, errm.New("OpenAI API key is required")
	}
	config.Model = lang.Check(config.Model, defaultModel)
	config.URL = lang.Check(config.URL, defaultURL)

	cli.C().SetAuthToken(config.APIKey)

	agent := &Agent{
		cli: cli,
		cfg: config,
	}

	// Test connection if needed (may take tokens)
	if config.IsTest {
		if err := agent.testConnection(ctx); err != nil {
			return nil, errm.Wrap(err, "failed to connect to OpenAI API")
		}
	}

	return agent, nil
}

// callAPI makes a request to the OpenAI API
func (a *Agent) CallAPI(ctx context.Context, req model.APIRequest) (model.APIResponse, error) {
	// Prepare request
	reqBody := chatCompletionRequest{
		Model: a.cfg.Model,
		Messages: []message{
			{
				Role:    "system",
				Content: req.SystemPrompt,
			},
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	var respBody chatCompletionResponse
	requestURL := lang.Check(req.URL, a.cfg.URL)
	_, err := a.cli.Post(ctx, requestURL, reqBody, &respBody)
	if err != nil {
		return model.APIResponse{}, errm.Wrap(err, "failed to make API request")
	}

	// Check for API errors
	if respBody.Error != nil {
		return model.APIResponse{}, errm.Errorf("OpenAI API error: %s", respBody.Error.Message)
	}

	// Extract response
	var content string
	if len(respBody.Choices) > 0 {
		content = strings.TrimSpace(respBody.Choices[0].Message.Content)
	}

	out := model.APIResponse{
		CreateTime:       time.Unix(respBody.Created, 0),
		Content:          content,
		PromptTokens:     respBody.Usage.PromptTokens,
		CompletionTokens: respBody.Usage.CompletionTokens,
		TotalTokens:      respBody.Usage.TotalTokens,
	}

	return out, nil
}

// testConnection tests the connection to OpenAI API
func (a *Agent) testConnection(ctx context.Context) error {
	// Simple test prompt
	testPrompt := "Respond with 'OK' if you can understand this message."

	_, err := a.CallAPI(ctx, model.APIRequest{
		Prompt:      testPrompt,
		MaxTokens:   10,
		Temperature: 0.5,
		URL:         a.cfg.URL,
	})
	if err != nil {
		return errm.Wrap(err, "connection test failed")
	}
	return nil

}
