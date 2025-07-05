package claude

import (
	"context"
	"strings"
	"time"

	"github.com/maxbolgarin/cliex"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib/lang"
)

const (
	defaultModel   = "claude-3-5-haiku-20241022"
	defaultBaseURL = "https://api.anthropic.com"
)

var _ interfaces.AgentAPI = (*Agent)(nil)

// Agent implements the AIAgent interface using Anthropic's Claude API
type Agent struct {
	cfg model.ModelConfig
	cli *cliex.HTTP
}

// NewAgent creates a new Claude agent
func New(ctx context.Context, cli *cliex.HTTP, cfg model.ModelConfig) (*Agent, error) {
	if cfg.APIKey == "" {
		return nil, errm.New("Claude API key is required")
	}
	cfg.Model = lang.Check(cfg.Model, defaultModel)
	cfg.URL = lang.Check(cfg.URL, defaultBaseURL)

	cli.C().SetHeader("x-api-key", cfg.APIKey)

	agent := &Agent{
		cfg: cfg,
		cli: cli,
	}

	// Test connection
	if cfg.IsTest {
		if err := agent.testConnection(ctx); err != nil {
			return nil, errm.Wrap(err, "failed to connect to Claude API")
		}
	}

	return agent, nil
}

// CallAPI makes a request to the Claude API
func (a *Agent) CallAPI(ctx context.Context, req model.APIRequest) (model.APIResponse, error) {
	// Prepare request
	reqBody := messagesRequest{
		Model:       a.cfg.Model,
		System:      req.SystemPrompt,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages: []message{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
	}

	var respBody messagesResponse
	_, err := a.cli.Post(ctx, a.cfg.URL, reqBody, &respBody)
	if err != nil {
		return model.APIResponse{}, errm.Wrap(err, "failed to make API request")
	}

	// Check for API errors
	if respBody.Error != nil {
		return model.APIResponse{}, errm.Errorf("Claude API error: %s", respBody.Error.Message)
	}

	// Extract response
	if len(respBody.Content) == 0 {
		return model.APIResponse{}, errm.New("no content in response")
	}

	var responseText strings.Builder
	for _, c := range respBody.Content {
		if c.Type == "text" {
			responseText.WriteString(c.Text)
		}
	}

	content := strings.TrimSpace(responseText.String())
	out := model.APIResponse{
		CreateTime:       time.Now(),
		Content:          content,
		PromptTokens:     respBody.Usage.InputTokens,
		CompletionTokens: respBody.Usage.OutputTokens,
		TotalTokens:      respBody.Usage.InputTokens + respBody.Usage.OutputTokens,
	}

	return out, nil
}

// testConnection tests the connection to Claude API
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
