package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Agent implements the AIAgent interface using OpenAI API
type Agent struct {
	config     config.AgentConfig
	logger     logze.Logger
	httpClient *http.Client
	apiURL     string
}

// OpenAI API request/response structures
type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []choice  `json:"choices"`
	Usage   usage     `json:"usage"`
	Error   *apiError `json:"error,omitempty"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewAgent creates a new OpenAI agent
func NewAgent(ctx context.Context, cfg config.AgentConfig) (*Agent, error) {
	if cfg.APIKey == "" {
		return nil, errm.New("OpenAI API key is required")
	}

	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini" // Default to GPT-4o mini
	}

	// Set API URL - support for custom endpoints (Azure OpenAI, local models, etc.)
	apiURL := "https://api.openai.com/v1/chat/completions"
	if cfg.BaseURL != "" {
		apiURL = strings.TrimSuffix(cfg.BaseURL, "/") + "/v1/chat/completions"
		if strings.Contains(cfg.BaseURL, "azure") {
			// Azure OpenAI has different URL structure
			apiURL = strings.TrimSuffix(cfg.BaseURL, "/") + "/openai/deployments/" + cfg.Model + "/chat/completions?api-version=2024-02-15-preview"
		}
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Add proxy support if specified
	if cfg.ProxyURL != "" {
		// Note: In a real implementation, you'd parse the proxy URL and set up the transport
		// For now, we'll just log that proxy is configured
	}

	agent := &Agent{
		config:     cfg,
		logger:     logze.Default(),
		httpClient: httpClient,
		apiURL:     apiURL,
	}

	// Test connection
	if err := agent.testConnection(ctx); err != nil {
		return nil, errm.Wrap(err, "failed to connect to OpenAI API")
	}

	return agent, nil
}

// GenerateDescription generates a description for code changes
func (a *Agent) GenerateDescription(ctx context.Context, diff string) (string, error) {
	prompt := a.buildDescriptionPrompt(diff)

	response, err := a.callAPI(ctx, prompt)
	if err != nil {
		return "", errm.Wrap(err, "failed to generate description")
	}

	return response, nil
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, diff string) (string, error) {
	prompt := a.buildReviewPrompt(filename, diff)

	response, err := a.callAPI(ctx, prompt)
	if err != nil {
		return "", errm.Wrap(err, "failed to review code")
	}

	return response, nil
}

// SummarizeChanges summarizes the changes in a set of files
func (a *Agent) SummarizeChanges(ctx context.Context, changes []*models.FileDiff) (string, error) {
	prompt := a.buildSummaryPrompt(changes)

	response, err := a.callAPI(ctx, prompt)
	if err != nil {
		return "", errm.Wrap(err, "failed to summarize changes")
	}

	return response, nil
}

// callAPI makes a request to the OpenAI API
func (a *Agent) callAPI(ctx context.Context, prompt string) (string, error) {
	// Prepare request
	reqBody := chatCompletionRequest{
		Model: a.config.Model,
		Messages: []message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: float64(a.config.Temperature),
		MaxTokens:   a.config.MaxTokens,
		Stream:      false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", errm.Wrap(err, "failed to marshal request")
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", a.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", errm.Wrap(err, "failed to create request")
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Handle different authentication methods
	if strings.Contains(a.apiURL, "azure") {
		// Azure OpenAI uses api-key header
		req.Header.Set("api-key", a.config.APIKey)
	} else {
		// Standard OpenAI API uses Authorization Bearer
		req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	}

	// Make request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", errm.Wrap(err, "failed to make API request")
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errm.Wrap(err, "failed to read response")
	}

	// Parse response
	var chatResp chatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", errm.Wrap(err, "failed to parse response")
	}

	// Check for API errors
	if chatResp.Error != nil {
		return "", errm.New(fmt.Sprintf("OpenAI API error: %s", chatResp.Error.Message))
	}

	// Handle non-200 status codes
	if resp.StatusCode != http.StatusOK {
		return "", errm.New(fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(body)))
	}

	// Extract response
	if len(chatResp.Choices) == 0 {
		return "", errm.New("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)

	// Log token usage for monitoring
	a.logger.Debug("OpenAI API call completed",
		"model", chatResp.Model,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"total_tokens", chatResp.Usage.TotalTokens,
	)

	return content, nil
}

// testConnection tests the connection to OpenAI API
func (a *Agent) testConnection(ctx context.Context) error {
	// Simple test prompt
	testPrompt := "Respond with 'OK' if you can understand this message."

	response, err := a.callAPI(ctx, testPrompt)
	if err != nil {
		return errm.Wrap(err, "connection test failed")
	}

	a.logger.Info("OpenAI connection test successful", "model", a.config.Model, "response", response)
	return nil
}

// buildDescriptionPrompt creates a prompt for generating PR/MR descriptions
func (a *Agent) buildDescriptionPrompt(diff string) string {
	return fmt.Sprintf(`Analyze the following code changes and generate a clear, concise description of what was changed and why.

Guidelines:
- Focus on the main purpose and impact of the changes
- Mention key files or components affected
- Keep it professional and informative
- Use bullet points for multiple changes
- Limit to 3-4 sentences or bullet points
- Write in English

Code changes:
%s

Generate a description:`, diff)
}

// buildReviewPrompt creates a prompt for code review
func (a *Agent) buildReviewPrompt(filename, diff string) string {
	return fmt.Sprintf(`Review the following code changes for file "%s" and provide feedback on:

1. Code quality and best practices
2. Potential bugs or issues
3. Performance considerations
4. Security concerns
5. Maintainability and readability

Guidelines:
- Be constructive and specific
- Suggest improvements where applicable
- If the code looks good, just say "OK" or "LGTM"
- Focus on significant issues, not minor style preferences
- Provide examples when suggesting changes
- Write in English

File: %s

Code changes:
%s

Review:`, filename, filename, diff)
}

// buildSummaryPrompt creates a prompt for summarizing multiple changes
func (a *Agent) buildSummaryPrompt(changes []*models.FileDiff) string {
	var changesText strings.Builder

	for i, change := range changes {
		if i > 0 {
			changesText.WriteString("\n\n---\n\n")
		}
		changesText.WriteString(fmt.Sprintf("File: %s\n", change.NewPath))
		if change.IsNew {
			changesText.WriteString("Status: New file\n")
		} else if change.IsDeleted {
			changesText.WriteString("Status: Deleted file\n")
		} else if change.IsRenamed {
			changesText.WriteString(fmt.Sprintf("Status: Renamed from %s\n", change.OldPath))
		} else {
			changesText.WriteString("Status: Modified\n")
		}
		changesText.WriteString("Changes:\n")
		changesText.WriteString(change.Diff)
	}

	return fmt.Sprintf(`Summarize the following code changes across multiple files:

Guidelines:
- Provide a high-level overview of the changes
- Group related changes together
- Mention the overall impact or purpose
- Keep it concise but informative
- Write in English

Changes:
%s

Summary:`, changesText.String())
}
