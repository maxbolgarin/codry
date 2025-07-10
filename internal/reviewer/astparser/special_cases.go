package astparser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// SpecialCasesHandler handles special cases in code analysis
type SpecialCasesHandler struct {
	provider       interfaces.CodeProvider
	astParser      *Parser
	symbolAnalyzer *ExternalRefsAnalyzer
	diffParser     *DiffParser
	log            logze.Logger
}

// SpecialCaseType represents different types of special cases
type SpecialCaseType string

const (
	SpecialCaseNewFile           SpecialCaseType = "new_file"
	SpecialCaseDeletedFile       SpecialCaseType = "deleted_file"
	SpecialCaseConfigFile        SpecialCaseType = "config_file"
	SpecialCaseBinaryFile        SpecialCaseType = "binary_file"
	SpecialCaseGeneratedFile     SpecialCaseType = "generated_file"
	SpecialCaseTestFile          SpecialCaseType = "test_file"
	SpecialCaseMigrationFile     SpecialCaseType = "migration_file"
	SpecialCaseDocumentationFile SpecialCaseType = "documentation_file"
)

// SpecialCaseAnalysis represents analysis results for special cases
type SpecialCaseAnalysis struct {
	CaseType          SpecialCaseType   `json:"case_type"`
	FilePath          string            `json:"file_path"`
	Analysis          string            `json:"analysis"`
	Impact            string            `json:"impact"`
	Recommendations   []string          `json:"recommendations"`
	RelatedFiles      []string          `json:"related_files"`
	PotentialIssues   []string          `json:"potential_issues"`
	RequiresAttention bool              `json:"requires_attention"`
	ContextualInfo    map[string]string `json:"contextual_info"`
}

// NewFileAnalysis represents analysis specific to new files
type NewFileAnalysis struct {
	SpecialCaseAnalysis
	ExportedSymbols    []AffectedSymbol `json:"exported_symbols"`
	Dependencies       []string         `json:"dependencies"`
	PotentialConsumers []string         `json:"potential_consumers"`
}

// DeletedFileAnalysis represents analysis specific to deleted files
type DeletedFileAnalysis struct {
	SpecialCaseAnalysis
	DeletedSymbols    []AffectedSymbol  `json:"deleted_symbols"`
	BrokenReferences  []BrokenReference `json:"broken_references"`
	MigrationRequired bool              `json:"migration_required"`
}

// ConfigFileAnalysis represents analysis specific to configuration files
type ConfigFileAnalysis struct {
	SpecialCaseAnalysis
	ConfigFormat        string               `json:"config_format"`
	ChangedSettings     []ConfigSetting      `json:"changed_settings"`
	AffectedComponents  []ComponentReference `json:"affected_components"`
	BackwardsCompatible bool                 `json:"backwards_compatible"`
}

// BrokenReference represents a broken reference to a deleted symbol
type BrokenReference struct {
	FilePath         string `json:"file_path"`
	LineNumber       int    `json:"line_number"`
	ReferencedSymbol string `json:"referenced_symbol"`
	Context          string `json:"context"`
	Severity         string `json:"severity"` // "error", "warning", "info"
}

// ConfigSetting represents a changed configuration setting
type ConfigSetting struct {
	Key        string      `json:"key"`
	OldValue   interface{} `json:"old_value,omitempty"`
	NewValue   interface{} `json:"new_value,omitempty"`
	ChangeType string      `json:"change_type"` // "added", "modified", "removed"
	Impact     string      `json:"impact"`
}

// ComponentReference represents a component that uses configuration
type ComponentReference struct {
	FilePath       string   `json:"file_path"`
	ComponentName  string   `json:"component_name"`
	UsedSettings   []string `json:"used_settings"`
	LoadingPattern string   `json:"loading_pattern"`
}

// newSpecialCasesHandler creates a new special cases handler
func NewSpecialCasesHandler(provider interfaces.CodeProvider) *SpecialCasesHandler {
	return &SpecialCasesHandler{
		provider:       provider,
		astParser:      NewParser(),
		symbolAnalyzer: NewExternalRefsAnalyzer(provider),
		diffParser:     NewDiffParser(),
		log:            logze.With("component", "special_cases_handler"),
	}
}

