package reviewer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"

	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// ContextBundleBuilder builds comprehensive context bundles for LLM analysis
type ContextBundleBuilder struct {
	provider       interfaces.CodeProvider
	contextFinder  *astparser.ContextFinder
	symbolAnalyzer *astparser.SymbolAnalyzer
	diffParser     *astparser.DiffParser
	astParser      *astparser.Parser
	log            logze.Logger
}

// LLMContextBundle represents the final structured context for LLM
type LLMContextBundle struct {
	Overview OverviewContext         `json:"overview"`
	Files    []astparser.FileContext `json:"files"`
	Summary  SummaryContext          `json:"summary"`
	Metadata MetadataContext         `json:"metadata"`
}

// OverviewContext provides high-level overview of the changes
type OverviewContext struct {
	TotalFiles        int                       `json:"total_files"`
	TotalSymbols      int                       `json:"total_symbols"`
	ImpactScore       float64                   `json:"impact_score"`
	ChangeComplexity  astparser.ComplexityLevel `json:"change_complexity"`
	HighImpactChanges []string                  `json:"high_impact_changes"`
	ConfigChanges     []ConfigChangeInfo        `json:"config_changes"`
	DeletedSymbols    []DeletedSymbolInfo       `json:"deleted_symbols"`
	PotentialIssues   []string                  `json:"potential_issues"`
}

// SummaryContext provides summary information for the LLM
type SummaryContext struct {
	ChangesSummary  string         `json:"changes_summary"`
	AffectedAreas   []string       `json:"affected_areas"`
	ReviewFocus     []string       `json:"review_focus"`
	RiskAssessment  RiskAssessment `json:"risk_assessment"`
	Recommendations []string       `json:"recommendations"`
}

// MetadataContext provides metadata about the analysis
type MetadataContext struct {
	AnalysisTimestamp  string   `json:"analysis_timestamp"`
	AnalysisVersion    string   `json:"analysis_version"`
	SupportedLanguages []string `json:"supported_languages"`
	Limitations        []string `json:"limitations"`
}

// ConfigChangeInfo represents information about configuration changes
type ConfigChangeInfo struct {
	FilePath      string   `json:"file_path"`
	ConfigType    string   `json:"config_type"`
	ChangedKeys   []string `json:"changed_keys"`
	Impact        string   `json:"impact"`
	AffectedFiles []string `json:"affected_files"`
}

// DeletedSymbolInfo represents information about deleted symbols
type DeletedSymbolInfo struct {
	Symbol           astparser.AffectedSymbol `json:"symbol"`
	BrokenReferences []astparser.RelatedFile  `json:"broken_references"`
	Impact           string                   `json:"impact"`
}

// RiskAssessment provides risk assessment for the changes
type RiskAssessment struct {
	Level       RiskLevel `json:"level"`
	Score       float64   `json:"score"`
	Factors     []string  `json:"factors"`
	Mitigations []string  `json:"mitigations"`
}

// RiskLevel represents the risk level of changes
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// NewContextBundleBuilder creates a new context bundle builder
func NewContextBundleBuilder(provider interfaces.CodeProvider) *ContextBundleBuilder {
	return &ContextBundleBuilder{
		provider:       provider,
		contextFinder:  astparser.NewContextFinder(provider),
		symbolAnalyzer: astparser.NewSymbolAnalyzer(provider),
		diffParser:     astparser.NewDiffParser(),
		astParser:      astparser.NewParser(),
		log:            logze.With("component", "context_bundle_builder"),
	}
}

// BuildContextBundle builds a comprehensive context bundle for LLM analysis
func (cbb *ContextBundleBuilder) BuildContextBundle(ctx context.Context, request model.ReviewRequest, fileDiffs []*model.FileDiff) (*LLMContextBundle, error) {
	// Gather basic context using ContextFinder
	basicContext, err := cbb.contextFinder.GatherContext(ctx, request, fileDiffs)
	if err != nil {
		return nil, errm.Wrap(err, "failed to gather basic context")
	}

	// Enhance with detailed analysis
	enhancedFiles, err := cbb.enhanceFileContexts(ctx, request, basicContext.Files)
	if err != nil {
		cbb.log.Warn("failed to enhance file contexts", "error", err)
		// Continue with basic context
		enhancedFiles = basicContext.Files
	}

	// Build overview
	overview := cbb.buildOverview(enhancedFiles)

	// Build summary
	summary := cbb.buildSummary(enhancedFiles, overview)

	// Build metadata
	metadata := cbb.buildMetadata()

	bundle := &LLMContextBundle{
		Overview: overview,
		Files:    enhancedFiles,
		Summary:  summary,
		Metadata: metadata,
	}

	return bundle, nil
}

