package prompts

import (
	"fmt"
	"strings"

	"github.com/maxbolgarin/codry/internal/models"
)

// LanguageConfig defines the target language for AI responses
type LanguageConfig struct {
	Code         string `yaml:"code"`         // Language code (en, es, fr, de, ru, etc.)
	Name         string `yaml:"name"`         // Human-readable language name
	Instructions string `yaml:"instructions"` // Language-specific instructions for the AI
}

// DefaultLanguages provides common language configurations
var DefaultLanguages = map[string]LanguageConfig{
	"en": {
		Code:         "en",
		Name:         "English",
		Instructions: "Respond in clear, professional English. Use technical terminology appropriately.",
	},
	"es": {
		Code:         "es",
		Name:         "Spanish",
		Instructions: "Responde en español claro y profesional. Usa terminología técnica apropiada.",
	},
	"fr": {
		Code:         "fr",
		Name:         "French",
		Instructions: "Répondez en français clair et professionnel. Utilisez une terminologie technique appropriée.",
	},
	"de": {
		Code:         "de",
		Name:         "German",
		Instructions: "Antworten Sie in klarem, professionellem Deutsch. Verwenden Sie angemessene technische Terminologie.",
	},
	"ru": {
		Code:         "ru",
		Name:         "Russian",
		Instructions: "Отвечайте на русском языке четко и профессионально. Используйте соответствующую техническую терминологию.",
	},
	"pt": {
		Code:         "pt",
		Name:         "Portuguese",
		Instructions: "Responda em português claro e profissional. Use terminologia técnica apropriada.",
	},
	"it": {
		Code:         "it",
		Name:         "Italian",
		Instructions: "Rispondi in italiano chiaro e professionale. Usa una terminologia tecnica appropriata.",
	},
	"ja": {
		Code:         "ja",
		Name:         "Japanese",
		Instructions: "明確で専門的な日本語で回答してください。適切な技術用語を使用してください。",
	},
	"ko": {
		Code:         "ko",
		Name:         "Korean",
		Instructions: "명확하고 전문적인 한국어로 답변해 주세요. 적절한 기술 용어를 사용해 주세요.",
	},
	"zh": {
		Code:         "zh",
		Name:         "Chinese",
		Instructions: "请用清晰、专业的中文回答。适当使用技术术语。",
	},
}

// PromptTemplate represents a structured prompt template
type PromptTemplate struct {
	SystemPrompt string
	UserPrompt   string
	Language     LanguageConfig
}

// TemplateBuilder provides methods to build prompts with language support
type TemplateBuilder struct {
	language LanguageConfig
}

// NewTemplateBuilder creates a new template builder with language configuration
func NewTemplateBuilder(languageCode string) *TemplateBuilder {
	lang, exists := DefaultLanguages[languageCode]
	if !exists {
		lang = DefaultLanguages["en"] // Default to English
	}

	return &TemplateBuilder{
		language: lang,
	}
}

