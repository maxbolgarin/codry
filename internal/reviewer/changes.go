package reviewer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

func (s *Reviewer) generateCodeReview(ctx context.Context, bundle *reviewBundle) {
	if !s.cfg.EnableCodeReview {
		bundle.log.InfoIf(s.cfg.Verbose, "code review is disabled, skipping")
		return
	}
	bundle.log.Debug("generating code review")

	for _, change := range bundle.filesToReview {
		s.reviewCodeChanges(ctx, bundle, change)
	}

	bundle.log.InfoIf(s.cfg.Verbose, "finished code review")

	bundle.result.IsCodeReviewCreated = true
}

// reviewCodeChanges reviews individual files and creates comments
func (s *Reviewer) reviewCodeChanges(ctx context.Context, bundle *reviewBundle, change *model.FileDiff) {
	// Guard old path
	change.OldPath = lang.Check(change.OldPath, change.NewPath)

	fileHash := s.getFileHash(change.Diff)
	if oldHash, ok := s.processedMRs.Lookup(bundle.request.String(), change.NewPath); ok {
		if oldHash == fileHash {
			bundle.log.DebugIf(s.cfg.Verbose, "skipping already reviewed", "file", change.NewPath)
			return
		}
	}

	bundle.log.DebugIf(s.cfg.Verbose, "performing review", "file", change.NewPath)

	reviewResult, err := s.performBasicReview(ctx, bundle.request, change, bundle.log)
	if err != nil {
		msg := "failed to perform basic review"
		bundle.log.Err(err, msg)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	// Skip if no issues found
	if reviewResult == nil || !reviewResult.HasIssues || len(reviewResult.Comments) == 0 {
		bundle.log.DebugIf(s.cfg.Verbose, "no issues found", "file", change.NewPath)
		s.processedMRs.Set(bundle.request.String(), change.NewPath, fileHash)
		return
	}

	commentsCreated := s.processReviewResults(ctx, bundle.request, change, reviewResult, bundle.log)
	bundle.result.CommentsCreated += commentsCreated
	s.processedMRs.Set(bundle.request.String(), change.NewPath, fileHash)

	bundle.log.InfoIf(s.cfg.Verbose, "reviewed successfully", "file", change.NewPath, "comments", len(reviewResult.Comments))
}

// performBasicReview performs basic review without enhanced context (fallback)
func (s *Reviewer) performBasicReview(ctx context.Context, request model.ReviewRequest, change *model.FileDiff, log logze.Logger) (*model.FileReviewResult, error) {
	fullFileContent, cleanDiff, err := s.prepareFileContentAndDiff(ctx, request, change, log)
	if err != nil {
		return nil, errm.Wrap(err, "failed to prepare file content and diff")
	}
	return s.agent.ReviewCode(ctx, change.NewPath, fullFileContent, cleanDiff)
}

// processReviewResults processes the review results and creates comments
func (s *Reviewer) processReviewResults(ctx context.Context, request model.ReviewRequest, change *model.FileDiff, reviewResult *model.FileReviewResult, log logze.Logger) int {
	commentsCreated := 0

	// Enhance comments with diff position information and set programming language
	if err := s.parser.enhanceReviewComments(change.Diff, reviewResult.Comments); err != nil {
		log.Warn("failed to enhance comments with diff positions", "error", err)
	}

	// Set programming language for each comment if not already set
	detectedLanguage := detectProgrammingLanguage(change.NewPath)
	for _, comment := range reviewResult.Comments {
		comment.CodeLanguage = lang.Check(comment.CodeLanguage, detectedLanguage)
	}

	// Score and filter comments based on scoring mode
	var filteredComments []*model.ReviewAIComment
	if s.cfg.Scoring.Mode != ScoringModeDisabled {
		filteredComments = s.scoreAndFilterComments(ctx, reviewResult.Comments, change, log)
	} else {
		filteredComments = reviewResult.Comments
	}

	// Create line-specific comments
	for _, reviewComment := range filteredComments {
		// Ensure file path is set (AI might not include it in JSON response)
		if reviewComment.FilePath == "" {
			reviewComment.FilePath = change.NewPath
		}

		comment := buildComment(s.cfg.Language, reviewComment)

		err := s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
		if err != nil {
			log.Error("failed to create comment", "error", err, "file", change.NewPath, "line", reviewComment.Line)
			continue
		}

		commentsCreated++

		log.DebugIf(s.cfg.Verbose,
			"created comment",
			"file", change.NewPath,
			"line", reviewComment.Line,
			"type", reviewComment.IssueType,
			"priority", reviewComment.Priority,
			"confidence", reviewComment.Confidence)
	}

	// Log filtering results if scoring was used
	if s.cfg.Scoring.Mode != ScoringModeDisabled && len(reviewResult.Comments) > len(filteredComments) {
		filteredCount := len(reviewResult.Comments) - len(filteredComments)
		log.InfoIf(s.cfg.Verbose, "filtered low-quality comments",
			"total_comments", len(reviewResult.Comments),
			"filtered_count", filteredCount,
			"final_count", len(filteredComments),
			"scoring_mode", string(s.cfg.Scoring.Mode),
			"file", change.NewPath)
	}

	return commentsCreated
}

// prepareFileContentAndDiff gets the original file content (before changes) and clean diff format
func (s *Reviewer) prepareFileContentAndDiff(ctx context.Context, request model.ReviewRequest, change *model.FileDiff, log logze.Logger) (string, string, error) {
	// Generate clean diff with logical grouping
	cleanDiff, err := s.parser.GenerateCleanDiff(change.Diff)
	if err != nil {
		return "", "", errm.Wrap(err, "failed to generate clean diff")
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
func (s *Reviewer) getOriginalFileContent(ctx context.Context, request model.ReviewRequest, filePath string, log logze.Logger) (string, error) {
	// Try to get the file content from the target branch (base branch)
	// This represents the "before" state that changes are being applied to
	if request.MergeRequest.TargetBranch != "" {
		content, err := s.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.TargetBranch)
		if err == nil {
			return content, nil
		}
		log.Debug("failed to get content from target branch, trying source commit parent", "error", err)
	}

	// Fallback: try to get from source commit (this will be the "after" state, but better than nothing)
	// In a proper implementation, we'd want to get the parent commit of the source branch
	if request.MergeRequest.SHA != "" {
		content, err := s.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.SHA)
		if err != nil {
			return "", errm.Wrap(err, "failed to get file content from any source")
		}
		log.Warn("using source commit content as fallback - this may include some changes")
		return content, nil
	}

	return "", errm.New("no valid commit reference available")
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
	header := reviewHeaders.GetByType(lrc.IssueType)

	comment := strings.Builder{}
	comment.WriteString("## ")
	comment.WriteString(header)
	comment.WriteString("\n\n**")
	comment.WriteString(reviewHeaders.ConfidenceHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetConfidence(lrc.Confidence))
	comment.WriteString("\n**")
	comment.WriteString(reviewHeaders.PriorityHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetPriority(lrc.Priority))
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