// enhanceFileContexts enhances file contexts with detailed symbol analysis
func (cbb *ContextBundleBuilder) enhanceFileContexts(ctx context.Context, request model.ReviewRequest, files []astparser.FileContext) ([]astparser.FileContext, error) {
	var enhancedFiles []astparser.FileContext

	for _, fileContext := range files {
		enhanced, err := cbb.enhanceFileContext(ctx, request, fileContext)
		if err != nil {
			cbb.log.Warn("failed to enhance file context", "error", err, "file", fileContext.FilePath)
			// Continue with original context
			enhancedFiles = append(enhancedFiles, fileContext)
			continue
		}
		enhancedFiles = append(enhancedFiles, *enhanced)
	}

	return enhancedFiles, nil
}

// enhanceFileContext enhances a single file context with detailed analysis
func (cbb *ContextBundleBuilder) enhanceFileContext(ctx context.Context, request model.ReviewRequest, fileContext astparser.FileContext) (*astparser.FileContext, error) {
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
	if content != "" && fileContext.DiffHunk != "" {
		impact, err := cbb.diffParser.AnalyzeDiffImpact(fileContext.DiffHunk, fileContext.FilePath, content, cbb.astParser)
		if err == nil {
			// Add impact information to existing symbols or create new ones
			enhanced.AffectedSymbols = cbb.mergeSymbolInformation(enhanced.AffectedSymbols, impact.AffectedSymbols)
		}
	}

	// Enhance symbol information with usage context
	for i, symbol := range enhanced.AffectedSymbols {
		usageContext, err := cbb.symbolAnalyzer.AnalyzeSymbolUsage(ctx, request.ProjectID, request.MergeRequest.SHA, symbol)
		if err != nil {
			cbb.log.Warn("failed to analyze symbol usage", "error", err, "symbol", symbol.Name)
			continue
		}

		// Convert usage context to related files
		enhanced.RelatedFiles = append(enhanced.RelatedFiles, cbb.convertUsageToRelatedFiles(usageContext)...)

		// Update symbol with enhanced context information
		enhanced.AffectedSymbols[i] = cbb.enhanceSymbolWithContext(symbol, usageContext)
	}

	return &enhanced, nil
}