// BuildDescriptionPrompt creates a prompt for generating PR/MR descriptions
func (tb *TemplateBuilder) BuildDescriptionPrompt(diff string) PromptTemplate {
	systemPrompt := fmt.Sprintf(`You are an expert software engineer and technical writer specializing in code analysis and documentation.

Your task is to analyze code changes and generate a clear, comprehensive description for a Pull Request or Merge Request.

CORE PRINCIPLES:
- Focus on the business impact and technical changes
- Use clear, concise language that both technical and non-technical stakeholders can understand
- Organize information logically with proper categorization
- Highlight significant changes while avoiding trivial details
- Maintain a professional, informative tone

LANGUAGE INSTRUCTIONS:
%s

ANALYSIS FRAMEWORK:
1. Identify the primary purpose of the changes
2. Categorize changes by type and impact
3. Focus on what changed, why it matters, and how it affects the system
4. Use action-oriented language that clearly communicates the changes

FORMATTING REQUIREMENTS:
- Use markdown formatting for structure and readability
- Create logical sections with clear headings
- Use bullet points for lists and multiple items
- Keep descriptions concise but comprehensive
- Ensure the output is ready for direct use in the PR/MR description`, tb.language.Instructions)

	userPrompt := fmt.Sprintf(`Analyze the following code changes and generate a structured description.

STRUCTURE YOUR RESPONSE IN THESE SECTIONS (only include sections that are relevant):

**🚀 New Features**
- Describe new functionality, APIs, components, or modules
- Highlight new configuration options and their purpose
- Focus on user-facing or system-level enhancements

**🔄 Changes & Improvements**
- Describe modifications to existing functionality
- Explain performance improvements or optimizations
- Highlight reliability and stability enhancements

**🐛 Bug Fixes**
- List resolved issues and problems
- Describe improved error handling and edge cases
- Focus on fixes that impact user experience or system stability

**♻️ Refactoring & Code Quality**
- Describe code structure improvements
- Highlight architectural enhancements
- Mention improved maintainability and readability

**🧪 Testing**
- List new or updated tests
- Describe improved test coverage
- Highlight testing infrastructure improvements

**🗑️ Removals & Cleanup**
- Describe removed deprecated components
- List cleaned up unused code or dependencies
- Mention simplified or consolidated functionality

**📋 Other Changes**
- Documentation updates
- Configuration changes
- Dependency updates
- Build and deployment improvements

GUIDELINES:
- Only include sections that have actual changes
- Be specific about what changed, not how it was implemented
- Focus on the impact and benefit of each change
- Use clear, action-oriented language
- Avoid technical jargon unless necessary
- Keep each bullet point focused on a single change or improvement

Code changes to analyze:
---
%s
---

Generate a clear, well-structured description:`, diff)

	return PromptTemplate{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language,
	}
}

// BuildReviewPrompt creates a prompt for code review
func (tb *TemplateBuilder) BuildReviewPrompt(filename, diff string) PromptTemplate {
	systemPrompt := fmt.Sprintf(`You are a senior software engineer and code reviewer with expertise in software architecture, security, performance, and best practices.

Your role is to provide thorough, constructive code reviews that help improve code quality, maintainability, and reliability.

CORE RESPONSIBILITIES:
- Identify potential bugs, security vulnerabilities, and logic errors
- Evaluate code architecture, design patterns, and best practices
- Assess performance implications and optimization opportunities
- Review code maintainability, readability, and documentation
- Ensure compliance with coding standards and conventions

LANGUAGE INSTRUCTIONS:
%s

REVIEW METHODOLOGY:
1. Analyze code changes line by line for potential issues
2. Consider the broader architectural impact of changes
3. Evaluate security, performance, and maintainability aspects
4. Provide specific, actionable feedback with examples
5. Balance thoroughness with practicality

COMMUNICATION STYLE:
- Be constructive and professional
- Provide specific examples and suggestions
- Focus on significant issues over minor style preferences
- Explain the reasoning behind your recommendations
- Offer alternative approaches when appropriate`, tb.language.Instructions)

	userPrompt := fmt.Sprintf(`Review the following code changes for the file "%s" and provide detailed feedback.

FOCUS AREAS FOR REVIEW:

**🐛 Potential Bugs & Logic Issues:**
- Incorrect logic or algorithm implementations
- Missing edge case handling
- Null/undefined reference issues
- Race conditions and concurrency problems
- Resource leaks and cleanup issues
- Error handling gaps or improper exception management

**🔒 Security Concerns:**
- Input validation and sanitization
- Authentication and authorization issues
- Data exposure and privacy concerns
- Injection vulnerabilities (SQL, XSS, etc.)
- Cryptographic implementation issues
- Secrets and sensitive data handling

**⚡ Performance Issues:**
- Inefficient algorithms or data structures
- Unnecessary computations or redundant operations
- Memory allocation and garbage collection concerns
- I/O operations and database query optimization
- Caching opportunities and strategies
- Scalability considerations

**🏗️ Architecture & Design:**
- SOLID principles adherence
- Design pattern implementation
- Separation of concerns
- Code coupling and cohesion
- Abstraction levels and interfaces
- Modularity and reusability

**📖 Code Quality & Maintainability:**
- Code readability and clarity
- Naming conventions and documentation
- Code duplication and DRY principle
- Complex logic that needs simplification
- Magic numbers and hard-coded values
- Test coverage and testability

**🎯 Standards & Best Practices:**
- Language-specific idioms and conventions
- Framework and library best practices
- Coding standards compliance
- Documentation and comments quality
- Version control and change management

REVIEW FORMAT:
For each issue found, provide:
1. **Clear issue title** - Brief description of the problem
2. **Detailed explanation** - Why this is a concern and potential impact
3. **Specific recommendation** - How to fix or improve the code
4. **Code example** - Show the problematic code and suggested improvement

Use markdown formatting for clarity and structure.

If no significant issues are found, respond with: "✅ **LGTM** - The changes look good. No critical issues identified."

IMPORTANT GUIDELINES:
- Focus on the added/modified lines (marked with '+' or context around them)
- Only comment on significant issues that impact functionality, security, or maintainability
- Provide specific, actionable feedback rather than general suggestions
- Be thorough but avoid nitpicking on minor style issues
- Consider the broader context and impact of the changes

File: %s

Code changes to review:
---
%s
---

Provide your detailed code review:`, filename, filename, diff)

	return PromptTemplate{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language,
	}
}

