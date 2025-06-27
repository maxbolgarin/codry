package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"google.golang.org/genai"
)

const (
	Gemini25Pro   = "gemini-2.5-pro-preview-06-05"
	Gemini25Flash = "gemini-2.5-flash-preview-05-20"
)

// Agent implements the AIAgent interface for Google Gemini
type Agent struct {
	client *genai.Client
	config config.AgentConfig
}

// NewAgent creates a new Gemini agent
func NewAgent(ctx context.Context, cfg config.AgentConfig) (*Agent, error) {
	if cfg.APIKey == "" {
		return nil, errm.New("Gemini API key is required")
	}

	transport := &http.Transport{}
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, errm.Wrap(err, "failed to parse proxy URL")
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
		return nil, errm.Wrap(err, "failed to create Gemini client")
	}

	// Set default model if not specified
	if cfg.Model == "" {
		cfg.Model = Gemini25Flash
	}

	return &Agent{
		client: client,
		config: cfg,
	}, nil
}

// GenerateDescription generates a description for the given diff
func (a *Agent) GenerateDescription(ctx context.Context, fullDiff string) (string, error) {
	prompt := fmt.Sprintf(PromptGenerateDescription, fullDiff)
	return a.generate(ctx, prompt)
}

// ReviewCode reviews code changes and returns feedback
func (a *Agent) ReviewCode(ctx context.Context, filePath, diff string) (string, error) {
	prompt := fmt.Sprintf(PromptReviewCode, filePath, diff)
	result, err := a.generate(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Check if the response indicates no issues
	if strings.Contains(result, "выглядят хорошо") || strings.HasPrefix(strings.TrimSpace(result), "OK") {
		return "", nil
	}

	return result, nil
}

// SummarizeChanges provides a summary of multiple file changes
func (a *Agent) SummarizeChanges(ctx context.Context, changes []*models.FileDiff) (string, error) {
	if len(changes) == 0 {
		return "Нет изменений для анализа", nil
	}

	var summary strings.Builder
	summary.WriteString("Краткая сводка изменений:\n\n")

	fileCount := len(changes)
	addedFiles := 0
	modifiedFiles := 0
	deletedFiles := 0

	for _, change := range changes {
		if change.IsNew {
			addedFiles++
		} else if change.IsDeleted {
			deletedFiles++
		} else {
			modifiedFiles++
		}
	}

	summary.WriteString(fmt.Sprintf("📊 **Статистика**: %d файлов изменено\n", fileCount))
	if addedFiles > 0 {
		summary.WriteString(fmt.Sprintf("➕ Добавлено файлов: %d\n", addedFiles))
	}
	if modifiedFiles > 0 {
		summary.WriteString(fmt.Sprintf("📝 Изменено файлов: %d\n", modifiedFiles))
	}
	if deletedFiles > 0 {
		summary.WriteString(fmt.Sprintf("❌ Удалено файлов: %d\n", deletedFiles))
	}

	// List key files
	if len(changes) <= 10 {
		summary.WriteString("\n**Затронутые файлы**:\n")
		for _, change := range changes {
			status := "📝"
			if change.IsNew {
				status = "➕"
			} else if change.IsDeleted {
				status = "❌"
			}
			summary.WriteString(fmt.Sprintf("- %s `%s`\n", status, change.NewPath))
		}
	}

	return summary.String(), nil
}

// generate calls the Gemini API to generate content
func (a *Agent) generate(ctx context.Context, prompt string) (string, error) {
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "text/plain",
		Temperature:      &a.config.Temperature,
	}

	if a.config.MaxTokens > 0 {
		maxTokens := int32(a.config.MaxTokens)
		config.MaxOutputTokens = maxTokens
	}

	result, err := a.client.Models.GenerateContent(ctx,
		a.config.Model,
		[]*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}},
		config,
	)

	if err != nil {
		return "", a.handleAPIError(err)
	}

	if len(result.Candidates) == 0 {
		return "", errm.New("no candidates returned from Gemini API")
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", errm.New("invalid response structure from Gemini API")
	}

	return candidate.Content.Parts[0].Text, nil
}

// handleAPIError handles various API errors and returns appropriate error types
func (a *Agent) handleAPIError(err error) error {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "location is not supported"):
		return errm.New("region not supported by Gemini API")
	case strings.Contains(errStr, "429"):
		return errm.New("rate limit exceeded")
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "403"):
		return errm.New("authentication failed")
	case strings.Contains(errStr, "400"):
		return errm.New("bad request to Gemini API")
	case strings.Contains(errStr, "503"):
		return errm.New("Gemini API service unavailable")
	case strings.Contains(errStr, "500") || strings.Contains(errStr, "502"):
		return errm.New("Gemini API server error")
	default:
		return errm.Wrap(err, "Gemini API error")
	}
}
