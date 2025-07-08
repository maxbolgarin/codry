package prompts

import (
	"fmt"
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

// BuildEnhancedStructuredReviewPrompt creates a prompt for structured code review with enhanced context
func (tb *Builder) BuildEnhancedReviewPrompt(filename string, enhancedCtx *EnhancedContext, cleanDiff string) model.Prompt {
	systemPrompt := fmt.Sprintf(reviewSystemPromptTemplate, tb.language.Instructions)

	// Build enhanced context section
	contextSection := tb.buildContextSection(enhancedCtx)

	fmt.Println(filename, contextSection)

	userPrompt := fmt.Sprintf(structuredReviewUserPromptTemplate,
		contextSection,
		filename,
		enhancedCtx.FileContent,
		cleanDiff,
	)

	return model.Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language.Language,
	}
}

// buildContextSection creates a rich context section for the prompt
func (tb *Builder) buildContextSection(ctx *EnhancedContext) string {
	var contextBuilder strings.Builder

	contextBuilder.WriteString("## 🧠 ENHANCED CONTEXT ANALYSIS\n\n")

	// Imported packages
	if len(ctx.ImportedPackages) > 0 {
		contextBuilder.WriteString("### 📦 IMPORTED PACKAGES:\n")
		for _, pkg := range ctx.ImportedPackages {
			contextBuilder.WriteString(fmt.Sprintf("- %s\n", pkg))
		}
		contextBuilder.WriteString("\n")
	}

	// Function signatures with enhanced information
	if len(ctx.FunctionSignatures) > 0 {
		contextBuilder.WriteString("### 🔧 FUNCTION SIGNATURES:\n")
		for _, fn := range ctx.FunctionSignatures {
			exported := ""
			if fn.IsExported {
				exported = " (exported)"
			}
			contextBuilder.WriteString(fmt.Sprintf("- **%s**%s", fn.Name, exported))
			if len(fn.Parameters) > 0 {
				contextBuilder.WriteString(fmt.Sprintf(" - params: %s", strings.Join(fn.Parameters, ", ")))
			}
			if len(fn.Returns) > 0 {
				contextBuilder.WriteString(fmt.Sprintf(" - returns: %s", strings.Join(fn.Returns, ", ")))
			}
			contextBuilder.WriteString("\n")
		}
		contextBuilder.WriteString("\n")
	}

	// Type definitions with enhanced information
	if len(ctx.TypeDefinitions) > 0 {
		contextBuilder.WriteString("### 🏗️ TYPE DEFINITIONS:\n")
		for _, typedef := range ctx.TypeDefinitions {
			exported := ""
			if typedef.IsExported {
				exported = " (exported)"
			}
			contextBuilder.WriteString(fmt.Sprintf("- **%s** %s%s", typedef.Name, typedef.Type, exported))
			if len(typedef.Fields) > 0 {
				contextBuilder.WriteString(fmt.Sprintf(" - fields: %s", strings.Join(typedef.Fields, ", ")))
			}
			if len(typedef.Methods) > 0 {
				contextBuilder.WriteString(fmt.Sprintf(" - methods: %s", strings.Join(typedef.Methods, ", ")))
			}
			contextBuilder.WriteString("\n")
		}
		contextBuilder.WriteString("\n")
	}

	// Enhanced security context
	secCtx := ctx.SecurityContext
	if secCtx.HasAuthenticationLogic || secCtx.HandlesUserInput || secCtx.AccessesDatabase ||
		secCtx.NetworkOperations || secCtx.CryptographicOperations {
		contextBuilder.WriteString("### 🔒 SECURITY CONTEXT:\n")
		if secCtx.HasAuthenticationLogic {
			contextBuilder.WriteString("🚨 **Authentication/Authorization logic detected** - Pay special attention to access controls\n")
		}
		if secCtx.HandlesUserInput {
			contextBuilder.WriteString("⚠️ **User input handling detected** - Verify input validation and sanitization\n")
		}
		if secCtx.AccessesDatabase {
			contextBuilder.WriteString("🛡️ **Database operations detected** - Check for SQL injection and data validation\n")
		}
		if secCtx.NetworkOperations {
			contextBuilder.WriteString("🌐 **Network operations detected** - Verify secure communication and error handling\n")
		}
		if secCtx.CryptographicOperations {
			contextBuilder.WriteString("🔐 **Cryptographic operations detected** - Ensure proper key management and algorithms\n")
		}
		contextBuilder.WriteString("\n")
	}

	// Usage patterns with real code examples
	if len(ctx.UsagePatterns) > 0 {
		contextBuilder.WriteString("### 🎯 USAGE PATTERNS:\n")
		for _, pattern := range ctx.UsagePatterns {
			contextBuilder.WriteString(fmt.Sprintf("- **%s**: %s\n", pattern.Pattern, pattern.Description))

			// Show actual code examples if available
			if len(pattern.Examples) > 0 {
				contextBuilder.WriteString("  ```\n")
				for i, example := range pattern.Examples {
					if i < 2 { // Limit to 2 examples to avoid overwhelming
						contextBuilder.WriteString(fmt.Sprintf("  %s\n", example))
					}
				}
				contextBuilder.WriteString("  ```\n")
			}

			// Add best practice guidance
			if pattern.BestPractice != "" {
				contextBuilder.WriteString(fmt.Sprintf("  💡 **Best Practice**: %s\n", pattern.BestPractice))
			}
		}
		contextBuilder.WriteString("\n")
	}

	// Related files with enhanced context
	if len(ctx.RelatedFiles) > 0 {
		contextBuilder.WriteString("### 🔗 RELATED FILES:\n")
		for _, related := range ctx.RelatedFiles {
			relationshipIcon := tb.getRelationshipIcon(related.Relationship)
			contextBuilder.WriteString(fmt.Sprintf("- %s **%s** (%s):\n", relationshipIcon, related.Path, related.Relationship))

			// Show snippet if it's meaningful
			if related.Snippet != "" && len(related.Snippet) < 300 {
				contextBuilder.WriteString("```\n")
				contextBuilder.WriteString(related.Snippet)
				contextBuilder.WriteString("\n```\n")
			}
		}
		contextBuilder.WriteString("\n")
	}

	// Semantic changes analysis with enhanced details
	if len(ctx.SemanticChanges) > 0 {
		contextBuilder.WriteString("### 🧠 SEMANTIC CHANGES ANALYSIS:\n")
		for _, change := range ctx.SemanticChanges {
			impactIcon := "💡"
			if change.Impact == "high" {
				impactIcon = "🚨"
			} else if change.Impact == "medium" {
				impactIcon = "⚠️"
			}
			contextBuilder.WriteString(fmt.Sprintf("- %s **%s** (%s impact): %s\n",
				impactIcon, change.Type, change.Impact, change.Description))
			if change.Context != "" {
				contextBuilder.WriteString(fmt.Sprintf("  📋 **Context**: %s\n", change.Context))
			}
			if len(change.Lines) > 0 {
				contextBuilder.WriteString(fmt.Sprintf("  📍 **Lines**: %v\n", change.Lines))
			}
		}
		contextBuilder.WriteString("\n")
	}

	return contextBuilder.String()
}

// getRelationshipIcon returns an appropriate icon for the relationship type
func (tb *Builder) getRelationshipIcon(relationship string) string {
	switch strings.ToLower(relationship) {
	case "dependency", "function_call", "method_call":
		return "📞"
	case "dependent", "dependents":
		return "🎯"
	case "test", "tests":
		return "🧪"
	case "same_package":
		return "📦"
	case "imports", "imported_by":
		return "📥"
	case "before_state":
		return "⏪"
	case "architecture", "architectural":
		return "🏗️"
	case "security":
		return "🔒"
	default:
		return "📄"
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