// AnalyzeSpecialCase analyzes a special case file
func (sch *SpecialCasesHandler) AnalyzeSpecialCase(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	caseType := sch.IdentifySpecialCaseType(fileDiff)

	switch caseType {
	case SpecialCaseNewFile:
		return sch.analyzeNewFile(ctx, request, fileDiff)
	case SpecialCaseDeletedFile:
		return sch.analyzeDeletedFile(ctx, request, fileDiff)
	case SpecialCaseConfigFile:
		return sch.analyzeConfigFile(ctx, request, fileDiff)
	case SpecialCaseBinaryFile:
		return sch.analyzeBinaryFile(ctx, request, fileDiff)
	case SpecialCaseGeneratedFile:
		return sch.analyzeGeneratedFile(ctx, request, fileDiff)
	case SpecialCaseTestFile:
		return sch.analyzeTestFile(ctx, request, fileDiff)
	case SpecialCaseMigrationFile:
		return sch.analyzeMigrationFile(ctx, request, fileDiff)
	case SpecialCaseDocumentationFile:
		return sch.analyzeDocumentationFile(ctx, request, fileDiff)
	default:
		return nil, errm.New("not a special case")
	}
}

// IdentifySpecialCaseType identifies the type of special case
func (sch *SpecialCasesHandler) IdentifySpecialCaseType(fileDiff *model.FileDiff) SpecialCaseType {
	filePath := fileDiff.NewPath
	if filePath == "" {
		filePath = fileDiff.OldPath
	}

	// Check for deleted files
	if fileDiff.IsDeleted {
		return SpecialCaseDeletedFile
	}

	// Check for new files
	if fileDiff.IsNew {
		return SpecialCaseNewFile
	}

	// Check for binary files
	if fileDiff.IsBinary {
		return SpecialCaseBinaryFile
	}

	fileName := strings.ToLower(filepath.Base(filePath))
	fileExt := strings.ToLower(filepath.Ext(filePath))
	dirName := strings.ToLower(filepath.Dir(filePath))

	// Check for configuration files
	if sch.isConfigFile(fileName, fileExt, dirName) {
		return SpecialCaseConfigFile
	}

	// Check for test files
	if sch.isTestFile(fileName, fileExt, dirName) {
		return SpecialCaseTestFile
	}

	// Check for generated files
	if sch.isGeneratedFile(fileName, fileExt, filePath, fileDiff.Diff) {
		return SpecialCaseGeneratedFile
	}

	// Check for migration files
	if sch.isMigrationFile(fileName, fileExt, dirName) {
		return SpecialCaseMigrationFile
	}

	// Check for documentation files
	if sch.isDocumentationFile(fileName, fileExt, dirName) {
		return SpecialCaseDocumentationFile
	}

	return ""
}

// File type identification methods