// mergeSymbolInformation merges symbol information from different sources
func (cbb *ContextBundleBuilder) mergeSymbolInformation(existing []astparser.AffectedSymbol, additional []astparser.AffectedSymbol) []astparser.AffectedSymbol {
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
func (cbb *ContextBundleBuilder) getSymbolKey(symbol astparser.AffectedSymbol) string {
	return fmt.Sprintf("%s:%s:%d:%d", symbol.FilePath, symbol.Name, symbol.StartLine, symbol.EndLine)
}

// mergeSymbols merges two symbol objects
func (cbb *ContextBundleBuilder) mergeSymbols(symbol1, symbol2 astparser.AffectedSymbol) astparser.AffectedSymbol {
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
func (cbb *ContextBundleBuilder) convertUsageToRelatedFiles(usage astparser.SymbolUsageContext) []astparser.RelatedFile {
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
		if dep.Source == "internal" && dep.FilePath != "" {
			relatedFile := astparser.RelatedFile{
				FilePath:         dep.FilePath,
				Relationship:     "dependency",
				RelevantFunction: dep.SymbolName,
			}
			relatedFiles = append(relatedFiles, relatedFile)
		}
	}

	return relatedFiles
}

// enhanceSymbolWithContext enhances a symbol with usage context information
func (cbb *ContextBundleBuilder) enhanceSymbolWithContext(symbol astparser.AffectedSymbol, usage astparser.SymbolUsageContext) astparser.AffectedSymbol {
	enhanced := symbol

	// Update context information
	enhanced.Context.Package = cbb.extractPackageFromPath(symbol.FilePath)

	// Set caller count and dependency information in a more structured way
	// This could be extended to include more detailed usage statistics

	return enhanced
}

// buildOverview builds the overview context
func (cbb *ContextBundleBuilder) buildOverview(files []astparser.FileContext) OverviewContext {
	overview := OverviewContext{
		TotalFiles:        len(files),
		TotalSymbols:      0,
		ImpactScore:       0.0,
		ChangeComplexity:  astparser.ComplexityLow,
		HighImpactChanges: make([]string, 0),
		ConfigChanges:     make([]ConfigChangeInfo, 0),
		DeletedSymbols:    make([]DeletedSymbolInfo, 0),
		PotentialIssues:   make([]string, 0),
	}

	var totalImpactScore float64
	var maxComplexity astparser.ComplexityLevel

	for _, file := range files {
		// Count symbols
		overview.TotalSymbols += len(file.AffectedSymbols)

		// Analyze impact and complexity
		fileComplexity := cbb.assessFileComplexity(file)
		fileImpact := cbb.calculateFileImpactScore(file)

		totalImpactScore += fileImpact

		// Track maximum complexity
		if cbb.compareComplexity(fileComplexity, maxComplexity) > 0 {
			maxComplexity = fileComplexity
		}

		// Identify high-impact changes
		if fileImpact > 10.0 { // Threshold for high impact
			description := fmt.Sprintf("High-impact changes in %s (%d symbols affected)", file.FilePath, len(file.AffectedSymbols))
			overview.HighImpactChanges = append(overview.HighImpactChanges, description)
		}

		// Collect configuration changes
		if file.ConfigContext != nil {
			configChange := ConfigChangeInfo{
				FilePath:      file.FilePath,
				ConfigType:    file.ConfigContext.ConfigType,
				ChangedKeys:   file.ConfigContext.ChangedKeys,
				Impact:        file.ConfigContext.ImpactAssessment,
				AffectedFiles: cbb.extractFilePathsFromRelated(file.ConfigContext.ConsumingCode),
			}
			overview.ConfigChanges = append(overview.ConfigChanges, configChange)
		}

		// Collect deleted symbols
		if file.ChangeType == astparser.ChangeTypeDeleted {
			for _, symbol := range file.AffectedSymbols {
				deletedInfo := DeletedSymbolInfo{
					Symbol:           symbol,
					BrokenReferences: cbb.findBrokenReferences(file.RelatedFiles),
					Impact:           cbb.assessDeletionImpact(symbol, file.RelatedFiles),
				}
				overview.DeletedSymbols = append(overview.DeletedSymbols, deletedInfo)
			}
		}

		// Collect potential issues
		overview.PotentialIssues = append(overview.PotentialIssues, cbb.identifyFileIssues(file)...)
	}

	overview.ImpactScore = totalImpactScore
	overview.ChangeComplexity = maxComplexity

	return overview
}

// buildSummary builds the summary context
func (cbb *ContextBundleBuilder) buildSummary(files []astparser.FileContext, overview OverviewContext) SummaryContext {
	summary := SummaryContext{
		AffectedAreas:   make([]string, 0),
		ReviewFocus:     make([]string, 0),
		Recommendations: make([]string, 0),
	}

	// Generate changes summary
	summary.ChangesSummary = cbb.generateChangesSummary(files, overview)

	// Identify affected areas
	summary.AffectedAreas = cbb.identifyAffectedAreas(files)

	// Determine review focus areas
	summary.ReviewFocus = cbb.determineReviewFocus(files, overview)

	// Assess risk
	summary.RiskAssessment = cbb.assessRisk(overview)

	// Generate recommendations
	summary.Recommendations = cbb.generateRecommendations(files, overview)

	return summary
}

// buildMetadata builds the metadata context
func (cbb *ContextBundleBuilder) buildMetadata() MetadataContext {
	return MetadataContext{
		AnalysisTimestamp:  fmt.Sprintf("%d", time.Now().Unix()),
		AnalysisVersion:    "1.0",
		SupportedLanguages: []string{"go", "javascript", "typescript", "python"},
		Limitations: []string{
			"AST parsing may fail for syntactically incorrect code",
			"Cross-repository dependencies are not analyzed",
			"Dynamic function calls may not be detected",
			"Generated code analysis may be incomplete",
		},
	}
}

// Helper methods for building overview and summary

// assessFileComplexity assesses the complexity of changes in a file
func (cbb *ContextBundleBuilder) assessFileComplexity(file astparser.FileContext) astparser.ComplexityLevel {
	symbolCount := len(file.AffectedSymbols)
	relatedCount := len(file.RelatedFiles)

	if symbolCount > 10 || relatedCount > 20 {
		return astparser.ComplexityCritical
	} else if symbolCount > 5 || relatedCount > 10 {
		return astparser.ComplexityHigh
	} else if symbolCount > 2 || relatedCount > 5 {
		return astparser.ComplexityMedium
	} else {
		return astparser.ComplexityLow
	}
}

// calculateFileImpactScore calculates an impact score for a file
func (cbb *ContextBundleBuilder) calculateFileImpactScore(file astparser.FileContext) float64 {
	score := 0.0

	// Base score from symbol count
	score += float64(len(file.AffectedSymbols)) * 2.0

	// Additional score based on symbol types
	for _, symbol := range file.AffectedSymbols {
		switch symbol.Type {
		case astparser.SymbolTypeInterface:
			score += 5.0
		case astparser.SymbolTypeClass, astparser.SymbolTypeStruct:
			score += 3.0
		case astparser.SymbolTypeFunction, astparser.SymbolTypeMethod:
			score += 2.0
		default:
			score += 1.0
		}

		// Score for dependencies
		score += float64(len(symbol.Dependencies)) * 0.5
	}

	// Score for related files (callers/dependencies)
	score += float64(len(file.RelatedFiles)) * 0.5

	// Higher score for config files
	if file.ConfigContext != nil {
		score += 5.0
		score += float64(len(file.ConfigContext.ChangedKeys)) * 1.0
	}

	return score
}

// compareComplexity compares two complexity levels
func (cbb *ContextBundleBuilder) compareComplexity(a, b astparser.ComplexityLevel) int {
	levels := map[astparser.ComplexityLevel]int{
		astparser.ComplexityLow:      1,
		astparser.ComplexityMedium:   2,
		astparser.ComplexityHigh:     3,
		astparser.ComplexityCritical: 4,
	}

	return levels[a] - levels[b]
}

// extractFilePathsFromRelated extracts file paths from related files
func (cbb *ContextBundleBuilder) extractFilePathsFromRelated(relatedFiles []astparser.RelatedFile) []string {
	var paths []string
	for _, rf := range relatedFiles {
		paths = append(paths, rf.FilePath)
	}
	return paths
}

// findBrokenReferences finds broken references from related files
func (cbb *ContextBundleBuilder) findBrokenReferences(relatedFiles []astparser.RelatedFile) []astparser.RelatedFile {
	var broken []astparser.RelatedFile
	for _, rf := range relatedFiles {
		if rf.Relationship == "broken_caller" || rf.Relationship == "caller" {
			broken = append(broken, rf)
		}
	}
	return broken
}

// assessDeletionImpact assesses the impact of deleting a symbol
func (cbb *ContextBundleBuilder) assessDeletionImpact(symbol astparser.AffectedSymbol, relatedFiles []astparser.RelatedFile) string {
	brokenCount := len(cbb.findBrokenReferences(relatedFiles))

	if brokenCount == 0 {
		return "Low impact - no broken references found"
	} else if brokenCount <= 3 {
		return fmt.Sprintf("Medium impact - %d references may be broken", brokenCount)
	} else {
		return fmt.Sprintf("High impact - %d references may be broken", brokenCount)
	}
}

// identifyFileIssues identifies potential issues in a file
func (cbb *ContextBundleBuilder) identifyFileIssues(file astparser.FileContext) []string {
	var issues []string

	// Large number of symbols affected
	if len(file.AffectedSymbols) > 10 {
		issues = append(issues, fmt.Sprintf("Large change in %s - %d symbols affected", file.FilePath, len(file.AffectedSymbols)))
	}

	// Interface changes
	for _, symbol := range file.AffectedSymbols {
		if symbol.Type == astparser.SymbolTypeInterface {
			issues = append(issues, fmt.Sprintf("Interface %s.%s modified - potential breaking change", file.FilePath, symbol.Name))
		}
	}

	// High coupling
	if len(file.RelatedFiles) > 15 {
		issues = append(issues, fmt.Sprintf("High coupling in %s - %d related files", file.FilePath, len(file.RelatedFiles)))
	}

	return issues
}

// generateChangesSummary generates a summary of changes
func (cbb *ContextBundleBuilder) generateChangesSummary(files []astparser.FileContext, overview OverviewContext) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%d files modified with %d symbols affected", overview.TotalFiles, overview.TotalSymbols))

	// Add complexity information
	parts = append(parts, fmt.Sprintf("Change complexity: %s", overview.ChangeComplexity))

	// Add specific change type information
	addedCount, modifiedCount, deletedCount := cbb.countChangeTypes(files)
	if addedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d files added", addedCount))
	}
	if modifiedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d files modified", modifiedCount))
	}
	if deletedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d files deleted", deletedCount))
	}

	// Add configuration changes
	if len(overview.ConfigChanges) > 0 {
		parts = append(parts, fmt.Sprintf("%d configuration files changed", len(overview.ConfigChanges)))
	}

	return strings.Join(parts, ". ")
}

