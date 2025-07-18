package llmcontext

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"

	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Builder builds comprehensive context bundles for LLM analysis
type Builder struct {
	provider       interfaces.CodeProvider
	contextFinder  *astparser.ContextManager
	symbolAnalyzer *astparser.ExternalRefsAnalyzer
	diffParser     *astparser.DiffParser
	astParser      *astparser.Parser
	log            logze.Logger
	isVerbose      bool

	repoDataProvider *repoDataProvider
}

// NewBuilder creates a new context bundle builder
func NewBuilder(provider interfaces.CodeProvider, isVerbose bool) *Builder {
	return &Builder{
		provider:         provider,
		contextFinder:    astparser.NewContextFinder(provider),
		symbolAnalyzer:   astparser.NewExternalRefsAnalyzer(provider),
		diffParser:       astparser.NewDiffParser(),
		astParser:        astparser.NewParser(),
		log:              logze.With("component", "context_bundle_builder"),
		isVerbose:        isVerbose,
		repoDataProvider: newRepoDataProvider(provider, isVerbose),
	}
}

// BuildContext builds a comprehensive context bundle for LLM analysis
func (cbb *Builder) BuildContext(ctx context.Context, projectID string, mrIID int) (*ContextBundle, error) {
	cbb.log.DebugIf(cbb.isVerbose, "loading repository data")

	err := cbb.repoDataProvider.loadData(ctx, projectID, mrIID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to load repository data")
	}

	cbb.log.DebugIf(cbb.isVerbose, "loaded all data for context gathering")

	mrContext, err := gatherMRContext(projectID, cbb.repoDataProvider)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get MR context")
	}

	cbb.log.DebugIf(cbb.isVerbose, "gathered MR context")

	contextRequest := astparser.ContextRequest{
		ProjectID:    projectID,
		MergeRequest: cbb.repoDataProvider.mr,
		FileDiffs:    cbb.repoDataProvider.diffs,
		RepoDataHead: cbb.repoDataProvider.repoDataHead,
		RepoDataBase: cbb.repoDataProvider.repoDataBase,
	}

	// Gather basic context using ContextFinder
	filesContext, err := cbb.contextFinder.GatherFilesContext(ctx, contextRequest)
	if err != nil {
		return nil, errm.Wrap(err, "failed to gather basic context")
	}

	cbb.log.DebugIf(cbb.isVerbose, "gathered basic context")

	// Enhance with detailed analysis
	enhancedFiles, err := cbb.enhanceFileContexts(ctx, contextRequest, filesContext)
	if err != nil {
		cbb.log.Warn("failed to enhance file contexts", "error", err)
		// Continue with basic context
		enhancedFiles = filesContext
	}

	cbb.log.DebugIf(cbb.isVerbose, "enhanced file contexts")

	bundle := &ContextBundle{
		Files:     enhancedFiles,
		MRContext: mrContext,
	}

	cbb.log.DebugIf(cbb.isVerbose, "built context bundle")
	time.Sleep(time.Second)

	return bundle, nil
}

// enhanceFileContexts enhances file contexts with detailed symbol analysis
func (cbb *Builder) enhanceFileContexts(ctx context.Context, request astparser.ContextRequest, files []*astparser.FileContext) ([]*astparser.FileContext, error) {
	var enhancedFiles []*astparser.FileContext

	for _, fileContext := range files {
		enhanced, err := cbb.enhanceFileContext(ctx, request, *fileContext)
		if err != nil {
			cbb.log.Warn("failed to enhance file context", "error", err, "file", fileContext.FilePath)
			// Continue with original context
			enhancedFiles = append(enhancedFiles, fileContext)
			continue
		}
		enhancedFiles = append(enhancedFiles, enhanced)
	}

	return enhancedFiles, nil
}

// enhanceFileContext enhances a single file context with detailed analysis
func (cbb *Builder) enhanceFileContext(ctx context.Context, request astparser.ContextRequest, fileContext astparser.FileContext) (*astparser.FileContext, error) {
	enhanced := fileContext

	// Get file content for detailed analysis
	var content string
	var err error

	if fileContext.ChangeType != astparser.ChangeTypeDeleted {
		content, err = cbb.provider.GetFileContent(ctx, request.ProjectID, fileContext.FilePath, request.MergeRequest.SHA)
		if err != nil {
			cbb.log.Warn("failed to get file content", "error", err, "file", fileContext.FilePath)
		}
	}

	// Perform diff impact analysis
	if content != "" && fileContext.Diff != "" {
		impact, err := cbb.diffParser.AnalyzeDiffImpact(fileContext.Diff, fileContext.FilePath, content, cbb.astParser)
		if err == nil {
			// Add impact information to existing symbols or create new ones
			enhanced.AffectedSymbols = cbb.mergeSymbolInformation(enhanced.AffectedSymbols, impact.AffectedSymbols)
		}
	}

	// Enhance symbol information with usage context
	for i, symbol := range enhanced.AffectedSymbols {
		usageContext, err := cbb.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.RepoDataHead, symbol)
		if err != nil {
			cbb.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		// Convert usage context to related files
		enhanced.RelatedFiles = append(enhanced.RelatedFiles, cbb.convertUsageToRelatedFiles(usageContext)...)

		// Update symbol with enhanced context information
		enhanced.AffectedSymbols[i] = cbb.enhanceSymbolWithContext(symbol)
	}

	return &enhanced, nil
}

