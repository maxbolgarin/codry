package reviewer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/reviewer/llmcontext"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

func (s *Reviewer) generateCodeReview(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.Generate.CodeReview {
		s.logFlow(bundle.log, "code review is disabled, skipping")
		return
	}
	s.logFlow(bundle.log, "generating code review")

	for _, change := range bundle.request.Context.FilesForReview {
		if err := s.reviewCodeChanges(ctx, bundle, change); err != nil {
			msg := "failed to perform basic review"
			bundle.log.Err(err, msg)
			if strings.Contains(err.Error(), "context canceled") {
				return
			}
			bundle.result.Errors = append(bundle.result.Errors, erro.Wrap(err, msg))
		}
	}

	s.logFlow(bundle.log, "finished code review")

	bundle.result.IsCodeReviewCreated = true
}

// reviewCodeChanges reviews individual files and creates comments
func (s *Reviewer) reviewCodeChanges(ctx context.Context, bundle *reviewBundle, change *llmcontext.FileContext) error {
	// Guard old path
	change.Diff.OldPath = lang.Check(change.Diff.OldPath, change.Diff.NewPath)

	s.logFlow(bundle.log, "performing review", "file", change.Diff.NewPath)

	reviewResult, err := s.performContextAwareReview(ctx, bundle.request, change, bundle.log)
	if err != nil {
		return err
	}

	// Skip if no issues found
	if reviewResult == nil || !reviewResult.HasIssues || len(reviewResult.Comments) == 0 {
		bundle.log.DebugIf(s.cfg.Verbose, "no issues found", "file", change.Diff.NewPath)
		return nil
	}

	commentsCreated := s.processReviewResults(ctx, bundle.request, change, reviewResult, bundle.log)
	bundle.result.CommentsCreated += commentsCreated

	s.logFlow(bundle.log, "reviewed successfully",
		"file", change.Diff.NewPath,
		"comments", len(reviewResult.Comments),
	)

	return nil
}

// performBasicReview performs basic review without enhanced context (fallback)
func (s *Reviewer) performBasicReview(ctx context.Context, request ReviewRequest, change *llmcontext.FileContext, log logze.Logger) (*model.FileReviewResult, error) {
	fullFileContent, cleanDiff, err := s.prepareFileContentAndDiff(ctx, request, change.Diff, log)
	if err != nil {
		return nil, erro.Wrap(err, "failed to prepare file content and diff")
	}
	return s.agent.ReviewCode(ctx, change.Diff.NewPath, cleanDiff, fullFileContent)
}

// performContextAwareReview performs enhanced review with rich context when available, falls back to basic review
func (s *Reviewer) performContextAwareReview(ctx context.Context, request ReviewRequest, change *llmcontext.FileContext, log logze.Logger) (*model.FileReviewResult, error) {
	// Generate clean diff with logical grouping
	cleanDiff, err := s.parser.GenerateCleanDiff(change.Diff.Diff)
	if err != nil {
		return nil, erro.Wrap(err, "failed to generate clean diff")
	}
	return s.agent.ReviewCodeWithContext(ctx, change.Diff.NewPath, cleanDiff, change.Context, request.Context.MR)
}

// processReviewResults processes the review results and creates comments
func (s *Reviewer) processReviewResults(ctx context.Context, request ReviewRequest, change *llmcontext.FileContext, reviewResult *model.FileReviewResult, log logze.Logger) int {
	commentsCreated := 0

	// Enhance comments with diff position information and set programming language
	if err := s.parser.EnhanceReviewComments(change.Diff.Diff, reviewResult.Comments); err != nil {
		log.Warn("failed to enhance comments with diff positions", "error", err)
	}

	// Set programming language for each comment if not already set
	detectedLanguage := detectProgrammingLanguage(change.Diff.NewPath)
	for _, comment := range reviewResult.Comments {
		comment.CodeLanguage = lang.Check(comment.CodeLanguage, detectedLanguage)
	}

	// Score and filter comments based on scoring mode
	filteredComments := s.scoreAndFilterComments(ctx, reviewResult.Comments, change.Diff, log)

	// Create line-specific comments
	for _, reviewComment := range filteredComments {
		// Ensure file path is set (AI might not include it in JSON response)
		if reviewComment.FilePath == "" {
			reviewComment.FilePath = change.Diff.NewPath
		}

		comment := buildComment(s.cfg.Language, reviewComment)

		err := s.provider.CreateComment(ctx, request.ProjectID, request.Context.MR.IID, comment)
		if err != nil {
			log.Error("failed to create comment", "error", err, "file", change.Diff.NewPath, "line", reviewComment.Line)
			continue
		}

		commentsCreated++

		log.DebugIf(s.cfg.Verbose,
			"created comment",
			"file", change.Diff.NewPath,
			"line", reviewComment.Line,
			"type", reviewComment.IssueType,
			"impact", reviewComment.IssueImpact,
			"priority", reviewComment.FixPriority,
			"confidence", reviewComment.ModelConfidence)
	}

	// Log filtering results if scoring was used
	if len(reviewResult.Comments) > len(filteredComments) {
		filteredCount := len(reviewResult.Comments) - len(filteredComments)
		s.logFlow(log, "filtered low-quality comments",
			"total_comments", len(reviewResult.Comments),
			"filtered_count", filteredCount,
			"final_count", len(filteredComments),
			"scoring_mode", string(s.cfg.Scoring.Mode),
			"file", change.Diff.NewPath)
	}

	return commentsCreated
}

