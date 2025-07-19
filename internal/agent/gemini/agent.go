package gemini

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/lang"
	"google.golang.org/genai"
)

const (
	defaultModel = "gemini-2.5-flash"
)

var _ interfaces.AgentAPI = (*Agent)(nil)

// Agent implements the AIAgent interface for Google Gemini
type Agent struct {
	client *genai.Client
	config model.ModelConfig
}

// NewAgent creates a new Gemini agent
func New(ctx context.Context, cfg model.ModelConfig) (*Agent, error) {
	if cfg.APIKey == "" {
		return nil, erro.New("Gemini API key is required")
	}
	cfg.Model = lang.Check(cfg.Model, defaultModel)

	transport := &http.Transport{}
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, erro.Wrap(err, "failed to parse proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{
			Transport: transport,
		},
	})
	if err != nil {
		return nil, erro.Wrap(err, "failed to create Gemini client")
	}

	agent := &Agent{
		client: client,
		config: cfg,
	}

	if cfg.IsTest {
		if err := agent.testConnection(ctx); err != nil {
			return nil, erro.Wrap(err, "failed to connect to Gemini API")
		}
	}

	return agent, nil
}

// generate calls the Gemini API to generate content
func (a *Agent) CallAPI(ctx context.Context, req model.APIRequest) (model.APIResponse, error) {
	config := &genai.GenerateContentConfig{
		ResponseMIMEType:  lang.Check(req.ResponseType, "text/plain"),
		Temperature:       &req.Temperature,
		MaxOutputTokens:   int32(req.MaxTokens),
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: req.SystemPrompt}}},
	}

	result, err := a.client.Models.GenerateContent(ctx,
		a.config.Model,
		[]*genai.Content{{Parts: []*genai.Part{{Text: req.Prompt}}}},
		config,
	)
	if err != nil {
		return model.APIResponse{}, a.handleAPIError(err)
	}

	var content string
	if len(result.Candidates) > 0 {
		candidate := result.Candidates[0]
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			content = candidate.Content.Parts[0].Text
		}
	}

	out := model.APIResponse{
		CreateTime:       result.CreateTime,
		Content:          content,
		PromptTokens:     int(result.UsageMetadata.PromptTokenCount),
		CompletionTokens: int(result.UsageMetadata.CandidatesTokenCount),
		TotalTokens:      int(result.UsageMetadata.TotalTokenCount),
	}

	return out, nil
}

// handleAPIError handles various API errors and returns appropriate error types
func (a *Agent) handleAPIError(err error) error {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "location is not supported"):
		return erro.New("region not supported by Gemini API")
	case strings.Contains(errStr, "429"):
		return erro.New("rate limit exceeded")
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "403"):
		return erro.New("authentication failed")
	case strings.Contains(errStr, "400"):
		return erro.New("bad request to Gemini API")
	case strings.Contains(errStr, "503"):
		return erro.New("Gemini API service unavailable")
	case strings.Contains(errStr, "500") || strings.Contains(errStr, "502"):
		return erro.New("Gemini API server error")
	default:
		return erro.Wrap(err, "Gemini API error")
	}
}

func (a *Agent) testConnection(ctx context.Context) error {
	_, err := a.CallAPI(ctx, model.APIRequest{
		Prompt:      "Respond with 'OK' if you can understand this message.",
		MaxTokens:   10,
		Temperature: 0.5,
	})
	return err
}
