package astparser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
	sitter "github.com/smacker/go-tree-sitter"
)

// ContextFinder orchestrates the process of gathering comprehensive context for code changes
type ContextFinder struct {
	provider       interfaces.CodeProvider
	astParser      *Parser
	symbolAnalyzer *SymbolAnalyzer
	diffParser     *DiffParser
	log            logze.Logger
}

// ChangeType represents the type of file change
type ChangeType string

const (
	ChangeTypeModified ChangeType = "MODIFIED"
	ChangeTypeAdded    ChangeType = "ADDED"
	ChangeTypeDeleted  ChangeType = "DELETED"
	ChangeTypeRenamed  ChangeType = "RENAMED"
)

// ContextBundle represents the final structured context for LLM
type ContextBundle struct {
	Files []FileContext `json:"files"`
}

// FileContext represents context for a single changed file
type FileContext struct {
	FilePath        string           `json:"file_path"`
	ChangeType      ChangeType       `json:"change_type"`
	DiffHunk        string           `json:"diff_hunk"`
	AffectedSymbols []AffectedSymbol `json:"affected_symbols"`
	RelatedFiles    []RelatedFile    `json:"related_files"`
	ConfigContext   *ConfigContext   `json:"config_context,omitempty"`
}

// RelatedFile represents a file related to the changed file
type RelatedFile struct {
	FilePath         string `json:"file_path"`
	Relationship     string `json:"relationship"` // "caller", "dependency", "test", "same_package"
	CodeSnippet      string `json:"code_snippet"`
	Line             int    `json:"line,omitempty"`
	RelevantFunction string `json:"relevant_function,omitempty"`
}

// ConfigContext represents context for configuration file changes
type ConfigContext struct {
	ConfigType       string        `json:"config_type"` // "yaml", "json", "env", etc.
	ChangedKeys      []string      `json:"changed_keys"`
	ConsumingCode    []RelatedFile `json:"consuming_code"`
	ImpactAssessment string        `json:"impact_assessment"`
}

// newContextFinder creates a new context finder
func NewContextFinder(provider interfaces.CodeProvider) *ContextFinder {
	return &ContextFinder{
		provider:       provider,
		astParser:      NewParser(),
		symbolAnalyzer: NewSymbolAnalyzer(provider),
		diffParser:     NewDiffParser(),
		log:            logze.With("component", "context_finder"),
	}
}

// GatherContext gathers comprehensive context for a merge request
func (cf *ContextFinder) GatherContext(ctx context.Context, request model.ReviewRequest, fileDiffs []*model.FileDiff) (*ContextBundle, error) {
	bundle := &ContextBundle{
		Files: make([]FileContext, 0, len(fileDiffs)),
	}

	// Process each changed file
	for _, fileDiff := range fileDiffs {
		fileContext, err := cf.processFileDiff(ctx, request, fileDiff)
		if err != nil {
			cf.log.Warn("failed to process file diff", "error", err, "file", fileDiff.NewPath)
			continue
		}

		if fileContext != nil {
			bundle.Files = append(bundle.Files, *fileContext)
		}
	}

	return bundle, nil
}

// processFileDiff processes a single file diff to extract context
func (cf *ContextFinder) processFileDiff(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*FileContext, error) {
	// Determine change type
	changeType := cf.determineChangeType(fileDiff)

	// Create base file context
	fileContext := &FileContext{
		FilePath:        fileDiff.NewPath,
		ChangeType:      changeType,
		DiffHunk:        fileDiff.Diff,
		AffectedSymbols: make([]AffectedSymbol, 0),
		RelatedFiles:    make([]RelatedFile, 0),
	}

	// Handle different change types
	switch changeType {
	case ChangeTypeAdded:
		return cf.processAddedFile(ctx, request, fileDiff, fileContext)
	case ChangeTypeDeleted:
		return cf.processDeletedFile(ctx, request, fileDiff, fileContext)
	case ChangeTypeModified, ChangeTypeRenamed:
		return cf.processModifiedFile(ctx, request, fileDiff, fileContext)
	}

	return fileContext, nil
}

// determineChangeType determines the type of change for a file
func (cf *ContextFinder) determineChangeType(fileDiff *model.FileDiff) ChangeType {
	if fileDiff.IsNew {
		return ChangeTypeAdded
	}
	if fileDiff.IsDeleted {
		return ChangeTypeDeleted
	}
	if fileDiff.IsRenamed {
		return ChangeTypeRenamed
	}
	return ChangeTypeModified
}

