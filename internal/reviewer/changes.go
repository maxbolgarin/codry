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

// reviewCodeChanges reviews individual files and creates comments
func (s *Reviewer) reviewCodeChanges(ctx context.Context, request model.ReviewRequest, changes []*model.FileDiff, log logze.Logger) (int, error) {
	if !s.cfg.EnableCodeReview {
		log.Info("code review is disabled, skipping")
		return 0, nil
	}

	log.Info("reviewing code changes")
	commentsCreated := 0

	errs := errm.NewList()
	for _, change := range changes {
		fileHash := s.getFileHash(change.Diff)
		if oldHash, ok := s.processedMRs.Lookup(request.String(), change.NewPath); ok {
			if oldHash == fileHash {
				log.Debug("skipping already reviewed", "file", change.NewPath)
				continue
			}
		}

		log.Debug("reviewing code change", "file", change.NewPath)

		// Get full file content (after changes applied) and clean diff
		fullFileContent, cleanDiff, err := s.prepareFileContentAndDiff(ctx, request, change, log)
		if err != nil {
			errs.Wrap(err, "failed to prepare file content and diff", "file", change.NewPath)
			log.Error("failed to prepare file content and diff", "error", err, "file", change.NewPath)
			continue
		}

		// Use enhanced structured review with full file content and clean diff
		reviewResult, err := s.agent.ReviewCode(ctx, change.NewPath, fullFileContent, cleanDiff)
		if err != nil {
			errs.Wrap(err, "failed to review code change", "file", change.NewPath)
			log.Error("failed to review code change", "error", err, "file", change.NewPath)
			continue
		}

		// Skip if no issues found
		if reviewResult == nil || !reviewResult.HasIssues || len(reviewResult.Comments) == 0 {
			log.Debug("no issues found in file", "file", change.NewPath)
			s.processedMRs.Set(request.String(), change.NewPath, fileHash)
			continue
		}

		// Enhance comments with diff position information and set programming language
		if err := s.parser.enhanceReviewComments(change.Diff, reviewResult.Comments); err != nil {
			log.Warn("failed to enhance comments with diff positions", "error", err)
		}

		// Set programming language for each comment if not already set
		detectedLanguage := detectProgrammingLanguage(change.NewPath)
		for _, comment := range reviewResult.Comments {
			if comment.CodeLanguage == "" {
				comment.CodeLanguage = detectedLanguage
			}
		}

		// Create line-specific comments
		for _, reviewComment := range reviewResult.Comments {
			// Ensure file path is set (AI might not include it in JSON response)
			if reviewComment.FilePath == "" {
				reviewComment.FilePath = change.NewPath
			}

			comment := ToComment(s.cfg.Language, reviewComment)
			comment.Type = model.CommentTypeInline

			err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
			if err != nil {
				errs.Wrap(err, "failed to create comment", "file", change.NewPath, "line", reviewComment.Line)
				log.Error("failed to create comment", "error", err, "file", change.NewPath, "line", reviewComment.Line)
				continue
			}

			commentsCreated++
			log.Info("created line-specific comment", "file", change.NewPath, "line", reviewComment.Line, "type", reviewComment.IssueType)
		}

		s.processedMRs.Set(request.String(), change.NewPath, fileHash)
		log.Info("file reviewed with line-specific comments", "file", change.NewPath, "comments", len(reviewResult.Comments))
	}

	return commentsCreated, errs.Err()
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

	change.OldPath = lang.Check(change.OldPath, change.NewPath)
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

// ToComment converts a LineReviewComment to a Comment model
func ToComment(language model.Language, lrc *model.ReviewAIComment) *model.Comment {
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
	comment.WriteString(reviewHeaders.SeverityHeader)
	comment.WriteString("**: ")
	comment.WriteString(reviewHeaders.GetSeverity(lrc.Severity))
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
	if lrc.Suggestion != "" && lrc.CodeSnippet != "" {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.SuggestionHeader)
		comment.WriteString("\n\n")
		comment.WriteString(lrc.Suggestion)
		comment.WriteString("\n\n```diff\n")
		comment.WriteString(lrc.CodeSnippet)
		comment.WriteString("\n```\n\n")
	}

	body := comment.String()

	return &model.Comment{
		Body:     body,
		FilePath: lrc.FilePath,
		Line:     lrc.Line,
		OldLine:  lrc.OldLine,
		Position: lrc.Position,
		Type:     model.CommentTypeReview,
	}
}

// cleanDiffSnippet removes diff prefixes (+, -, space) from code snippet and normalizes indentation
func cleanDiffSnippet(snippet string) string {
	lines := strings.Split(snippet, "\n")
	var cleaned []string

	if len(lines) == 0 {
		return ""
	}

	// First pass: remove diff prefixes and collect non-empty lines
	var processedLines []string
	for _, line := range lines {
		if len(line) == 0 {
			processedLines = append(processedLines, "")
			continue
		}

		// Remove diff prefixes: +, -, or space at the beginning
		cleanLine := line
		if line[0] == '+' || line[0] == '-' || line[0] == ' ' {
			cleanLine = line[1:]
		}
		processedLines = append(processedLines, cleanLine)
	}

	// // Second pass: find minimum leading whitespace (excluding empty lines)
	// minIndent := -1
	// for _, line := range processedLines {
	// 	if len(line) == 0 {
	// 		continue // Skip empty lines when calculating minimum indent
	// 	}

	// 	indent := 0
	// 	for i := 0; i < len(line); i++ {
	// 		if line[i] == ' ' || line[i] == '\t' {
	// 			if line[i] == '\t' {
	// 				indent += 4 // Count tabs as 4 spaces for normalization
	// 			} else {
	// 				indent++
	// 			}
	// 		} else {
	// 			break
	// 		}
	// 	}

	// 	if minIndent == -1 || indent < minIndent {
	// 		minIndent = indent
	// 	}
	// }

	// // If no indentation found, set to 0
	// if minIndent == -1 {
	// 	minIndent = 0
	// }

	// // Third pass: remove the minimum indentation from all lines
	// for _, line := range processedLines {
	// 	if len(line) == 0 {
	// 		cleaned = append(cleaned, "")
	// 		continue
	// 	}

	// 	// Remove leading whitespace up to minIndent
	// 	currentIndent := 0
	// 	startIndex := 0
	// 	for i := 0; i < len(line) && currentIndent < minIndent; i++ {
	// 		if line[i] == ' ' {
	// 			currentIndent++
	// 			startIndex = i + 1
	// 		} else if line[i] == '\t' {
	// 			currentIndent += 4 // Count tabs as 4 spaces
	// 			startIndex = i + 1
	// 		} else {
	// 			break
	// 		}
	// 	}

	// 	if startIndex < len(line) {
	// 		cleaned = append(cleaned, line[startIndex:])
	// 	} else {
	// 		cleaned = append(cleaned, "")
	// 	}
	// }

	return strings.Join(cleaned, "\n")
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
