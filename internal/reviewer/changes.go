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
	bundle.log.DebugIf(s.cfg.Verbose, "generating code review")

	commentsCreated, err := s.reviewCodeChanges(ctx, bundle.request, bundle.filesToReview, bundle.log)
	if err != nil {
		msg := "failed to generate code review"
		bundle.log.Error(msg, "error", err)
		bundle.result.Errors = append(bundle.result.Errors, errm.Wrap(err, msg))
		return
	}

	bundle.log.InfoIf(s.cfg.Verbose, "finished code review")

	bundle.result.CommentsCreated = commentsCreated
	bundle.result.IsCodeReviewCreated = true
}

// reviewCodeChanges reviews individual files and creates comments
func (s *Reviewer) reviewCodeChanges(ctx context.Context, request model.ReviewRequest, changes []*model.FileDiff, log logze.Logger) (int, error) {

	commentsCreated := 0

	errs := errm.NewList()
	for _, change := range changes {
		// Guard old path
		change.OldPath = lang.Check(change.OldPath, change.NewPath)

		fileHash := s.getFileHash(change.Diff)
		if oldHash, ok := s.processedMRs.Lookup(request.String(), change.NewPath); ok {
			if oldHash == fileHash {
				log.DebugIf(s.cfg.Verbose, "skipping already reviewed", "file", change.NewPath)
				continue
			}
		}

		log.DebugIf(s.cfg.Verbose, "performing review", "file", change.NewPath)

		// Gather enhanced context for the file
		enhancedCtx, err := s.contextGatherer.GatherEnhancedContext(ctx, request, change)
		if err != nil {
			log.Warn("failed to gather enhanced context, falling back to basic review", "error", err, "file", change.NewPath)
			// Fallback to basic review
			reviewResult, err := s.performBasicReview(ctx, request, change, log)
			if err != nil {
				errs.Wrap(err, "failed to perform basic review", "file", change.NewPath)
				continue
			}
			commentsCreated += s.processReviewResults(ctx, request, change, reviewResult, log)
			continue
		}

		// Generate clean diff
		cleanDiff, err := s.parser.GenerateCleanDiff(change.Diff)
		if err != nil {
			errs.Wrap(err, "failed to generate clean diff", "file", change.NewPath)
			log.Error("failed to generate clean diff", "error", err, "file", change.NewPath)
			continue
		}

		enhancedCtx.CleanDiff = cleanDiff

		// Perform enhanced review with rich context
		promptsCtx := s.convertToPromptsContext(enhancedCtx)
		reviewResult, err := s.agent.ReviewCodeWithContext(ctx, change.NewPath, promptsCtx)
		if err != nil {
			errs.Wrap(err, "failed to review code change with enhanced context", "file", change.NewPath)
			log.Error("failed to review code change with enhanced context", "error", err, "file", change.NewPath)
			continue
		}

		// Skip if no issues found
		if reviewResult == nil || !reviewResult.HasIssues || len(reviewResult.Comments) == 0 {
			log.Debug("no significant issues found in file after enhanced analysis", "file", change.NewPath)
			s.processedMRs.Set(request.String(), change.NewPath, fileHash)
			continue
		}

		commentsCreated += s.processReviewResults(ctx, request, change, reviewResult, log)
		s.processedMRs.Set(request.String(), change.NewPath, fileHash)
		log.Info("file reviewed with enhanced context analysis", "file", change.NewPath, "comments", len(reviewResult.Comments))
	}

	return commentsCreated, errs.Err()
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

	// Get enhanced context for quality scoring
	// enhancedCtx, err := s.contextGatherer.GatherEnhancedContext(ctx, request, change)
	// if err != nil {
	// 	log.Warn("failed to gather context for quality scoring", "error", err)
	// 	enhancedCtx = &EnhancedContext{} // Use empty context as fallback
	// }

	// Apply quality scoring and filtering
	originalCount := len(reviewResult.Comments)
	//reviewResult.Comments = s.qualityScorer.ScoreAndFilterComments(reviewResult.Comments, enhancedCtx)
	filteredCount := len(reviewResult.Comments)

	if originalCount > filteredCount {
		log.Info("quality scorer filtered comments",
			"original", originalCount,
			"filtered", filteredCount,
			"removed", originalCount-filteredCount)
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

		err := s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
		if err != nil {
			log.Error("failed to create comment", "error", err, "file", change.NewPath, "line", reviewComment.Line)
			continue
		}

		commentsCreated++
		log.Info("created high-quality comment",
			"file", change.NewPath,
			"line", reviewComment.Line,
			"type", reviewComment.IssueType,
			"priority", reviewComment.Priority,
			"confidence", reviewComment.Confidence)
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

// convertToPromptsContext converts reviewer.EnhancedContext to prompts.EnhancedContext
func (s *Reviewer) convertToPromptsContext(ctx *EnhancedContext) *prompts.EnhancedContext {
	return &prompts.EnhancedContext{
		FilePath:         ctx.FilePath,
		FileContent:      ctx.FileContent,
		CleanDiff:        ctx.CleanDiff,
		ImportedPackages: ctx.ImportedPackages,
		RelatedFiles: func() []prompts.RelatedFile {
			var converted []prompts.RelatedFile
			for _, rf := range ctx.RelatedFiles {
				converted = append(converted, prompts.RelatedFile{
					Path:         rf.Path,
					Relationship: rf.Relationship,
					Snippet:      rf.Snippet,
				})
			}
			return converted
		}(),
		FunctionSignatures: func() []prompts.FunctionSignature {
			var converted []prompts.FunctionSignature
			for _, fs := range ctx.FunctionSignatures {
				converted = append(converted, prompts.FunctionSignature{
					Name:       fs.Name,
					Parameters: fs.Parameters,
					Returns:    fs.Returns,
					IsExported: fs.IsExported,
					LineNumber: fs.LineNumber,
				})
			}
			return converted
		}(),
		TypeDefinitions: func() []prompts.TypeDefinition {
			var converted []prompts.TypeDefinition
			for _, td := range ctx.TypeDefinitions {
				converted = append(converted, prompts.TypeDefinition{
					Name:       td.Name,
					Type:       td.Type,
					Fields:     td.Fields,
					Methods:    td.Methods,
					IsExported: td.IsExported,
					LineNumber: td.LineNumber,
				})
			}
			return converted
		}(),
		UsagePatterns: func() []prompts.UsagePattern {
			var converted []prompts.UsagePattern
			for _, up := range ctx.UsagePatterns {
				converted = append(converted, prompts.UsagePattern{
					Pattern:      up.Pattern,
					Description:  up.Description,
					Examples:     up.Examples,
					BestPractice: up.BestPractice,
				})
			}
			return converted
		}(),
		SecurityContext: prompts.SecurityContext{
			HasAuthenticationLogic:  ctx.SecurityContext.HasAuthenticationLogic,
			HasInputValidation:      ctx.SecurityContext.HasInputValidation,
			HandlesUserInput:        ctx.SecurityContext.HandlesUserInput,
			AccessesDatabase:        ctx.SecurityContext.AccessesDatabase,
			HandlesFileOperations:   ctx.SecurityContext.HandlesFileOperations,
			NetworkOperations:       ctx.SecurityContext.NetworkOperations,
			CryptographicOperations: ctx.SecurityContext.CryptographicOperations,
		},
		SemanticChanges: func() []prompts.SemanticChange {
			var converted []prompts.SemanticChange
			for _, sc := range ctx.SemanticChanges {
				converted = append(converted, prompts.SemanticChange{
					Type:        sc.Type,
					Impact:      sc.Impact,
					Description: sc.Description,
					Lines:       sc.Lines,
					Context:     sc.Context,
				})
			}
			return converted
		}(),
	}
}
