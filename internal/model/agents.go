package model

import (
	"context"
	"time"
)

// AIAgent defines the interface for AI code review agents
type AIAgent interface {
	GenerateDescription(ctx context.Context, fullDiff string) (string, error)
	ReviewCode(ctx context.Context, filePath, diff string) (string, error)
	SummarizeChanges(ctx context.Context, changes []*FileDiff) (string, error)
	GenerateCommentReply(ctx context.Context, originalComment, replyContext string) (string, error)
}

// AgentAPI defines the interface for calling LLM AI models
type AgentAPI interface {
	CallAPI(ctx context.Context, req APIRequest) (APIResponse, error)
}

// PromptBuilder defines the interface for building prompts for AI agents
type PromptBuilder interface {
	BuildDescriptionPrompt(diff string) Prompt
	BuildReviewPrompt(filename, diff string) Prompt
	BuildSummaryPrompt(changes []*FileDiff) Prompt
}

// ModelConfig represents model-specific configuration
type ModelConfig struct {
	APIKey   string
	Model    string
	URL      string
	ProxyURL string
	IsTest   bool
}

// APIRequest represents a request to an LLM API
type APIRequest struct {
	Prompt       string
	SystemPrompt string
	MaxTokens    int
	Temperature  float32
	URL          string
}

// APIResponse represents a response from an LLM API
type APIResponse struct {
	CreateTime       time.Time
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Prompt represents a structured prompt for LLM
type Prompt struct {
	SystemPrompt string
	UserPrompt   string
	Language     Language
}

type Language string

const (
	LanguageEnglish    Language = "en"
	LanguageRussian    Language = "ru"
	LanguageSpanish    Language = "es"
	LanguageFrench     Language = "fr"
	LanguageItalian    Language = "it"
	LanguageGerman     Language = "de"
	LanguagePortuguese Language = "pt"
	LanguageJapanese   Language = "ja"
	LanguageKorean     Language = "ko"
	LanguageChinese    Language = "zh"
)
