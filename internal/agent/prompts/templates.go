package prompts

import (
	"fmt"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
)

var _ interfaces.PromptBuilder = &Builder{}

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
	userPrompt := fmt.Sprintf(descriptionUserPromptTemplate, diff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildReviewPrompt creates a prompt for code review
func (tb *Builder) BuildReviewPrompt(filename, diff string) model.Prompt {
	systemPrompt := fmt.Sprintf(reviewSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(reviewUserPromptTemplate, filename, filename, diff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildSummaryPrompt creates a prompt for summarizing multiple file changes
func (tb *Builder) BuildSummaryPrompt(changes []*model.FileDiff) model.Prompt {
	systemPrompt := fmt.Sprintf(summarySystemPromptTemplate, tb.language.Instructions)

	// Build the changes description
	var changesText strings.Builder
	for i, change := range changes {
		if i > 0 {
			changesText.WriteString("\n\n---\n\n")
		}

		// File status
		status := "Modified"
		if change.IsNew {
			status = "New file"
		} else if change.IsDeleted {
			status = "Deleted file"
		} else if change.IsRenamed {
			status = fmt.Sprintf("Renamed from %s", change.OldPath)
		}

		changesText.WriteString(fmt.Sprintf("**File:** `%s`\n", change.NewPath))
		changesText.WriteString(fmt.Sprintf("**Status:** %s\n", status))
		changesText.WriteString("**Changes:**\n")
		changesText.WriteString("```diff\n")
		changesText.WriteString(change.Diff)
		changesText.WriteString("\n```")
	}

	userPrompt := fmt.Sprintf(summaryUserPromptTemplate, len(changes), tb.countTotalLines(changes), len(changes), changesText.String())

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildCommentReplyPrompt creates a prompt for generating a reply to a comment
func (tb *Builder) BuildCommentReplyPrompt(originalComment, replyContext string) model.Prompt {
	systemPrompt := fmt.Sprintf(commentReplySystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(commentReplyUserPromptTemplate, originalComment, replyContext)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// countTotalLines counts the total number of lines across all diffs
func (tb *Builder) countTotalLines(changes []*model.FileDiff) int {
	total := 0
	for _, change := range changes {
		total += len(strings.Split(change.Diff, "\n"))
	}
	return total
}