// identifyAffectedAreas identifies affected areas/modules
func (cbb *ContextBundleBuilder) identifyAffectedAreas(files []astparser.FileContext) []string {
	areaMap := make(map[string]int)

	for _, file := range files {
		// Extract area from file path (directory structure)
		dir := filepath.Dir(file.FilePath)
		if dir != "." {
			// Use the top-level directory as the area
			parts := strings.Split(dir, "/")
			if len(parts) > 0 {
				area := parts[0]
				areaMap[area]++
			}
		}
	}

	var areas []string
	for area, count := range areaMap {
		areas = append(areas, fmt.Sprintf("%s (%d files)", area, count))
	}

	return areas
}

// determineReviewFocus determines what the review should focus on
func (cbb *ContextBundleBuilder) determineReviewFocus(files []astparser.FileContext, overview OverviewContext) []string {
	var focus []string

	// High-impact changes
	if len(overview.HighImpactChanges) > 0 {
		focus = append(focus, "High-impact changes requiring careful review")
	}

	// Interface changes
	interfaceChanges := 0
	for _, file := range files {
		for _, symbol := range file.AffectedSymbols {
			if symbol.Type == astparser.SymbolTypeInterface {
				interfaceChanges++
			}
		}
	}
	if interfaceChanges > 0 {
		focus = append(focus, fmt.Sprintf("Interface changes (%d) - check for breaking changes", interfaceChanges))
	}

	// Configuration changes
	if len(overview.ConfigChanges) > 0 {
		focus = append(focus, "Configuration changes - verify impact on dependent systems")
	}

	// Deleted symbols
	if len(overview.DeletedSymbols) > 0 {
		focus = append(focus, "Deleted symbols - check for broken references")
	}

	// Complex changes
	if overview.ChangeComplexity == astparser.ComplexityHigh || overview.ChangeComplexity == astparser.ComplexityCritical {
		focus = append(focus, "Complex changes requiring thorough testing")
	}

	return focus
}

