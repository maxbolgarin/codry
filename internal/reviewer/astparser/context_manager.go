package astparser

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

// ContextManager orchestrates the process of gathering comprehensive context for code changes
type ContextManager struct {
	provider       interfaces.CodeProvider
	astParser      *Parser
	symbolAnalyzer *ExternalRefsAnalyzer
	diffParser     *DiffParser
	log            logze.Logger
}

// newContextFinder creates a new context finder
func NewContextFinder(provider interfaces.CodeProvider) *ContextManager {
	return &ContextManager{
		provider:       provider,
		astParser:      NewParser(),
		symbolAnalyzer: NewExternalRefsAnalyzer(provider),
		diffParser:     NewDiffParser(),
		log:            logze.With("component", "context_finder"),
	}
}

type ContextRequest struct {
	ProjectID    string
	MergeRequest *model.MergeRequest
	FileDiffs    []*model.FileDiff
	RepoDataHead *model.RepositorySnapshot
	RepoDataBase *model.RepositorySnapshot
}

// GatherContext gathers comprehensive context for a merge request
func (cf *ContextManager) GatherFilesContext(ctx context.Context, request ContextRequest) ([]*FileContext, error) {

	files := make([]*FileContext, 0, len(request.FileDiffs))
	// Process each changed file
	for _, fileDiff := range request.FileDiffs {
		fileContext, err := cf.processFileDiff(ctx, request, fileDiff)
		if err != nil {
			cf.log.Warn("failed to process file diff", "error", err, "file", fileDiff.NewPath)
			continue
		}

		if fileContext != nil {
			files = append(files, fileContext)
		}
	}

	return files, nil
}

// processFileDiff processes a single file diff to extract context
func (cf *ContextManager) processFileDiff(ctx context.Context, request ContextRequest, fileDiff *model.FileDiff) (*FileContext, error) {
	// Create base file context
	fileContext := &FileContext{
		FilePath:        fileDiff.NewPath,
		ChangeType:      cf.determineChangeType(fileDiff),
		Diff:            fileDiff.Diff,
		AffectedSymbols: make([]AffectedSymbol, 0),
		RelatedFiles:    make([]RelatedFile, 0),
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
		return cf.processDeletedFile(ctx, request, fileDiff, fileContext)

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

		// Add caller information to related files
		for _, caller := range usageContext.Callers {
			symbol.Callers = append(symbol.Callers, Dependency{
				Name: caller.FunctionName,
				Line: caller.LineNumber,
				Type: SymbolTypeFunction,
				Code: lang.Check(caller.CodeSnippet, caller.Code),
			})
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			})
		}

		depMap := make(map[string]Dependency)
		for _, dep := range usageContext.Dependencies {
			depMap[dep.SymbolName] = Dependency{
				Name: dep.SymbolName,
				Code: dep.Code,
			}
		}

		for i := range symbol.Dependencies {
			symbol.Dependencies[i].Code = depMap[symbol.Dependencies[i].Name].Code
		}

		// Set usage context for the symbol
		symbol.Context.Package = cf.extractPackageFromPath(fileDiff.NewPath)
		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// processDeletedFile processes a deleted file
func (cf *ContextManager) processDeletedFile(ctx context.Context, request ContextRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	fileDiff.OldPath = lang.Check(fileDiff.OldPath, fileDiff.NewPath)

	// For deleted files, we need to get the content from the base branch
	content := ""
	for _, file := range request.RepoDataBase.Files {
		if file.Path == fileDiff.OldPath {
			content = file.Content
			break
		}
	}
	if content == "" {
		cf.log.Warn("failed to get base file content for deleted file", "file", fileDiff.OldPath)
		return fileContext, nil
	}

	// Find all symbols that were in the deleted file
	allSymbols, err := cf.astParser.FindAllSymbolsInFile(ctx, fileDiff.OldPath, content)
	if err != nil {
		cf.log.Warn("failed to find symbols in deleted file", "error", err, "file", fileDiff.OldPath)
		return fileContext, nil
	}

	// For each symbol, find where it's still being used (this indicates broken code)
	for _, symbol := range allSymbols {
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.RepoDataBase, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage for deleted symbol", "error", err, "symbol", symbol.Name)
			continue
		}

		// Add caller information to related files
		for _, caller := range usageContext.Callers {
			symbol.Callers = append(symbol.Callers, Dependency{
				Name: caller.FunctionName,
				Line: caller.LineNumber,
				Type: SymbolTypeFunction,
				Code: lang.Check(caller.CodeSnippet, caller.Code),
			})
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			})
		}

		depMap := make(map[string]Dependency)
		for _, dep := range usageContext.Dependencies {
			depMap[dep.SymbolName] = Dependency{
				Name: dep.SymbolName,
				Code: dep.Code,
			}
		}

		for i := range symbol.Dependencies {
			symbol.Dependencies[i].Code = depMap[symbol.Dependencies[i].Name].Code
		}

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
		return nil, errm.Wrap(err, "failed to parse diff lines")
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
		return nil, errm.Wrap(err, "failed to get current file content")
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

		// Add caller information to related files
		for _, caller := range usageContext.Callers {
			symbol.Callers = append(symbol.Callers, Dependency{
				Name: caller.FunctionName,
				Line: caller.LineNumber,
				Type: SymbolTypeFunction,
				Code: lang.Check(caller.CodeSnippet, caller.Code),
			})
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			})
		}

		depMap := make(map[string]Dependency)
		for _, dep := range usageContext.Dependencies {
			depMap[dep.SymbolName] = Dependency{
				Name: dep.SymbolName,
				Code: dep.Code,
			}
		}

		for i := range symbol.Dependencies {
			symbol.Dependencies[i].Code = depMap[symbol.Dependencies[i].Name].Code
		}

		// Set usage context for the symbol
		symbol.Context.Package = cf.extractPackageFromPath(fileDiff.NewPath)

		// Add dependency information
		for _, dep := range usageContext.Dependencies {
			if dep.SourceFile == "internal" && dep.SymbolName != "" {
				fileContext.RelatedFiles = append(fileContext.RelatedFiles, RelatedFile{
					FilePath:         dep.SourceFile,
					Relationship:     "dependency",
					RelevantFunction: dep.SymbolName,
				})
			}
		}

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