// processAddedFile processes a newly added file
func (cf *ContextFinder) processAddedFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	// For new files, the entire file is "affected"
	// Get the full file content
	content, err := cf.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
	if err != nil {
		// If we can't get the content, extract it from the diff
		content = cf.extractContentFromDiff(fileDiff.Diff)
	}

	// Find all symbols in the new file
	allSymbols, err := cf.findAllSymbolsInFile(fileDiff.NewPath, content)
	if err != nil {
		cf.log.Warn("failed to find symbols in new file", "error", err, "file", fileDiff.NewPath)
		return fileContext, nil
	}

	// For each symbol, find if it's already being used (cross-references within the same PR)
	for _, symbol := range allSymbols {
		// Analyze usage context
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.ProjectID, request.MergeRequest.SHA, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		// Add caller information to related files
		for _, caller := range usageContext.Callers {
			relatedFile := RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			}
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, relatedFile)
		}

		// Set usage context for the symbol
		symbol.Context.Package = cf.extractPackageFromPath(fileDiff.NewPath)
		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// processDeletedFile processes a deleted file
func (cf *ContextFinder) processDeletedFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	fileDiff.OldPath = lang.Check(fileDiff.OldPath, fileDiff.NewPath)

	// For deleted files, we need to get the content from the base branch
	content, err := cf.getBaseFileContent(ctx, request, fileDiff.OldPath)
	if err != nil {

		cf.log.Warn("failed to get base file content for deleted file", "error", err, "file", fileDiff.OldPath)
		return fileContext, nil
	}

	// Find all symbols that were in the deleted file
	allSymbols, err := cf.findAllSymbolsInFile(fileDiff.OldPath, content)
	if err != nil {
		cf.log.Warn("failed to find symbols in deleted file", "error", err, "file", fileDiff.OldPath)
		return fileContext, nil
	}

	// For each symbol, find where it's still being used (this indicates broken code)
	for _, symbol := range allSymbols {
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.ProjectID, request.MergeRequest.SHA, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage for deleted symbol", "error", err, "symbol", symbol.Name)
			continue
		}

		// Any callers found indicate potential issues
		for _, caller := range usageContext.Callers {
			relatedFile := RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "broken_caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			}
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, relatedFile)
		}

		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// processModifiedFile processes a modified file
func (cf *ContextFinder) processModifiedFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	// Check if this is a configuration file
	if cf.isConfigFile(fileDiff.NewPath) {
		return cf.processConfigFile(ctx, request, fileDiff, fileContext)
	}

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
		usageContext, err := cf.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.ProjectID, request.MergeRequest.SHA, symbol)
		if err != nil {
			cf.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		// Add caller information
		for _, caller := range usageContext.Callers {
			relatedFile := RelatedFile{
				FilePath:         caller.FilePath,
				Relationship:     "caller",
				CodeSnippet:      caller.CodeSnippet,
				Line:             caller.LineNumber,
				RelevantFunction: caller.FunctionName,
			}
			fileContext.RelatedFiles = append(fileContext.RelatedFiles, relatedFile)
		}

		// Add dependency information
		for _, dep := range usageContext.Dependencies {
			if dep.Source == "internal" && dep.FilePath != "" {
				relatedFile := RelatedFile{
					FilePath:         dep.FilePath,
					Relationship:     "dependency",
					RelevantFunction: dep.SymbolName,
				}
				fileContext.RelatedFiles = append(fileContext.RelatedFiles, relatedFile)
			}
		}

		fileContext.AffectedSymbols = append(fileContext.AffectedSymbols, symbol)
	}

	return fileContext, nil
}

// processConfigFile processes changes in configuration files
func (cf *ContextFinder) processConfigFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, fileContext *FileContext) (*FileContext, error) {
	// Extract changed configuration keys
	changedKeys := cf.extractChangedConfigKeys(fileDiff.Diff)

	// Find code that consumes this configuration
	consumers, err := cf.symbolAnalyzer.findConfigFileConsumers(ctx, request.ProjectID, request.MergeRequest.SHA, fileDiff.NewPath)
	if err != nil {
		cf.log.Warn("failed to find config file consumers", "error", err, "file", fileDiff.NewPath)
	}

	// Create config context
	configContext := &ConfigContext{
		ConfigType:       cf.detectConfigType(fileDiff.NewPath),
		ChangedKeys:      changedKeys,
		ConsumingCode:    make([]RelatedFile, 0),
		ImpactAssessment: cf.assessConfigImpact(changedKeys),
	}

	// Add consuming code as related files
	for _, consumer := range consumers {
		relatedFile := RelatedFile{
			FilePath:     consumer,
			Relationship: "config_consumer",
		}
		configContext.ConsumingCode = append(configContext.ConsumingCode, relatedFile)
		fileContext.RelatedFiles = append(fileContext.RelatedFiles, relatedFile)
	}

	fileContext.ConfigContext = configContext

	return fileContext, nil
}