func (sch *SpecialCasesHandler) isConfigFile(fileName, fileExt, dirName string) bool {
	configExtensions := []string{".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".config", ".env", ".properties"}
	configFiles := []string{"makefile", "dockerfile", "docker-compose.yml", "go.mod", "go.sum", "package.json", "requirements.txt", "composer.json", "pom.xml", "build.gradle"}
	configDirs := []string{"config", "configs", "configuration", "settings", ".github", ".gitlab"}

	// Check extensions
	for _, ext := range configExtensions {
		if fileExt == ext {
			return true
		}
	}

	// Check specific filenames
	for _, cf := range configFiles {
		if fileName == cf {
			return true
		}
	}

	// Check directories
	for _, cd := range configDirs {
		if strings.Contains(dirName, cd) {
			return true
		}
	}

	return false
}

func (sch *SpecialCasesHandler) isTestFile(fileName, fileExt, dirName string) bool {
	testPatterns := []string{"test", "spec", "_test", ".test", "_spec", ".spec"}
	testDirs := []string{"test", "tests", "spec", "specs", "__tests__"}

	// Check filename patterns
	for _, pattern := range testPatterns {
		if strings.Contains(fileName, pattern) {
			return true
		}
	}

	// Check directories
	for _, td := range testDirs {
		if strings.Contains(dirName, td) {
			return true
		}
	}

	return false
}

func (sch *SpecialCasesHandler) isGeneratedFile(fileName, fileExt, filePath, diff string) bool {
	generatedPatterns := []string{"generated", "auto-generated", "autogenerated", ".pb.go", "_pb.go", ".gen.go", "_gen.go"}
	generatedDirs := []string{"generated", "gen", "build", "dist", "target", "out"}

	// Check filename patterns
	for _, pattern := range generatedPatterns {
		if strings.Contains(fileName, pattern) || strings.Contains(filePath, pattern) {
			return true
		}
	}

	// Check directories
	for _, gd := range generatedDirs {
		if strings.Contains(strings.ToLower(filepath.Dir(filePath)), gd) {
			return true
		}
	}

	// Check for generated code markers in diff
	generatedMarkers := []string{"// Code generated", "/* Code generated", "# This file is generated", "// AUTO-GENERATED", "# AUTO-GENERATED"}
	for _, marker := range generatedMarkers {
		if strings.Contains(diff, marker) {
			return true
		}
	}

	return false
}

func (sch *SpecialCasesHandler) isMigrationFile(fileName, fileExt, dirName string) bool {
	migrationDirs := []string{"migration", "migrations", "migrate", "schema", "db"}
	migrationPatterns := []string{"migration", "migrate", "schema", ".sql", "up.sql", "down.sql"}

	// Check directories
	for _, md := range migrationDirs {
		if strings.Contains(dirName, md) {
			return true
		}
	}

	// Check filename patterns
	for _, pattern := range migrationPatterns {
		if strings.Contains(fileName, pattern) {
			return true
		}
	}

	return false
}

func (sch *SpecialCasesHandler) isDocumentationFile(fileName, fileExt, dirName string) bool {
	docExtensions := []string{".md", ".rst", ".txt", ".adoc", ".asciidoc"}
	docFiles := []string{"readme", "changelog", "license", "contributing", "authors", "todo", "install", "usage"}
	docDirs := []string{"doc", "docs", "documentation", "manual"}

	// Check extensions
	for _, ext := range docExtensions {
		if fileExt == ext {
			return true
		}
	}

	// Check filenames
	for _, df := range docFiles {
		if strings.Contains(strings.ToLower(fileName), df) {
			return true
		}
	}

	// Check directories
	for _, dd := range docDirs {
		if strings.Contains(dirName, dd) {
			return true
		}
	}

	return false
}

// Special case analysis methods

func (sch *SpecialCasesHandler) analyzeNewFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseNewFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: true,
		ContextualInfo:    make(map[string]string),
	}

	// Get file content
	content, err := sch.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
	if err != nil {
		// Fallback to extracting from diff
		content = sch.extractContentFromDiff(fileDiff.Diff)
	}

	// Analyze symbols in the new file
	symbols, err := sch.findAllSymbolsInFile(fileDiff.NewPath, content)
	if err != nil {
		sch.log.Warn("failed to analyze symbols in new file", "error", err, "file", fileDiff.NewPath)
	}

	// Identify exported symbols
	exportedSymbols := sch.identifyExportedSymbols(symbols)

	// Find dependencies
	dependencies := sch.extractDependencies(content, fileDiff.NewPath)

	// Check if this file is already being used
	consumers, err := sch.findPotentialConsumers(ctx, request, fileDiff.NewPath, exportedSymbols)
	if err != nil {
		sch.log.Warn("failed to find potential consumers", "error", err)
	}

	// Generate analysis
	analysis.Analysis = fmt.Sprintf("New file with %d symbols, %d exported", len(symbols), len(exportedSymbols))
	analysis.Impact = sch.assessNewFileImpact(exportedSymbols, dependencies, consumers)
	analysis.RelatedFiles = dependencies
	analysis.Recommendations = sch.generateNewFileRecommendations(exportedSymbols, dependencies, consumers)
	analysis.PotentialIssues = sch.identifyNewFileIssues(symbols, dependencies)

	// Add contextual information
	analysis.ContextualInfo["total_symbols"] = fmt.Sprintf("%d", len(symbols))
	analysis.ContextualInfo["exported_symbols"] = fmt.Sprintf("%d", len(exportedSymbols))
	analysis.ContextualInfo["dependencies"] = fmt.Sprintf("%d", len(dependencies))
	analysis.ContextualInfo["potential_consumers"] = fmt.Sprintf("%d", len(consumers))

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeDeletedFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseDeletedFile,
		FilePath:          fileDiff.OldPath,
		RequiresAttention: true,
		ContextualInfo:    make(map[string]string),
	}

	// Get the file content from the base branch (before deletion)
	content, err := sch.getBaseFileContent(ctx, request, fileDiff.OldPath)
	if err != nil {
		sch.log.Warn("failed to get base file content", "error", err, "file", fileDiff.OldPath)
		// Fallback: extract content from diff (removed lines)
		content = sch.extractRemovedContentFromDiff(fileDiff.Diff)
	}

	// Analyze symbols that were in the deleted file
	symbols, err := sch.findAllSymbolsInFile(fileDiff.OldPath, content)
	if err != nil {
		sch.log.Warn("failed to analyze symbols in deleted file", "error", err, "file", fileDiff.OldPath)
	}

	// Find broken references
	brokenReferences, err := sch.findBrokenReferences(ctx, request, symbols)
	if err != nil {
		sch.log.Warn("failed to find broken references", "error", err)
	}

	// Assess migration requirements
	migrationRequired := len(brokenReferences) > 0

	// Generate analysis
	analysis.Analysis = fmt.Sprintf("Deleted file contained %d symbols", len(symbols))
	analysis.Impact = sch.assessDeletedFileImpact(symbols, brokenReferences)
	analysis.Recommendations = sch.generateDeletedFileRecommendations(symbols, brokenReferences, migrationRequired)
	analysis.PotentialIssues = sch.identifyDeletedFileIssues(brokenReferences)

	// Extract related files from broken references
	var relatedFiles []string
	for _, br := range brokenReferences {
		relatedFiles = append(relatedFiles, br.FilePath)
	}
	analysis.RelatedFiles = sch.removeDuplicates(relatedFiles)

	// Add contextual information
	analysis.ContextualInfo["deleted_symbols"] = fmt.Sprintf("%d", len(symbols))
	analysis.ContextualInfo["broken_references"] = fmt.Sprintf("%d", len(brokenReferences))
	analysis.ContextualInfo["migration_required"] = fmt.Sprintf("%t", migrationRequired)

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeConfigFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseConfigFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: true,
		ContextualInfo:    make(map[string]string),
	}

	// Determine config format
	configFormat := sch.detectConfigFormat(fileDiff.NewPath)

	// Parse configuration changes
	changedSettings := sch.parseConfigChanges(fileDiff.Diff, configFormat)

	// Find components that use this configuration
	affectedComponents, err := sch.findConfigConsumers(ctx, request, fileDiff.NewPath)
	if err != nil {
		sch.log.Warn("failed to find config consumers", "error", err)
	}

	// Assess backwards compatibility
	backwardsCompatible := sch.assessConfigBackwardsCompatibility(changedSettings)

	// Generate analysis
	analysis.Analysis = fmt.Sprintf("Configuration file (%s) with %d changed settings", configFormat, len(changedSettings))
	analysis.Impact = sch.assessConfigFileImpact(changedSettings, affectedComponents, backwardsCompatible)
	analysis.Recommendations = sch.generateConfigFileRecommendations(changedSettings, affectedComponents, backwardsCompatible)
	analysis.PotentialIssues = sch.identifyConfigFileIssues(changedSettings, backwardsCompatible)

	// Extract related files
	var relatedFiles []string
	for _, component := range affectedComponents {
		relatedFiles = append(relatedFiles, component.FilePath)
	}
	analysis.RelatedFiles = sch.removeDuplicates(relatedFiles)

	// Add contextual information
	analysis.ContextualInfo["config_format"] = configFormat
	analysis.ContextualInfo["changed_settings"] = fmt.Sprintf("%d", len(changedSettings))
	analysis.ContextualInfo["affected_components"] = fmt.Sprintf("%d", len(affectedComponents))
	analysis.ContextualInfo["backwards_compatible"] = fmt.Sprintf("%t", backwardsCompatible)

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeBinaryFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseBinaryFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: false,
		ContextualInfo:    make(map[string]string),
	}

	fileExt := strings.ToLower(filepath.Ext(fileDiff.NewPath))

	// Determine binary file type
	binaryType := sch.determineBinaryType(fileExt, fileDiff.NewPath)

	analysis.Analysis = fmt.Sprintf("Binary file change (%s)", binaryType)
	analysis.Impact = sch.assessBinaryFileImpact(binaryType)
	analysis.Recommendations = sch.generateBinaryFileRecommendations(binaryType)
	analysis.PotentialIssues = sch.identifyBinaryFileIssues(binaryType)

	analysis.ContextualInfo["binary_type"] = binaryType
	analysis.ContextualInfo["file_extension"] = fileExt

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeGeneratedFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseGeneratedFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: false,
		ContextualInfo:    make(map[string]string),
	}

	analysis.Analysis = "Generated file - typically should not be manually reviewed"
	analysis.Impact = "Low - generated files should be reproducible from source"
	analysis.Recommendations = []string{
		"Verify that the generation process is documented",
		"Check if the source files that generate this were also changed",
		"Consider excluding generated files from code review if possible",
	}
	analysis.PotentialIssues = []string{
		"Manual modifications to generated files will be lost on regeneration",
	}

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeTestFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseTestFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: true,
		ContextualInfo:    make(map[string]string),
	}

	// Get file content to analyze test structure
	content, err := sch.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
	if err != nil {
		content = sch.extractContentFromDiff(fileDiff.Diff)
	}

	// Analyze test structure
	testInfo := sch.analyzeTestStructure(content, fileDiff.NewPath)

	analysis.Analysis = fmt.Sprintf("Test file with %d test cases", testInfo.TestCount)
	analysis.Impact = sch.assessTestFileImpact(testInfo)
	analysis.Recommendations = sch.generateTestFileRecommendations(testInfo)
	analysis.PotentialIssues = sch.identifyTestFileIssues(testInfo)

	// Find the files being tested
	testedFiles := sch.findTestedFiles(fileDiff.NewPath, content)
	analysis.RelatedFiles = testedFiles

	analysis.ContextualInfo["test_count"] = fmt.Sprintf("%d", testInfo.TestCount)
	analysis.ContextualInfo["tested_files"] = fmt.Sprintf("%d", len(testedFiles))

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeMigrationFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseMigrationFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: true,
		ContextualInfo:    make(map[string]string),
	}

	analysis.Analysis = "Database migration file"
	analysis.Impact = "High - database schema changes affect entire application"
	analysis.Recommendations = []string{
		"Verify migration is reversible",
		"Test migration on copy of production data",
		"Ensure migration is backwards compatible during deployment",
		"Document any manual steps required",
	}
	analysis.PotentialIssues = []string{
		"Migration may cause downtime",
		"Large data migrations may take significant time",
		"Rollback may be complex",
	}

	return analysis, nil
}

func (sch *SpecialCasesHandler) analyzeDocumentationFile(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseAnalysis, error) {
	analysis := &SpecialCaseAnalysis{
		CaseType:          SpecialCaseDocumentationFile,
		FilePath:          fileDiff.NewPath,
		RequiresAttention: false,
		ContextualInfo:    make(map[string]string),
	}

	analysis.Analysis = "Documentation file change"
	analysis.Impact = "Low - documentation changes don't affect functionality"
	analysis.Recommendations = []string{
		"Verify documentation accuracy",
		"Check for typos and grammar",
		"Ensure formatting is correct",
	}
	analysis.PotentialIssues = []string{
		"Outdated documentation may mislead users",
	}

	return analysis, nil
}

// Helper methods (continued in next part due to length)

// ... rest of helper methods will be added in a separate file or continuation