// assessRisk assesses the overall risk of the changes
func (cbb *ContextBundleBuilder) assessRisk(overview OverviewContext) RiskAssessment {
	assessment := RiskAssessment{
		Level:       RiskLevelLow,
		Score:       overview.ImpactScore,
		Factors:     make([]string, 0),
		Mitigations: make([]string, 0),
	}

	// Assess risk level based on various factors
	riskFactors := 0

	if overview.ChangeComplexity == astparser.ComplexityCritical {
		riskFactors += 3
		assessment.Factors = append(assessment.Factors, "Critical complexity changes")
	} else if overview.ChangeComplexity == astparser.ComplexityHigh {
		riskFactors += 2
		assessment.Factors = append(assessment.Factors, "High complexity changes")
	}

	if len(overview.DeletedSymbols) > 0 {
		riskFactors += 2
		assessment.Factors = append(assessment.Factors, fmt.Sprintf("%d symbols deleted", len(overview.DeletedSymbols)))
	}

	if len(overview.ConfigChanges) > 0 {
		riskFactors += 1
		assessment.Factors = append(assessment.Factors, "Configuration changes")
	}

	if overview.ImpactScore > 50 {
		riskFactors += 2
		assessment.Factors = append(assessment.Factors, "High impact score")
	}

	// Determine risk level
	if riskFactors >= 5 {
		assessment.Level = RiskLevelCritical
	} else if riskFactors >= 3 {
		assessment.Level = RiskLevelHigh
	} else if riskFactors >= 1 {
		assessment.Level = RiskLevelMedium
	}

	// Generate mitigations
	assessment.Mitigations = cbb.generateMitigations(assessment.Level, assessment.Factors)

	return assessment
}