// prepareFileContentAndDiff gets the original file content (before changes) and clean diff format
func (s *Reviewer) prepareFileContentAndDiff(ctx context.Context, request ReviewRequest, change *model.FileDiff, log logze.Logger) (string, string, error) {
	// Generate clean diff with logical grouping
	cleanDiff, err := s.parser.GenerateCleanDiff(change.Diff)
	if err != nil {
		return "", "", erro.Wrap(err, "failed to generate clean diff")
	}

	// Handle new files - no original content exists
	if change.IsNew {
		return "", cleanDiff, nil
	}

	// Handle deleted files - get original content before deletion
	if change.IsDeleted {
		originalContent, err := s.getOriginalFileContent(ctx, request, change.OldPath, log)
		if err != nil {
			log.Warn("failed to get original content for deleted file", "error", err, "file", change.OldPath)
			return "", cleanDiff, nil
		}
		return originalContent, cleanDiff, nil
	}

	// For modified files, get the original content (before changes)
	originalContent, err := s.getOriginalFileContent(ctx, request, change.OldPath, log)
	if err != nil {
		log.Warn("failed to get original file content", "error", err, "file", change.OldPath)
		return "", cleanDiff, nil
	}

	return originalContent, cleanDiff, nil
}

// getOriginalFileContent retrieves the original file content before changes
func (s *Reviewer) getOriginalFileContent(ctx context.Context, request ReviewRequest, filePath string, log logze.Logger) (string, error) {
	// Try to get the file content from the target branch (base branch)
	// This represents the "before" state that changes are being applied to
	if request.Context.MR.TargetBranch != "" {
		content, err := s.provider.GetFileContent(ctx, request.ProjectID, filePath, request.Context.MR.TargetBranch)
		if err == nil {
			return content, nil
		}
		log.Debug("failed to get content from target branch, trying source commit parent", "error", err)
	}

	// Fallback: try to get from source commit (this will be the "after" state, but better than nothing)
	// In a proper implementation, we'd want to get the parent commit of the source branch
	if request.Context.MR.SHA != "" {
		content, err := s.provider.GetFileContent(ctx, request.ProjectID, filePath, request.Context.MR.SHA)
		if err != nil {
			return "", erro.Wrap(err, "failed to get file content from any source")
		}
		log.Warn("using source commit content as fallback - this may include some changes")
		return content, nil
	}

	return "", erro.New("no valid commit reference available")
}

// Helper methods for tracking processed MRs and files

func (s *Reviewer) getFileHash(diff string) string {
	if diff == "" {
		return ""
	}

	var hash uint64
	for i, char := range diff {
		hash = (hash*31 + uint64(char))
		if i >= 100 {
			break
		}
	}

	return fmt.Sprintf("%d:%d", len(diff), hash)
}

