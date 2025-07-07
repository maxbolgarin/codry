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
	"github.com/maxbolgarin/lang"
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
	return a.apiCall(ctx, a.pb.BuildDescriptionPrompt(diff), false)
}

// GenerateChangesOverview generates an overview of code changes§
func (a *Agent) GenerateChangesOverview(ctx context.Context, diff string) ([]model.FileChange, error) {
	prompt := a.pb.BuildChangesOverviewPrompt(diff)
	response, err := a.apiCall(ctx, prompt, true)
	if err != nil {
		return nil, errm.Wrap(err, "failed to call API for changes overview")
	}

	var result []model.FileChange
	err = json.Unmarshal([]byte(response), &result)
	if err != nil {
		fmt.Println(response)
		return nil, errm.Wrap(err, "failed to parse changes overview response as JSON")
	}

	return result, nil
}

// GenerateArchitectureReview generates an architecture review for all code changes
func (a *Agent) GenerateArchitectureReview(ctx context.Context, diff string) (string, error) {
	return a.apiCall(ctx, a.pb.BuildArchitectureReviewPrompt(diff), false)
}

// parseArchitectureReviewResponse parses the markdown response from architecture review
func (a *Agent) parseArchitectureReviewResponse(response string) *model.ArchitectureReviewResult {
	result := &model.ArchitectureReviewResult{}

	lines := strings.Split(response, "\n")
	var currentSection string
	var currentFinding *model.ArchitectureReviewFinding

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse main header (## Architecture review)
		if strings.HasPrefix(line, "## ") {
			// Extract general overview from the next non-empty lines until we hit a ###
			continue
		}

		// Parse section headers (### Architecture issues, etc.)
		if strings.HasPrefix(line, "### ") {
			currentSection = strings.TrimPrefix(line, "### ")
			currentSection = strings.Trim(currentSection, "*")
			continue
		}

		// Parse bullet points with findings
		if strings.HasPrefix(line, "- **") && strings.Contains(line, "**:") {
			// Save previous finding if exists
			if currentFinding != nil {
				a.addFindingToResult(result, currentSection, *currentFinding)
			}

			// Start new finding
			parts := strings.SplitN(line, "**:", 2)
			if len(parts) == 2 {
				currentFinding = &model.ArchitectureReviewFinding{
					Title:       strings.TrimPrefix(parts[0], "- **"),
					Description: strings.TrimSpace(parts[1]),
				}
			}
			continue
		}

		// Handle continuation of finding description or general overview
		if currentFinding == nil && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			// This might be general overview content
			if result.GeneralOverview == "" {
				result.GeneralOverview = line
			} else {
				result.GeneralOverview += " " + line
			}
		}
	}

	// Add the last finding if exists
	if currentFinding != nil {
		a.addFindingToResult(result, currentSection, *currentFinding)
	}

	return result
}

// addFindingToResult adds a finding to the appropriate section of the result
func (a *Agent) addFindingToResult(result *model.ArchitectureReviewResult, section string, finding model.ArchitectureReviewFinding) {
	switch {
	case strings.Contains(strings.ToLower(section), "architecture"):
		result.ArchitectureIssues = append(result.ArchitectureIssues, finding)
	case strings.Contains(strings.ToLower(section), "performance"):
		result.PerformanceIssues = append(result.PerformanceIssues, finding)
	case strings.Contains(strings.ToLower(section), "security"):
		result.SecurityIssues = append(result.SecurityIssues, finding)
	case strings.Contains(strings.ToLower(section), "documentation"):
		result.DocumentationNeeds = append(result.DocumentationNeeds, finding)
	default:
		// Default to architecture issues if section is unclear
		result.ArchitectureIssues = append(result.ArchitectureIssues, finding)
	}
}

// ReviewCode performs a code review on the given file
func (a *Agent) ReviewCode(ctx context.Context, filename, fullFileContent, cleanDiff string) (*model.FileReviewResult, error) {
	prompt := a.pb.BuildReviewPrompt(filename, fullFileContent, cleanDiff)
	response, err := a.apiCall(ctx, prompt, true)
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

// ReviewCodeWithContext performs enhanced code review using rich context information
func (a *Agent) ReviewCodeWithContext(ctx context.Context, filename string, enhancedCtx *prompts.EnhancedContext) (*model.FileReviewResult, error) {
	prompt := a.pb.BuildEnhancedReviewPrompt(filename, enhancedCtx, enhancedCtx.CleanDiff)
	response, err := a.apiCall(ctx, prompt, true)
	if err != nil {
		return nil, errm.Wrap(err, "failed to call API for enhanced context review")
	}

	result, err := unmarshal[model.FileReviewResult](response)
	if err != nil {
		fmt.Println(response)
		return nil, errm.Wrap(err, "failed to parse enhanced context review response as JSON")
	}

	result.FilePath = filename

	return &result, nil
}

func (a *Agent) apiCall(ctx context.Context, prompt model.Prompt, isJSON bool) (string, error) {
	response, err := a.api.CallAPI(ctx, model.APIRequest{
		Prompt:       prompt.UserPrompt,
		SystemPrompt: prompt.SystemPrompt,
		MaxTokens:    a.cfg.MaxTokens,
		Temperature:  a.cfg.Temperature,
		ResponseType: lang.If(isJSON, "application/json", "text/plain"),
	})
	if err != nil {
		return "", errm.Wrap(err, "failed to call API")
	}

	if response.Content == "" {
		return "", errm.New("empty response from API")
	}

	return response.Content, nil
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
		return result, errm.New("no valid JSON found in response")
	}

	jsonStr := response[start : end+1]

	// Attempt to fix common JSON issues
	jsonStr = fixCommonJSONIssues(jsonStr)

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// Log the problematic JSON for debugging
		fmt.Printf("Failed to parse JSON: %s", jsonStr)
		return result, errm.Wrap(err, "failed to parse JSON response")
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
