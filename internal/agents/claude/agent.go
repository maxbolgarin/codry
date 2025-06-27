package claude

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

// Agent implements the AIAgent interface using Anthropic's Claude API
type Agent struct {
	config     config.AgentConfig
	logger     logze.Logger
	httpClient *http.Client
	apiURL     string
}

// Claude API request/response structures
type messagesRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
	Messages    []message `json:"messages"`
	System      string    `json:"system,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []content `json:"content"`
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"`
	StopSequence string    `json:"stop_sequence,omitempty"`
	Usage        usage     `json:"usage"`
	Error        *apiError `json:"error,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// NewAgent creates a new Claude agent
func NewAgent(ctx context.Context, cfg config.AgentConfig) (*Agent, error) {
	if cfg.APIKey == "" {
		return nil, errm.New("Claude API key is required")
	}

	if cfg.Model == "" {
		cfg.Model = "claude-3-5-haiku-20241022" // Default to Claude 3.5 Haiku (cost-effective)
	}

	// Set API URL - Claude uses a fixed endpoint
	apiURL := "https://api.anthropic.com/v1/messages"
	if cfg.BaseURL != "" {
		apiURL = strings.TrimSuffix(cfg.BaseURL, "/") + "/v1/messages"
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 120 * time.Second, // Claude can be slower than OpenAI
	}

	agent := &Agent{
		config:     cfg,
		logger:     logze.Default(),
		httpClient: httpClient,
		apiURL:     apiURL,
	}

	// Test connection
	if err := agent.testConnection(ctx); err != nil {
		return nil, errm.Wrap(err, "failed to connect to Claude API")
	}

	return agent, nil
}

// GenerateDescription generates a description for code changes
func (a *Agent) GenerateDescription(ctx context.Context, diff string) (string, error) {
	prompt := a.buildDescriptionPrompt(diff)

	response, err := a.callAPI(ctx, prompt, "You are an expert software developer. Generate clear, concise descriptions of code changes.")
	if err != nil {
		return "", errm.Wrap(err, "failed to generate description")
	}

	return response, nil
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, diff string) (string, error) {
	prompt := a.buildReviewPrompt(filename, diff)

	response, err := a.callAPI(ctx, prompt, "You are a senior software engineer conducting a thorough code review. Focus on code quality, potential issues, and best practices.")
	if err != nil {
		return "", errm.Wrap(err, "failed to review code")
	}

	return response, nil
}

// SummarizeChanges summarizes the changes in a set of files
func (a *Agent) SummarizeChanges(ctx context.Context, changes []*models.FileDiff) (string, error) {
	prompt := a.buildSummaryPrompt(changes)

	response, err := a.callAPI(ctx, prompt, "You are a technical writer summarizing software changes. Provide clear, high-level overviews.")
	if err != nil {
		return "", errm.Wrap(err, "failed to summarize changes")
	}

	return response, nil
}

// callAPI makes a request to the Claude API
func (a *Agent) callAPI(ctx context.Context, prompt, systemPrompt string) (string, error) {
	// Prepare request
	reqBody := messagesRequest{
		Model:       a.config.Model,
		MaxTokens:   a.config.MaxTokens,
		Temperature: float64(a.config.Temperature),
		System:      systemPrompt,
		Messages: []message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
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
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

	// Handle non-200 status codes
	if resp.StatusCode != http.StatusOK {
		return "", errm.New(fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(body)))
	}

	// Parse response
	var claudeResp messagesResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", errm.Wrap(err, "failed to parse response")
	}

	// Check for API errors
	if claudeResp.Error != nil {
		return "", errm.New(fmt.Sprintf("Claude API error: %s", claudeResp.Error.Message))
	}

	// Extract response
	if len(claudeResp.Content) == 0 {
		return "", errm.New("no content in response")
	}

	var responseText strings.Builder
	for _, c := range claudeResp.Content {
		if c.Type == "text" {
			responseText.WriteString(c.Text)
		}
	}

	content := strings.TrimSpace(responseText.String())

	// Log token usage for monitoring
	a.logger.Debug("Claude API call completed",
		"model", claudeResp.Model,
		"input_tokens", claudeResp.Usage.InputTokens,
		"output_tokens", claudeResp.Usage.OutputTokens,
		"total_tokens", claudeResp.Usage.InputTokens+claudeResp.Usage.OutputTokens,
		"stop_reason", claudeResp.StopReason,
	)

	return content, nil
}

// testConnection tests the connection to Claude API
func (a *Agent) testConnection(ctx context.Context) error {
	// Simple test prompt
	testPrompt := "Respond with 'OK' if you can understand this message."

	response, err := a.callAPI(ctx, testPrompt, "You are a helpful assistant.")
	if err != nil {
		return errm.Wrap(err, "connection test failed")
	}

	a.logger.Info("Claude connection test successful", "model", a.config.Model, "response", response)
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
- Be thorough but concise
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
- Focus on the most important changes
- Write in English

Changes:
%s

Summary:`, changesText.String())
}
