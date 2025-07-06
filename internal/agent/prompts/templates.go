package prompts

import (
	"fmt"

	"github.com/maxbolgarin/codry/internal/model"
)

// Builder provides methods to build prompts with language support
type Builder struct {
	language LanguageConfig
}

// NewBuilder creates a new template builder with language configuration
func NewBuilder(language model.Language) *Builder {
	lang, exists := DefaultLanguages[language]
	if !exists {
		lang = DefaultLanguages[model.LanguageEnglish] // Default to English
	}
	return &Builder{
		language: lang,
	}
}

// BuildDescriptionPrompt creates a prompt for generating PR/MR descriptions
func (tb *Builder) BuildDescriptionPrompt(diff string) model.Prompt {
	systemPrompt := fmt.Sprintf(descriptionSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(descriptionUserPromptTemplate,
		tb.language.DescriptionHeaders.Title,
		tb.language.DescriptionHeaders.Title,
		tb.language.DescriptionHeaders.NewFeaturesHeader,
		tb.language.DescriptionHeaders.BugFixesHeader,
		tb.language.DescriptionHeaders.RefactoringHeader,
		tb.language.DescriptionHeaders.TestingHeader,
		tb.language.DescriptionHeaders.CIAndBuildHeader,
		tb.language.DescriptionHeaders.DocsImprovementHeader,
		tb.language.DescriptionHeaders.RemovalsAndCleanupHeader,
		tb.language.DescriptionHeaders.OtherChangesHeader,
		diff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildEnhancedStructuredReviewPrompt creates a prompt for structured code review with full file content and clean diff
func (tb *Builder) BuildReviewPrompt(filename, fullFileContent, cleanDiff string) model.Prompt {
	systemPrompt := fmt.Sprintf(reviewSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(structuredReviewUserPromptTemplate, filename, fullFileContent, cleanDiff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}
