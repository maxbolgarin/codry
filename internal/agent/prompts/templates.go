package prompts

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
)

// EnhancedContext contains rich context information for code analysis (local copy to avoid circular imports)
type EnhancedContext struct {
	FilePath           string
	FileContent        string
	CleanDiff          string
	ImportedPackages   []string
	RelatedFiles       []RelatedFile
	FunctionSignatures []FunctionSignature
	TypeDefinitions    []TypeDefinition
	UsagePatterns      []UsagePattern
	SecurityContext    SecurityContext
	SemanticChanges    []SemanticChange
}

// RelatedFile represents a file that has relationships with the target file
type RelatedFile struct {
	Path         string
	Relationship string
	Snippet      string
}

// FunctionSignature represents a function definition
type FunctionSignature struct {
	Name       string
	Parameters []string
	Returns    []string
	IsExported bool
	LineNumber int
}

// TypeDefinition represents struct, interface, or type definitions
type TypeDefinition struct {
	Name       string
	Type       string
	Fields     []string
	Methods    []string
	IsExported bool
	LineNumber int
}

// UsagePattern represents how certain patterns are used in the codebase
type UsagePattern struct {
	Pattern      string
	Description  string
	Examples     []string
	BestPractice string
}

// SecurityContext provides security-related context
type SecurityContext struct {
	HasAuthenticationLogic  bool
	HasInputValidation      bool
	HandlesUserInput        bool
	AccessesDatabase        bool
	HandlesFileOperations   bool
	NetworkOperations       bool
	CryptographicOperations bool
}

// SemanticChange represents a high-level change with business impact
type SemanticChange struct {
	Type        string
	Impact      string
	Description string
	Lines       []int
	Context     string
}

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

// BuildChangesOverviewPrompt creates a prompt for generating an overview of code changes
func (tb *Builder) BuildChangesOverviewPrompt(diff string) model.Prompt {
	systemPrompt := fmt.Sprintf(changesOverviewSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(changesOverviewUserPromptTemplate, diff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildArchitectureReviewPrompt creates a prompt for architecture review
func (tb *Builder) BuildArchitectureReviewPrompt(diff string) model.Prompt {
	systemPrompt := fmt.Sprintf(architectureReviewSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(architectureReviewUserPromptTemplate,
		tb.language.ArchitectureReviewHeaders.GeneralHeader,
		tb.language.ArchitectureReviewHeaders.ArchitectureIssuesHeader,
		tb.language.ArchitectureReviewHeaders.PerformanceIssuesHeader,
		tb.language.ArchitectureReviewHeaders.SecurityIssuesHeader,
		tb.language.ArchitectureReviewHeaders.DocsImprovementHeader,
		diff)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildReviewPrompt creates a prompt for structured code review with full file content and clean diff (legacy method)
func (tb *Builder) BuildReviewPrompt(filename, fullFileContent, cleanDiff string) model.Prompt {
	systemPrompt := fmt.Sprintf(reviewSystemPromptTemplate, tb.language.Instructions)
	userPrompt := fmt.Sprintf(structuredReviewUserPromptTemplate,
		"", // No additional context
		filename,
		fullFileContent,
		cleanDiff,
	)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// BuildScoringPrompt creates a prompt for scoring review comments to enable filtering
func (tb *Builder) BuildScoringPrompt(comments []*model.ReviewAIComment, filePath, diff string) model.Prompt {
	systemPrompt := fmt.Sprintf(scoringSystemPromptTemplate, tb.language.Instructions)

	// Estimate and pre-allocate comments builder capacity
	estimatedSize := len(comments) * 300 // ~300 chars average per comment
	for _, comment := range comments {
		estimatedSize += len(comment.Title) + len(comment.Description) +
			len(comment.Suggestion) + len(comment.CodeSnippet)
	}

	var commentsBuilder strings.Builder
	commentsBuilder.Grow(estimatedSize)

	// Build comments with efficient sprintf usage
	for i, comment := range comments {
		lineRange := strconv.Itoa(comment.Line)
		if comment.EndLine > comment.Line {
			lineRange += "-" + strconv.Itoa(comment.EndLine)
		}

		// Main comment structure with sprintf for readability
		commentsBuilder.WriteString(fmt.Sprintf("**Comment %d:**\n- Line: %s\n- Issue Type: %s\n- Priority: %s\n- Confidence: %s\n- Title: %s\n- Description: %s\n",
			i+1, lineRange, comment.IssueType, comment.Priority, comment.Confidence, comment.Title, comment.Description))

		// Optional fields
		if comment.Suggestion != "" {
			commentsBuilder.WriteString(fmt.Sprintf("- Suggestion: %s\n", comment.Suggestion))
		}

		if comment.CodeSnippet != "" {
			commentsBuilder.WriteString(fmt.Sprintf("- Code Snippet: ```\n%s\n```\n", comment.CodeSnippet))
		}

		commentsBuilder.WriteString("\n")
	}

	// Build user prompt with simple sprintf
	userPrompt := fmt.Sprintf(scoringUserPromptTemplate, filePath, diff, commentsBuilder.String())

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}
