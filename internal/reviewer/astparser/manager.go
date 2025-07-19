package astparser

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/logze/v2"
)

// ContextManager orchestrates the process of gathering comprehensive context for code changes
type ContextManager struct {
	provider       interfaces.CodeProvider
	astParser      *ASTParser
	symbolAnalyzer *Analyzer
	diffParser     *DiffParser
	log            logze.Logger
}

// newContextFinder creates a new context finder
func NewContextFinder(provider interfaces.CodeProvider) *ContextManager {
	return &ContextManager{
		provider:       provider,
		astParser:      NewParser(),
		symbolAnalyzer: NewAnalyzer(provider),
		diffParser:     NewDiffParser(),
		log:            logze.With("component", "context_finder"),
	}
}

type ContextRequest struct {
	ProjectID      string
	MergeRequest   *model.MergeRequest
	FilesForReview []*model.FileDiff
	RepoDataHead   *model.RepositorySnapshot
	RepoDataBase   *model.RepositorySnapshot
}

// GatherContext gathers comprehensive context for a merge request
func (cf *ContextManager) GatherFilesContext(ctx context.Context, request ContextRequest) ([]*FileContext, error) {

	files := make([]*FileContext, 0, len(request.FilesForReview))
	// Process each changed file
	for _, fileDiff := range request.FilesForReview {
		fileContext, err := cf.processFileDiff(ctx, request, fileDiff)
		if err != nil {
			cf.log.Warn("failed to process file diff", "error", err, "file", fileDiff.NewPath)
		}
		if fileContext == nil {
			fileContext = &FileContext{
				FilePath:   fileDiff.NewPath,
				ChangeType: cf.determineChangeType(fileDiff),
			}
		}
		files = append(files, fileContext)
	}

	return files, nil
}

// processFileDiff processes a single file diff to extract context
func (cf *ContextManager) processFileDiff(ctx context.Context, request ContextRequest, fileDiff *model.FileDiff) (*FileContext, error) {
	// Create base file context
	fileContext := &FileContext{
		FilePath:        fileDiff.NewPath,
		ChangeType:      cf.determineChangeType(fileDiff),
		AffectedSymbols: make([]AffectedSymbol, 0),
	}

	if cf.isConfigFile(fileDiff.NewPath) {
		// TODO: add config file processing
		cf.log.Warn("config file detected, skipping", "file", fileDiff.NewPath)
		return fileContext, nil
	}

	// Handle different change types
	switch fileContext.ChangeType {
	case ChangeTypeAdded:
		return cf.processAddedFile(ctx, request, fileDiff, fileContext)

	case ChangeTypeDeleted:
		// TODO: add deleted?
		cf.log.Warn("deleted file detected, skipping", "file", fileDiff.NewPath)
		return fileContext, nil

	case ChangeTypeModified, ChangeTypeRenamed:
		return cf.processModifiedFile(ctx, request, fileDiff, fileContext)

	}

	return fileContext, nil
}

// processAddedFile processes a newly added file
func (cf *ContextManager) processAddedFile(ctx context.Context, request ContextRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	// For new files, the entire file is "affected"
	// Get the full file content
	var content string
	for _, file := range request.RepoDataHead.Files {
		if file.Path == fileDiff.NewPath {
			content = file.Content
			break
		}
	}
	if content == "" {
		// If we can't get the content, extract it from the diff
		content = cf.extractContentFromDiff(fileDiff.Diff)
	}

	// Find all symbols in the new file
	allSymbols, err := cf.astParser.FindAllSymbolsInFile(ctx, fileDiff.NewPath, content)
	if err != nil {
		cf.log.Warn("failed to find symbols in new file", "error", err, "file", fileDiff.NewPath)
		return fileContext, nil
	}

	// For each symbol, find if it's already being used
	for _, symbol := range allSymbols {
		// Analyze usage context
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.RepoDataHead, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		symbol.Callers = usageContext.Callers
		symbol.Dependencies = usageContext.Dependencies

		// Set usage context for the symbol
		symbol.Context.Package = cf.extractPackageFromPath(fileDiff.NewPath)
		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// processModifiedFile processes a modified file
func (cf *ContextManager) processModifiedFile(ctx context.Context, request ContextRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {

	// Parse the diff to get changed lines
	diffLines, err := cf.diffParser.ParseDiffToLines(fileDiff.Diff)
	if err != nil {
		return nil, erro.Wrap(err, "failed to parse diff lines")
	}

	// Extract line numbers for added and modified lines
	var changedLines []int
	for _, line := range diffLines {
		if line.Type == DiffAddedLine && line.NewLine > 0 {
			changedLines = append(changedLines, line.NewLine)
		}
	}

	if len(changedLines) == 0 {
		return fileContext, nil
	}

	// Get the current file content (head version)
	content, err := cf.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get current file content")
	}

	// Find affected symbols
	affectedSymbols, err := cf.astParser.FindAffectedSymbols(ctx, fileDiff.NewPath, content, changedLines)
	if err != nil {
		cf.log.Warn("failed to find affected symbols, using fallback", "error", err, "file", fileDiff.NewPath)
		// Continue without symbol analysis
		return fileContext, nil
	}

	// For each affected symbol, gather comprehensive context
	for _, symbol := range affectedSymbols {
		// Analyze symbol usage
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.RepoDataHead, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		symbol.Callers = usageContext.Callers
		symbol.Dependencies = usageContext.Dependencies

		// Set usage context for the symbol
		symbol.Context.Package = cf.extractPackageFromPath(fileDiff.NewPath)
		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// Helper methods

// extractContentFromDiff extracts file content from diff (for new files)
func (cf *ContextManager) extractContentFromDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var content []string

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			content = append(content, line[1:]) // Remove + prefix
		}
	}

	return strings.Join(content, "\n")
}

// isConfigFile checks if a file is a configuration file
func (cf *ContextManager) isConfigFile(filePath string) bool {
	configExtensions := []string{".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".config", ".env"}
	configFiles := []string{"makefile", "dockerfile", "docker-compose.yml", "go.mod", "package.json", "requirements.txt"}

	ext := strings.ToLower(filepath.Ext(filePath))
	filename := strings.ToLower(filepath.Base(filePath))

	// Check extensions
	for _, configExt := range configExtensions {
		if ext == configExt {
			return true
		}
	}

	// Check specific filenames
	for _, configFile := range configFiles {
		if filename == configFile {
			return true
		}
	}

	return false
}

// extractPackageFromPath extracts package/module name from file path
func (cf *ContextManager) extractPackageFromPath(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "." {
		return "main"
	}

	parts := strings.Split(dir, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "unknown"
}