// BuildSummaryPrompt creates a prompt for summarizing multiple file changes
func (tb *TemplateBuilder) BuildSummaryPrompt(changes []*models.FileDiff) PromptTemplate {
	systemPrompt := fmt.Sprintf(`You are a technical lead and software architect responsible for providing high-level overviews of code changes across multiple files.

Your task is to analyze changes across multiple files and create a comprehensive, coherent summary that explains the overall impact and purpose of the changes.

CORE OBJECTIVES:
- Identify overarching themes and patterns in the changes
- Explain the business or technical motivation behind the changes
- Highlight cross-file dependencies and relationships
- Assess the overall impact on the system architecture
- Provide insights into the change management strategy

LANGUAGE INSTRUCTIONS:
%s

ANALYSIS APPROACH:
1. Look for patterns and themes across all changed files
2. Identify the main purpose or goal of the collective changes
3. Group related changes together logically
4. Consider the architectural and system-wide implications
5. Focus on the big picture while noting important details

SUMMARY STRUCTURE:
- Lead with the main purpose and scope of changes
- Group related changes by functionality or component
- Highlight significant architectural or design decisions
- Note any potential impacts or considerations
- Conclude with the overall assessment of the changes`, tb.language.Instructions)

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

	userPrompt := fmt.Sprintf(`Analyze the following code changes across multiple files and provide a comprehensive summary.

ANALYSIS REQUIREMENTS:

**🎯 Overall Purpose:**
- What is the main goal or objective of these changes?
- What problem is being solved or feature being implemented?
- How do the changes work together to achieve the objective?

**🔍 Change Categories:**
- Group related changes by functionality, component, or purpose
- Identify new features, improvements, fixes, and refactoring
- Highlight any infrastructure, configuration, or tooling changes

**🏗️ Architectural Impact:**
- How do these changes affect the overall system architecture?
- Are there new patterns, dependencies, or integrations introduced?
- What are the implications for system scalability, performance, or security?

**📈 Impact Assessment:**
- What components or systems are affected by these changes?
- Are there any breaking changes or compatibility considerations?
- How significant are these changes in terms of scope and complexity?

**🔗 Cross-File Relationships:**
- How do the changes in different files relate to each other?
- Are there dependencies or interactions between the modified components?
- What is the sequence or flow of the changes?

SUMMARY GUIDELINES:
- Provide a clear, executive-level overview
- Use technical language appropriate for developers and architects
- Focus on the most important and impactful changes
- Structure the response logically with clear sections
- Include specific file names when relevant to the discussion
- Conclude with an overall assessment of the change set

Files analyzed: %d
Total changes: %d lines across %d files

Code changes to summarize:
---
%s
---

Provide a comprehensive summary of these changes:`, len(changes), tb.countTotalLines(changes), len(changes), changesText.String())

	return PromptTemplate{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language,
	}
}