// generateRecommendations generates recommendations for the review
func (cbb *ContextBundleBuilder) generateRecommendations(files []astparser.FileContext, overview OverviewContext) []string {
	var recommendations []string

	// Based on complexity
	if overview.ChangeComplexity == astparser.ComplexityCritical {
		recommendations = append(recommendations, "Consider breaking this large change into smaller, more focused changes")
	}

	// Based on deleted symbols
	if len(overview.DeletedSymbols) > 0 {
		recommendations = append(recommendations, "Verify that all references to deleted symbols have been properly updated")
	}

	// Based on configuration changes
	if len(overview.ConfigChanges) > 0 {
		recommendations = append(recommendations, "Update documentation to reflect configuration changes")
		recommendations = append(recommendations, "Consider backwards compatibility for configuration changes")
	}

	// Based on interface changes
	interfaceCount := 0
	for _, file := range files {
		for _, symbol := range file.AffectedSymbols {
			if symbol.Type == astparser.SymbolTypeInterface {
				interfaceCount++
			}
		}
	}
	if interfaceCount > 0 {
		recommendations = append(recommendations, "Review interface changes for backwards compatibility")
		recommendations = append(recommendations, "Update API documentation if public interfaces changed")
	}

	// General recommendations
	if overview.TotalSymbols > 20 {
		recommendations = append(recommendations, "Ensure comprehensive test coverage for the large number of changes")
	}

	return recommendations
}

// generateMitigations generates risk mitigations
func (cbb *ContextBundleBuilder) generateMitigations(level RiskLevel, factors []string) []string {
	var mitigations []string

	switch level {
	case RiskLevelCritical:
		mitigations = append(mitigations, "Require multiple reviewers")
		mitigations = append(mitigations, "Perform comprehensive testing")
		mitigations = append(mitigations, "Consider staged deployment")
		mitigations = append(mitigations, "Prepare rollback plan")

	case RiskLevelHigh:
		mitigations = append(mitigations, "Require thorough review")
		mitigations = append(mitigations, "Ensure test coverage")
		mitigations = append(mitigations, "Test in staging environment")

	case RiskLevelMedium:
		mitigations = append(mitigations, "Standard review process")
		mitigations = append(mitigations, "Verify test coverage")

	case RiskLevelLow:
		mitigations = append(mitigations, "Standard review")
	}

	// Factor-specific mitigations
	for _, factor := range factors {
		if strings.Contains(factor, "deleted") {
			mitigations = append(mitigations, "Run static analysis to find orphaned references")
		}
		if strings.Contains(factor, "Configuration") {
			mitigations = append(mitigations, "Test configuration changes in isolated environment")
		}
	}

	return mitigations
}

// countChangeTypes counts different types of changes
func (cbb *ContextBundleBuilder) countChangeTypes(files []astparser.FileContext) (added, modified, deleted int) {
	for _, file := range files {
		switch file.ChangeType {
		case astparser.ChangeTypeAdded:
			added++
		case astparser.ChangeTypeModified, astparser.ChangeTypeRenamed:
			modified++
		case astparser.ChangeTypeDeleted:
			deleted++
		}
	}
	return
}

// extractPackageFromPath extracts package name from file path
func (cbb *ContextBundleBuilder) extractPackageFromPath(filePath string) string {
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
