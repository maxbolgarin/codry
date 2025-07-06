package model

import (
	"time"
)

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
	ResponseType string
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