// BuildCustomPrompt creates a custom prompt with language support
func (tb *TemplateBuilder) BuildCustomPrompt(systemPrompt, userPrompt string, additionalContext ...string) PromptTemplate {
	// Add language instructions to system prompt
	enhancedSystemPrompt := fmt.Sprintf(`%s

LANGUAGE INSTRUCTIONS:
%s

%s`, systemPrompt, tb.language.Instructions, strings.Join(additionalContext, "\n\n"))

	return PromptTemplate{
		SystemPrompt: enhancedSystemPrompt,
		UserPrompt:   userPrompt,
		Language:     tb.language,
	}
}

// GetLanguage returns the current language configuration
func (tb *TemplateBuilder) GetLanguage() LanguageConfig {
	return tb.language
}

// SetLanguage updates the language configuration
func (tb *TemplateBuilder) SetLanguage(languageCode string) {
	if lang, exists := DefaultLanguages[languageCode]; exists {
		tb.language = lang
	}
}

// GetSupportedLanguages returns all supported language codes
func GetSupportedLanguages() []string {
	var languages []string
	for code := range DefaultLanguages {
		languages = append(languages, code)
	}
	return languages
}

// countTotalLines counts the total number of lines across all diffs
func (tb *TemplateBuilder) countTotalLines(changes []*models.FileDiff) int {
	total := 0
	for _, change := range changes {
		total += len(strings.Split(change.Diff, "\n"))
	}
	return total
}

// PromptOptimizations contains advanced prompt engineering techniques
type PromptOptimizations struct {
	UseChainOfThought bool    // Enable step-by-step reasoning
	UseRolePlay       bool    // Use role-playing for better context
	UseExamples       bool    // Include few-shot examples
	UseConstraints    bool    // Add specific constraints and guidelines
	UseOutputFormat   bool    // Specify exact output format
	Temperature       float32 // Control creativity vs consistency
	MaxTokens         int     // Limit response length
}

// DefaultOptimizations provides recommended prompt optimizations
var DefaultOptimizations = PromptOptimizations{
	UseChainOfThought: true,
	UseRolePlay:       true,
	UseExamples:       false, // Can be resource intensive
	UseConstraints:    true,
	UseOutputFormat:   true,
	Temperature:       0.1, // Low for consistent, focused responses
	MaxTokens:         4000,
}

// ApplyOptimizations enhances a prompt template with advanced techniques
func (tb *TemplateBuilder) ApplyOptimizations(template PromptTemplate, opts PromptOptimizations) PromptTemplate {
	systemPrompt := template.SystemPrompt

	if opts.UseChainOfThought {
		systemPrompt += "\n\nUSE CHAIN OF THOUGHT: Break down complex analysis into clear, logical steps. Think through each aspect systematically before providing your final response."
	}

	if opts.UseConstraints {
		systemPrompt += "\n\nCONSTRAINTS:\n- Be specific and actionable in your feedback\n- Focus on significant issues over minor style preferences\n- Provide concrete examples and suggestions\n- Use proper markdown formatting\n- Stay within the specified scope and context"
	}

	if opts.UseOutputFormat {
		systemPrompt += "\n\nOUTPUT FORMAT: Structure your response with clear headings, bullet points, and code blocks where appropriate. Ensure the output is well-formatted and ready for use in a development environment."
	}

	return PromptTemplate{
		SystemPrompt: systemPrompt,
		UserPrompt:   template.UserPrompt,
		Language:     template.Language,
	}
}