// mergeSymbolInformation merges symbol information from different sources
func (cbb *Builder) mergeSymbolInformation(existing []astparser.AffectedSymbol, additional []astparser.AffectedSymbol) []astparser.AffectedSymbol {
	symbolMap := make(map[string]astparser.AffectedSymbol)

	// Add existing symbols
	for _, symbol := range existing {
		key := cbb.getSymbolKey(symbol)
		symbolMap[key] = symbol
	}

	// Merge or add additional symbols
	for _, symbol := range additional {
		key := cbb.getSymbolKey(symbol)
		if existingSymbol, exists := symbolMap[key]; exists {
			// Merge information
			merged := cbb.mergeSymbols(existingSymbol, symbol)
			symbolMap[key] = merged
		} else {
			symbolMap[key] = symbol
		}
	}

	// Convert back to slice
	var result []astparser.AffectedSymbol
	for _, symbol := range symbolMap {
		result = append(result, symbol)
	}

	return result
}

// getSymbolKey generates a unique key for a symbol
func (cbb *Builder) getSymbolKey(symbol astparser.AffectedSymbol) string {
	return fmt.Sprintf("%s:%s:%d:%d", symbol.FilePath, symbol.Name, symbol.StartLine, symbol.EndLine)
}

// mergeSymbols merges two symbol objects
func (cbb *Builder) mergeSymbols(symbol1, symbol2 astparser.AffectedSymbol) astparser.AffectedSymbol {
	merged := symbol1

	// Use the most detailed information
	if symbol2.FullCode != "" && len(symbol2.FullCode) > len(symbol1.FullCode) {
		merged.FullCode = symbol2.FullCode
	}

	if symbol2.DocComment != "" && len(symbol2.DocComment) > len(symbol1.DocComment) {
		merged.DocComment = symbol2.DocComment
	}

	// Merge dependencies
	depMap := make(map[string]astparser.Dependency)
	for _, dep := range symbol1.Dependencies {
		depMap[dep.Name] = dep
	}
	for _, dep := range symbol2.Dependencies {
		depMap[dep.Name] = dep
	}

	merged.Dependencies = make([]astparser.Dependency, 0, len(depMap))
	for _, dep := range depMap {
		merged.Dependencies = append(merged.Dependencies, dep)
	}

	return merged
}

// convertUsageToRelatedFiles converts symbol usage context to related files
func (cbb *Builder) convertUsageToRelatedFiles(usage astparser.SymbolUsageContext) []astparser.RelatedFile {
	var relatedFiles []astparser.RelatedFile

	// Add callers
	for _, caller := range usage.Callers {
		relatedFile := astparser.RelatedFile{
			FilePath:         caller.FilePath,
			Relationship:     "caller",
			CodeSnippet:      caller.CodeSnippet,
			Line:             caller.LineNumber,
			RelevantFunction: caller.FunctionName,
		}
		relatedFiles = append(relatedFiles, relatedFile)
	}

	// Add dependencies (only internal ones)
	for _, dep := range usage.Dependencies {
		if dep.SourceFile == "internal" && dep.SymbolName != "" {
			relatedFile := astparser.RelatedFile{
				FilePath:         dep.SourceFile,
				Relationship:     "dependency",
				RelevantFunction: dep.SymbolName,
			}
			relatedFiles = append(relatedFiles, relatedFile)
		}
	}

	return relatedFiles
}

// enhanceSymbolWithContext enhances a symbol with usage context information
func (cbb *Builder) enhanceSymbolWithContext(symbol astparser.AffectedSymbol) astparser.AffectedSymbol {
	enhanced := symbol

	// Update context information
	enhanced.Context.Package = cbb.extractPackageFromPath(symbol.FilePath)

	// Set caller count and dependency information in a more structured way
	// This could be extended to include more detailed usage statistics

	return enhanced
}

// extractPackageFromPath extracts package name from file path
func (cbb *Builder) extractPackageFromPath(filePath string) string {
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