// Helper methods

// extractContentFromDiff extracts file content from diff (for new files)
func (cf *ContextFinder) extractContentFromDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var content []string

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			content = append(content, line[1:]) // Remove + prefix
		}
	}

	return strings.Join(content, "\n")
}

// findAllSymbolsInFile finds all symbols in a file
func (cf *ContextFinder) findAllSymbolsInFile(filePath, content string) ([]AffectedSymbol, error) {
	rootNode, err := cf.astParser.ParseFileToAST(context.Background(), filePath, content)
	if err != nil {
		return nil, err
	}

	var symbols []AffectedSymbol
	cf.walkASTForSymbols(rootNode, filePath, content, &symbols)

	return symbols, nil
}

// walkASTForSymbols walks the AST to find all symbol definitions
func (cf *ContextFinder) walkASTForSymbols(node *sitter.Node, filePath, content string, symbols *[]AffectedSymbol) {
	if cf.astParser.IsSymbolNode(node.Type()) {
		symbol := cf.astParser.ExtractSymbolFromNode(node, filePath, content)
		if symbol.Name != "" {
			*symbols = append(*symbols, symbol)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			cf.walkASTForSymbols(child, filePath, content, symbols)
		}
	}
}

// getBaseFileContent gets the file content from the base branch
func (cf *ContextFinder) getBaseFileContent(ctx context.Context, request model.ReviewRequest, filePath string) (string, error) {
	// This would ideally get the content from the base branch
	// For now, we'll try to get it from the current SHA as a fallback
	return cf.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.SHA)
}

// isConfigFile checks if a file is a configuration file
func (cf *ContextFinder) isConfigFile(filePath string) bool {
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

// extractChangedConfigKeys extracts configuration keys that were changed
func (cf *ContextFinder) extractChangedConfigKeys(diff string) []string {
	var keys []string
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			// Simple key extraction (this could be enhanced with proper YAML/JSON parsing)
			key := cf.extractKeyFromConfigLine(line[1:]) // Remove +/- prefix
			if key != "" {
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// extractKeyFromConfigLine extracts a configuration key from a config file line
func (cf *ContextFinder) extractKeyFromConfigLine(line string) string {
	line = strings.TrimSpace(line)

	// YAML style: key: value
	if colonIndex := strings.Index(line, ":"); colonIndex != -1 {
		key := strings.TrimSpace(line[:colonIndex])
		// Remove any leading dashes (YAML list items)
		key = strings.TrimLeft(key, "- ")
		return key
	}

	// JSON style: "key": value
	if strings.Contains(line, "\":") {
		parts := strings.Split(line, "\":")
		if len(parts) > 0 {
			key := strings.Trim(parts[0], "\"")
			key = strings.TrimSpace(key)
			return key
		}
	}

	return ""
}

// detectConfigType detects the type of configuration file
func (cf *ContextFinder) detectConfigType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	filename := strings.ToLower(filepath.Base(filePath))

	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".ini":
		return "ini"
	case ".env":
		return "env"
	default:
		// Check specific filenames
		if filename == "makefile" {
			return "makefile"
		}
		if filename == "dockerfile" {
			return "dockerfile"
		}
		return "unknown"
	}
}

// assessConfigImpact provides an impact assessment for configuration changes
func (cf *ContextFinder) assessConfigImpact(changedKeys []string) string {
	if len(changedKeys) == 0 {
		return "No configuration keys changed"
	}

	// Simple impact assessment based on key names
	highImpactKeys := []string{"database", "db", "password", "secret", "api_key", "token", "host", "port", "url"}
	mediumImpactKeys := []string{"timeout", "retry", "cache", "log", "debug", "feature", "flag"}

	highImpactCount := 0
	mediumImpactCount := 0

	for _, key := range changedKeys {
		lowerKey := strings.ToLower(key)

		isHighImpact := false
		for _, highKey := range highImpactKeys {
			if strings.Contains(lowerKey, highKey) {
				highImpactCount++
				isHighImpact = true
				break
			}
		}

		if !isHighImpact {
			for _, mediumKey := range mediumImpactKeys {
				if strings.Contains(lowerKey, mediumKey) {
					mediumImpactCount++
					break
				}
			}
		}
	}

	if highImpactCount > 0 {
		return fmt.Sprintf("High impact: %d critical configuration keys changed", highImpactCount)
	} else if mediumImpactCount > 0 {
		return fmt.Sprintf("Medium impact: %d configuration keys changed", mediumImpactCount)
	} else {
		return fmt.Sprintf("Low impact: %d configuration keys changed", len(changedKeys))
	}
}

// extractPackageFromPath extracts package/module name from file path
func (cf *ContextFinder) extractPackageFromPath(filePath string) string {
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
