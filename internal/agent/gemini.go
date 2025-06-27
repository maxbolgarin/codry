package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/maxbolgarin/errm"
	"google.golang.org/genai"
)

var (
	ErrBadRegion     = errors.New("bad region")
	ErrLimitExceeded = errors.New("limit exceeded")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrBadRequest    = errors.New("bad request")
	ErrOverloaded    = errors.New("overloaded")
	errmerverError   = errors.New("server error")
)

const (
	Gemini25Pro   string = "gemini-2.5-pro-preview-06-05"
	Gemini25Flash string = "gemini-2.5-flash-preview-05-20"
)

type Config struct {
	APIKey    string `yaml:"api_key" env:"GEMINI_API_KEY"`
	ProxyURL  string `yaml:"proxy_url" env:"GEMINI_PROXY_URL"`
	ModelName string `yaml:"model_name" env:"GEMINI_MODEL_NAME"`
}

type Gemini struct {
	client *genai.Client
	cfg    Config
}

func NewGemini(ctx context.Context, cfg Config) (*Gemini, error) {
	if cfg.APIKey == "" {
		return nil, errm.New("API key is required")
	}

	transport := &http.Transport{}
	if cfg.ProxyURL != "" {
		fixedURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, errm.Wrap(err, "failed to parse proxy URL")
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(fixedURL),
		}
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{
			Transport: transport,
		},
	})
	if err != nil {
		return nil, errm.Wrap(err, "failed to create Gemini client")
	}
	if cfg.ModelName == "" {
		cfg.ModelName = Gemini25Flash
	}

	return &Gemini{
		client: client,
		cfg:    cfg,
	}, nil
}

func (g *Gemini) GenerateDescription(ctx context.Context, fullDiff string) (string, error) {
	prompt := fmt.Sprintf(PromptGenerateDescription, fullDiff)
	return g.generate(ctx, prompt)
}

const (
	ReviewPositiveIndicator = "выглядят хорошо"
)

func (g *Gemini) ReviewCode(ctx context.Context, filePath, diff string) (string, error) {
	prompt := fmt.Sprintf(PromptReviewCode, filePath, diff)
	result, err := g.generate(ctx, prompt)
	if err != nil {
		return "", err
	}
	if strings.Contains(result, ReviewPositiveIndicator) {
		return "", nil
	}
	return result, nil
}

func (g *Gemini) generate(ctx context.Context, prompt string) (string, error) {
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "text/plain",
	}

	result, err := g.client.Models.GenerateContent(ctx,
		g.cfg.ModelName,
		[]*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}},
		cfg,
	)
	switch {
	case err != nil && strings.Contains(err.Error(), "location is not supported"):
		return "", errm.Wrap(ErrBadRegion, err.Error())
	case err != nil && strings.Contains(err.Error(), "429"):
		return "", errm.Wrap(ErrLimitExceeded, err.Error())
	case err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403")):
		return "", errm.Wrap(ErrUnauthorized, err.Error())
	case err != nil && strings.Contains(err.Error(), "400"):
		return "", errm.Wrap(ErrBadRequest, err.Error())
	case err != nil && strings.Contains(err.Error(), "503"):
		return "", errm.Wrap(ErrOverloaded, err.Error())
	case err != nil && (strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), "NO_ERROR") || strings.Contains(err.Error(), "502")):
		return "", errm.Wrap(errmerverError, err.Error())
	case err != nil:
		return "", errm.Wrap(err, "generate content")
	case len(result.Candidates) == 0:
		return "", errm.Wrap(ErrBadRequest, "no candidates")
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", errm.Wrap(ErrBadRequest, "invalid response structure")
	}
	return candidate.Content.Parts[0].Text, nil
}