// buildComment converts a LineReviewComment to a Comment model
func buildComment(language model.Language, lrc *model.ReviewAIComment) *model.Comment {
	reviewHeaders := prompts.DefaultLanguages[language].CodeReviewHeaders

	comment := strings.Builder{}
	comment.WriteString(startMarkerCodeReview)
	comment.WriteString("## ")
	comment.WriteString(reviewHeaders.GetByType(lrc.IssueType))
	comment.WriteString("\n\n**")
	comment.WriteString(reviewHeaders.IssueImpactHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetIssueImpact(lrc.IssueImpact))
	comment.WriteString("\n**")
	comment.WriteString(reviewHeaders.FixPriorityHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetPriority(lrc.FixPriority))
	comment.WriteString("\n**")
	comment.WriteString(reviewHeaders.ModelConfidenceHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetConfidence(lrc.ModelConfidence))
	comment.WriteString("\n\n")
	if lrc.Title != "" {
		comment.WriteString("### ")
		comment.WriteString(lrc.Title)
		comment.WriteString("\n\n")
	}
	if lrc.Description != "" {
		comment.WriteString(lrc.Description)
		comment.WriteString("\n\n")
	}

	// Add current problematic code section if we have a code snippet
	if lrc.Suggestion != "" {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.SuggestionHeader)
		comment.WriteString("\n\n")
		comment.WriteString(lrc.Suggestion)
		if lrc.CodeSnippet != "" {
			comment.WriteString("\n\n")

			if strings.HasPrefix(lrc.CodeSnippet, "`") {
				comment.WriteString(lrc.CodeSnippet)
			} else {
				comment.WriteString("```")
				comment.WriteString(lrc.CodeLanguage)
				comment.WriteString("\n")
				comment.WriteString(lrc.CodeSnippet)
				comment.WriteString("\n```")
			}
		}
	}
	comment.WriteString(endMarkerCodeReview)

	body := comment.String()

	return &model.Comment{
		Body:     body,
		FilePath: lrc.FilePath,
		Line:     lrc.Line,
		OldLine:  lrc.OldLine,
		Position: lrc.Position,
		Type:     model.CommentTypeInline,
	}
}

// detectProgrammingLanguage detects programming language from file path
func detectProgrammingLanguage(filePath string) string {
	if filePath == "" {
		return "text"
	}

	// Get the file extension (including the dot)
	ext := strings.ToLower(filepath.Ext(filePath))

	// Map file extensions to language identifiers for markdown syntax highlighting
	languageMap := map[string]string{
		// Go
		".go": "go",

		// JavaScript/TypeScript
		".js":  "javascript",
		".jsx": "jsx",
		".ts":  "typescript",
		".tsx": "tsx",
		".vue": "vue",

		// Python
		".py":  "python",
		".pyw": "python",
		".pyi": "python",

		// Java
		".java": "java",
		".kt":   "kotlin",
		".kts":  "kotlin",

		// C/C++
		".c":   "c",
		".h":   "c",
		".cpp": "cpp",
		".cxx": "cpp",
		".cc":  "cpp",
		".hpp": "cpp",
		".hxx": "cpp",

		// C#
		".cs":  "csharp",
		".csx": "csharp",

		// Ruby
		".rb":  "ruby",
		".rbw": "ruby",

		// PHP
		".php":   "php",
		".phtml": "php",

		// Rust
		".rs": "rust",

		// Swift
		".swift": "swift",

		// Shell scripts
		".sh":   "bash",
		".bash": "bash",
		".zsh":  "zsh",
		".fish": "fish",

		// Web technologies
		".html": "html",
		".htm":  "html",
		".css":  "css",
		".scss": "scss",
		".sass": "sass",
		".less": "less",

		// Data formats
		".json": "json",
		".xml":  "xml",
		".yaml": "yaml",
		".yml":  "yaml",
		".toml": "toml",

		// Database
		".sql": "sql",

		// Configuration
		".ini":  "ini",
		".cfg":  "ini",
		".conf": "ini",

		// Documentation
		".md":       "markdown",
		".markdown": "markdown",
		".txt":      "text",

		// Docker
		".dockerfile": "dockerfile",
		"dockerfile":  "dockerfile",

		// Other languages
		".lua":   "lua",
		".perl":  "perl",
		".pl":    "perl",
		".r":     "r",
		".R":     "r",
		".scala": "scala",
		".clj":   "clojure",
		".hs":    "haskell",
		".elm":   "elm",
		".ex":    "elixir",
		".exs":   "elixir",
		".erl":   "erlang",
		".hrl":   "erlang",
		".dart":  "dart",
		".vim":   "vim",
	}

	// Special case for common filenames without extensions
	fileName := strings.ToLower(filepath.Base(filePath))
	switch fileName {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "gemfile":
		return "ruby"
	case "rakefile":
		return "ruby"
	case "package.json":
		return "json"
	case "composer.json":
		return "json"
	case ".gitignore", ".dockerignore", ".eslintignore":
		return "gitignore"
	case ".env", ".env.example":
		return "bash"
	}

	// Look up the extension in our language map
	if language, exists := languageMap[ext]; exists {
		return language
	}

	// If we can't determine the language, return a generic text format
	return "text"
}
