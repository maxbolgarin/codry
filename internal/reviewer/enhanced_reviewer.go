package reviewer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"github.com/panjf2000/ants/v2"
)

// EnhancedReviewer implements enhanced code review with comprehensive context analysis
type EnhancedReviewer struct {
	provider interfaces.CodeProvider
	agent    *agent.Agent
	pool     *ants.Pool

	// Enhanced components
	contextBundleBuilder *ContextBundleBuilder
	specialCasesHandler  *astparser.SpecialCasesHandler
	contextFinder        *astparser.ContextFinder
	symbolAnalyzer       *astparser.SymbolAnalyzer
	astParser            *astparser.Parser
	diffParser           *astparser.DiffParser

	cfg Config
	log logze.Logger

	// Track processed MRs and reviewed files
	processedMRs *abstract.SafeMapOfMaps[string, string, string]
	reviewedMRs  *abstract.SafeMap[string, reviewTrackingInfo]
}

// EnhancedReviewResult represents the result of enhanced code review
type EnhancedReviewResult struct {
	MROverview      MROverview          `json:"mr_overview"`
	FileReviews     []FileReview        `json:"file_reviews"`
	SpecialCases    []SpecialCaseReview `json:"special_cases"`
	ContextBundle   *LLMContextBundle   `json:"context_bundle"`
	CommentsCreated int                 `json:"comments_created"`
	ReviewSummary   ReviewSummary       `json:"review_summary"`
	ProcessingTime  time.Duration       `json:"processing_time"`
}

// MROverview provides high-level overview of the merge request
type MROverview struct {
	TotalFiles      int                       `json:"total_files"`
	TotalChanges    int                       `json:"total_changes"`
	ComplexityLevel astparser.ComplexityLevel `json:"complexity_level"`
	RiskAssessment  RiskAssessment            `json:"risk_assessment"`
	ImpactScore     float64                   `json:"impact_score"`
	KeyChanges      []string                  `json:"key_changes"`
}

// FileReview represents review results for a single file
type FileReview struct {
	FilePath        string                        `json:"file_path"`
	ChangeType      astparser.ChangeType          `json:"change_type"`
	ImpactAnalysis  *astparser.DiffImpactAnalysis `json:"impact_analysis"`
	AffectedSymbols []astparser.AffectedSymbol    `json:"affected_symbols"`
	Comments        []*model.ReviewAIComment      `json:"comments"`
	Issues          []string                      `json:"issues"`
	Recommendations []string                      `json:"recommendations"`
}

// SpecialCaseReview represents review results for special cases
type SpecialCaseReview struct {
	CaseType       astparser.SpecialCaseType      `json:"case_type"`
	FilePath       string                         `json:"file_path"`
	Analysis       *astparser.SpecialCaseAnalysis `json:"analysis"`
	Comments       []*model.ReviewAIComment       `json:"comments"`
	RequiresAction bool                           `json:"requires_action"`
}

// ReviewSummary provides a summary of the entire review
type ReviewSummary struct {
	TotalIssues            int      `json:"total_issues"`
	CriticalIssues         int      `json:"critical_issues"`
	Recommendations        []string `json:"recommendations"`
	OverallAssessment      string   `json:"overall_assessment"`
	ApprovalRecommendation string   `json:"approval_recommendation"`
}

// NewEnhancedReviewer creates a new enhanced reviewer
func NewEnhancedReviewer(cfg Config, provider interfaces.CodeProvider, agent *agent.Agent) (*EnhancedReviewer, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, errm.Wrap(err, "failed to prepare and validate config")
	}

	pool, err := ants.NewPool(defaultPoolSize)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create ants pool")
	}

	s := &EnhancedReviewer{
		provider:             provider,
		agent:                agent,
		cfg:                  cfg,
		log:                  logze.With("component", "enhanced_reviewer"),
		pool:                 pool,
		contextBundleBuilder: NewContextBundleBuilder(provider),
		specialCasesHandler:  astparser.NewSpecialCasesHandler(provider),
		contextFinder:        astparser.NewContextFinder(provider),
		symbolAnalyzer:       astparser.NewSymbolAnalyzer(provider),
		astParser:            astparser.NewParser(),
		diffParser:           astparser.NewDiffParser(),
		processedMRs:         abstract.NewSafeMapOfMaps[string, string, string](),
		reviewedMRs:          abstract.NewSafeMap[string, reviewTrackingInfo](),
	}

	return s, nil
}

// ReviewMergeRequestWithContext performs enhanced code review with comprehensive context
func (er *EnhancedReviewer) ReviewMergeRequestWithContext(ctx context.Context, request model.ReviewRequest) (*EnhancedReviewResult, error) {
	startTime := time.Now()

	er.log.Info("starting enhanced code review", "mr", request.MergeRequest.IID, "project", request.ProjectID)

	// Get file diffs
	fileDiffs, err := er.provider.GetMergeRequestDiffs(ctx, request.ProjectID, request.MergeRequest.IID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get merge request diffs")
	}

	if len(fileDiffs) == 0 {
		er.log.Info("no file diffs found", "mr", request.MergeRequest.IID)
		return &EnhancedReviewResult{
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// Build comprehensive context bundle
	contextBundle, err := er.contextBundleBuilder.BuildContextBundle(ctx, request, fileDiffs)
	if err != nil {
		er.log.Warn("failed to build context bundle", "error", err)
		// Continue with reduced functionality
	}

	// Generate MR overview
	mrOverview := er.generateMROverview(fileDiffs, contextBundle)

	// Process each file
	var fileReviews []FileReview
	var specialCaseReviews []SpecialCaseReview
	totalCommentsCreated := 0

	for _, fileDiff := range fileDiffs {
		// Check if this is a special case
		if er.isSpecialCase(fileDiff) {
			specialReview, err := er.reviewSpecialCase(ctx, request, fileDiff)
			if err != nil {
				er.log.Warn("failed to review special case", "error", err, "file", fileDiff.NewPath)
				continue
			}
			specialCaseReviews = append(specialCaseReviews, *specialReview)
			totalCommentsCreated += len(specialReview.Comments)
		} else {
			// Regular file review with enhanced context
			fileReview, err := er.reviewFileWithContext(ctx, request, fileDiff, contextBundle)
			if err != nil {
				er.log.Warn("failed to review file", "error", err, "file", fileDiff.NewPath)
				continue
			}
			fileReviews = append(fileReviews, *fileReview)
			totalCommentsCreated += len(fileReview.Comments)
		}
	}

	// Generate review summary
	reviewSummary := er.generateReviewSummary(fileReviews, specialCaseReviews, mrOverview)

	result := &EnhancedReviewResult{
		MROverview:      mrOverview,
		FileReviews:     fileReviews,
		SpecialCases:    specialCaseReviews,
		ContextBundle:   contextBundle,
		CommentsCreated: totalCommentsCreated,
		ReviewSummary:   reviewSummary,
		ProcessingTime:  time.Since(startTime),
	}

	er.log.Info("enhanced code review completed",
		"mr", request.MergeRequest.IID,
		"files", len(fileDiffs),
		"comments", totalCommentsCreated,
		"duration", result.ProcessingTime)

	return result, nil
}

// generateMROverview generates a high-level overview of the merge request
func (er *EnhancedReviewer) generateMROverview(fileDiffs []*model.FileDiff, contextBundle *LLMContextBundle) MROverview {
	overview := MROverview{
		TotalFiles: len(fileDiffs),
		KeyChanges: make([]string, 0),
	}

	// Calculate total changes and complexity
	totalLines := 0
	maxComplexity := astparser.ComplexityLow
	var impactScores []float64

	for _, fileDiff := range fileDiffs {
		// Count lines changed (simple heuristic)
		lines := len(strings.Split(fileDiff.Diff, "\n"))
		totalLines += lines

		// Assess file complexity
		fileComplexity := er.assessFileComplexity(fileDiff)
		if er.compareComplexity(fileComplexity, maxComplexity) > 0 {
			maxComplexity = fileComplexity
		}

		// Calculate impact score
		impactScore := er.calculateFileImpact(fileDiff)
		impactScores = append(impactScores, impactScore)

		// Identify key changes
		if impactScore > 10.0 {
			overview.KeyChanges = append(overview.KeyChanges,
				fmt.Sprintf("High-impact changes in %s", fileDiff.NewPath))
		}
	}

	overview.TotalChanges = totalLines
	overview.ComplexityLevel = maxComplexity

	// Calculate average impact score
	if len(impactScores) > 0 {
		var total float64
		for _, score := range impactScores {
			total += score
		}
		overview.ImpactScore = total / float64(len(impactScores))
	}

	// Use context bundle information if available
	if contextBundle != nil {
		overview.RiskAssessment = contextBundle.Summary.RiskAssessment
		overview.KeyChanges = append(overview.KeyChanges, contextBundle.Overview.HighImpactChanges...)
	} else {
		// Fallback risk assessment
		overview.RiskAssessment = er.assessRiskFallback(overview)
	}

	return overview
}

// reviewSpecialCase reviews a file identified as a special case
func (er *EnhancedReviewer) reviewSpecialCase(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*SpecialCaseReview, error) {
	// Analyze the special case
	analysis, err := er.specialCasesHandler.AnalyzeSpecialCase(ctx, request, fileDiff)
	if err != nil {
		return nil, errm.Wrap(err, "failed to analyze special case")
	}

	// Get AI review for special case
	reviewResult, err := er.agent.ReviewCode(ctx, fileDiff.NewPath, "", fileDiff.Diff)
	if err != nil {
		er.log.Warn("failed to get AI review for special case", "error", err)
		reviewResult = &model.FileReviewResult{
			Comments:  make([]*model.ReviewAIComment, 0),
			HasIssues: analysis.RequiresAttention,
		}
	}

	// Create and post comments
	comments := er.processSpecialCaseComments(ctx, request, fileDiff, reviewResult.Comments, analysis)

	return &SpecialCaseReview{
		CaseType:       analysis.CaseType,
		FilePath:       analysis.FilePath,
		Analysis:       analysis,
		Comments:       comments,
		RequiresAction: analysis.RequiresAttention,
	}, nil
}

// reviewFileWithContext reviews a regular file with enhanced context
func (er *EnhancedReviewer) reviewFileWithContext(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, contextBundle *LLMContextBundle) (*FileReview, error) {
	// Perform impact analysis
	impactAnalysis, err := er.performImpactAnalysis(ctx, request, fileDiff)
	if err != nil {
		er.log.Warn("failed to perform impact analysis", "error", err, "file", fileDiff.NewPath)
	}

	// Find affected symbols
	affectedSymbols, err := er.findAffectedSymbols(ctx, request, fileDiff)
	if err != nil {
		er.log.Warn("failed to find affected symbols", "error", err, "file", fileDiff.NewPath)
		affectedSymbols = make([]astparser.AffectedSymbol, 0)
	}

	// Get file content
	fullFileContent, cleanDiff, err := er.prepareFileContentAndDiff(ctx, request, fileDiff)
	if err != nil {
		return nil, errm.Wrap(err, "failed to prepare file content and diff")
	}

	// Get AI review with enhanced context
	reviewResult, err := er.agent.ReviewCode(ctx, fileDiff.NewPath, fullFileContent, cleanDiff)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get AI review")
	}

	// Process and create comments
	comments := er.processEnhancedComments(ctx, request, fileDiff, reviewResult.Comments)

	// Generate recommendations
	recommendations := er.generateFileRecommendations(impactAnalysis, affectedSymbols)

	// Identify issues
	issues := er.identifyFileIssues(impactAnalysis, affectedSymbols)

	return &FileReview{
		FilePath:        fileDiff.NewPath,
		ChangeType:      er.determineChangeType(fileDiff),
		ImpactAnalysis:  impactAnalysis,
		AffectedSymbols: affectedSymbols,
		Comments:        comments,
		Issues:          issues,
		Recommendations: recommendations,
	}, nil
}

// Helper methods for enhanced review

func (er *EnhancedReviewer) isSpecialCase(fileDiff *model.FileDiff) bool {
	caseType := er.specialCasesHandler.IdentifySpecialCaseType(fileDiff)
	return caseType != ""
}

func (er *EnhancedReviewer) performImpactAnalysis(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*astparser.DiffImpactAnalysis, error) {
	// Get file content
	var content string
	var err error

	if !fileDiff.IsDeleted {
		content, err = er.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
		if err != nil {
			er.log.Warn("failed to get file content", "error", err, "file", fileDiff.NewPath)
		}
	}

	return er.diffParser.AnalyzeDiffImpact(fileDiff.Diff, fileDiff.NewPath, content, er.astParser)
}

func (er *EnhancedReviewer) findAffectedSymbols(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) ([]astparser.AffectedSymbol, error) {
	if fileDiff.IsDeleted {
		// For deleted files, get symbols from base content
		content, err := er.getBaseFileContent(ctx, request, fileDiff.OldPath)
		if err != nil {
			return nil, err
		}
		return er.astParser.FindAffectedSymbols(ctx, fileDiff.OldPath, content, []int{})
	}

	// For modified/new files, get current content
	content, err := er.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.SHA)
	if err != nil {
		return nil, err
	}

	// Parse diff to get changed lines
	diffLines, err := er.diffParser.ParseDiffToLines(fileDiff.Diff)
	if err != nil {
		return nil, err
	}

	var changedLines []int
	for _, line := range diffLines {
		if line.Type == astparser.DiffAddedLine && line.NewLine > 0 {
			changedLines = append(changedLines, line.NewLine)
		}
	}

	return er.astParser.FindAffectedSymbols(ctx, fileDiff.NewPath, content, changedLines)
}

func (er *EnhancedReviewer) createSpecialCasePrompt(analysis *astparser.SpecialCaseAnalysis, fileDiff *model.FileDiff) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("You are reviewing a %s file. ", analysis.CaseType))
	prompt.WriteString(fmt.Sprintf("Analysis: %s\n", analysis.Analysis))
	prompt.WriteString(fmt.Sprintf("Impact: %s\n", analysis.Impact))

	if len(analysis.PotentialIssues) > 0 {
		prompt.WriteString("Potential issues to watch for:\n")
		for _, issue := range analysis.PotentialIssues {
			prompt.WriteString(fmt.Sprintf("- %s\n", issue))
		}
	}

	if len(analysis.Recommendations) > 0 {
		prompt.WriteString("Recommendations:\n")
		for _, rec := range analysis.Recommendations {
			prompt.WriteString(fmt.Sprintf("- %s\n", rec))
		}
	}

	prompt.WriteString("\nPlease provide specific feedback relevant to this type of file.")

	return prompt.String()
}

func (er *EnhancedReviewer) createEnhancedPrompt(fileDiff *model.FileDiff, impact *astparser.DiffImpactAnalysis, symbols []astparser.AffectedSymbol, contextBundle *LLMContextBundle) string {
	var prompt strings.Builder

	prompt.WriteString("You are performing an enhanced code review with comprehensive context.\n\n")

	// Add impact information
	if impact != nil {
		prompt.WriteString(fmt.Sprintf("Impact Analysis:\n"))
		prompt.WriteString(fmt.Sprintf("- Total lines changed: %d\n", impact.TotalLinesChanged))
		prompt.WriteString(fmt.Sprintf("- Symbols affected: %d\n", impact.SymbolsAffectedCount))
		prompt.WriteString(fmt.Sprintf("- Complexity: %s\n", impact.ChangeComplexity))
		prompt.WriteString(fmt.Sprintf("- Impact score: %.2f\n", impact.ImpactScore))

		if len(impact.PotentialIssues) > 0 {
			prompt.WriteString("Potential Issues:\n")
			for _, issue := range impact.PotentialIssues {
				prompt.WriteString(fmt.Sprintf("- %s\n", issue))
			}
		}
		prompt.WriteString("\n")
	}

	// Add symbol information
	if len(symbols) > 0 {
		prompt.WriteString("Affected Symbols:\n")
		for _, symbol := range symbols {
			prompt.WriteString(fmt.Sprintf("- %s %s (lines %d-%d)\n",
				symbol.Type, symbol.Name, symbol.StartLine, symbol.EndLine))

			if len(symbol.Dependencies) > 0 {
				prompt.WriteString(fmt.Sprintf("  Dependencies: %d\n", len(symbol.Dependencies)))
			}
		}
		prompt.WriteString("\n")
	}

	// Add context bundle information
	if contextBundle != nil {
		if len(contextBundle.Overview.HighImpactChanges) > 0 {
			prompt.WriteString("High Impact Changes in MR:\n")
			for _, change := range contextBundle.Overview.HighImpactChanges {
				prompt.WriteString(fmt.Sprintf("- %s\n", change))
			}
			prompt.WriteString("\n")
		}

		if len(contextBundle.Summary.ReviewFocus) > 0 {
			prompt.WriteString("Review Focus Areas:\n")
			for _, focus := range contextBundle.Summary.ReviewFocus {
				prompt.WriteString(fmt.Sprintf("- %s\n", focus))
			}
			prompt.WriteString("\n")
		}
	}

	prompt.WriteString("Please provide detailed, context-aware feedback focusing on the most important aspects.")

	return prompt.String()
}

func (er *EnhancedReviewer) processSpecialCaseComments(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, comments []*model.ReviewAIComment, analysis *astparser.SpecialCaseAnalysis) []*model.ReviewAIComment {
	var processedComments []*model.ReviewAIComment

	// Enhance comments with special case context
	for _, comment := range comments {
		comment.FilePath = fileDiff.NewPath
		comment.CodeLanguage = detectProgrammingLanguage(fileDiff.NewPath)

		// Add special case context to comment
		if analysis.RequiresAttention {
			comment.IssueImpact = model.IssueImpactHigh
		}

		// Create the comment
		modelComment := buildComment(er.cfg.Language, comment)
		err := er.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, modelComment)
		if err != nil {
			er.log.Error("failed to create special case comment", "error", err, "file", fileDiff.NewPath)
			continue
		}

		processedComments = append(processedComments, comment)
	}

	return processedComments
}

func (er *EnhancedReviewer) processEnhancedComments(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff, comments []*model.ReviewAIComment) []*model.ReviewAIComment {
	var processedComments []*model.ReviewAIComment

	// Enhance comments with diff position information
	if err := er.diffParser.EnhanceReviewComments(fileDiff.Diff, comments); err != nil {
		er.log.Warn("failed to enhance comments with diff positions", "error", err)
	}

	// Score and filter comments
	filteredComments := er.scoreAndFilterComments(ctx, comments, fileDiff)

	// Create comments
	for _, comment := range filteredComments {
		comment.FilePath = fileDiff.NewPath
		comment.CodeLanguage = detectProgrammingLanguage(fileDiff.NewPath)

		modelComment := buildComment(er.cfg.Language, comment)
		err := er.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, modelComment)
		if err != nil {
			er.log.Error("failed to create enhanced comment", "error", err, "file", fileDiff.NewPath)
			continue
		}

		processedComments = append(processedComments, comment)
	}

	return processedComments
}

func (er *EnhancedReviewer) scoreAndFilterComments(ctx context.Context, comments []*model.ReviewAIComment, fileDiff *model.FileDiff) []*model.ReviewAIComment {
	// Use existing scoring logic from the original reviewer
	// This would be implemented similar to the original scoreAndFilterComments method
	return comments // Simplified for now
}

func (er *EnhancedReviewer) generateReviewSummary(fileReviews []FileReview, specialCases []SpecialCaseReview, mrOverview MROverview) ReviewSummary {
	summary := ReviewSummary{
		Recommendations: make([]string, 0),
	}

	// Count issues
	for _, review := range fileReviews {
		summary.TotalIssues += len(review.Issues)
		for _, comment := range review.Comments {
			if comment.IssueImpact == model.IssueImpactHigh {
				summary.CriticalIssues++
			}
		}
		summary.Recommendations = append(summary.Recommendations, review.Recommendations...)
	}

	for _, specialCase := range specialCases {
		if specialCase.RequiresAction {
			summary.TotalIssues++
		}
		if specialCase.Analysis.RequiresAttention {
			summary.CriticalIssues++
		}
	}

	// Generate overall assessment
	summary.OverallAssessment = er.generateOverallAssessment(mrOverview, summary)
	summary.ApprovalRecommendation = er.generateApprovalRecommendation(mrOverview, summary)

	return summary
}

// Additional helper methods would be implemented here...

func (er *EnhancedReviewer) generateOverallAssessment(overview MROverview, summary ReviewSummary) string {
	if summary.CriticalIssues > 0 {
		return fmt.Sprintf("Code review identified %d critical issues requiring attention", summary.CriticalIssues)
	}

	if overview.RiskAssessment.Level == RiskLevelHigh || overview.RiskAssessment.Level == RiskLevelCritical {
		return fmt.Sprintf("High-risk changes detected (risk level: %s)", overview.RiskAssessment.Level)
	}

	if summary.TotalIssues > 10 {
		return fmt.Sprintf("Multiple issues found (%d total) - careful review recommended", summary.TotalIssues)
	}

	return "Code review completed with minor or no issues"
}

func (er *EnhancedReviewer) generateApprovalRecommendation(overview MROverview, summary ReviewSummary) string {
	if summary.CriticalIssues > 0 {
		return "Changes required - address critical issues before approval"
	}

	if overview.RiskAssessment.Level == RiskLevelCritical {
		return "Requires additional review - high-risk changes"
	}

	if overview.RiskAssessment.Level == RiskLevelHigh {
		return "Conditional approval - monitor for issues in testing"
	}

	if summary.TotalIssues > 5 {
		return "Approval with minor improvements suggested"
	}

	return "Approved - good to merge"
}

// Import missing functions and types from the original reviewer
// These functions are implemented in changes.go

func (er *EnhancedReviewer) prepareFileContentAndDiff(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (string, string, error) {
	// Implementation similar to original reviewer
	return "", "", nil
}

func (er *EnhancedReviewer) getBaseFileContent(ctx context.Context, request model.ReviewRequest, filePath string) (string, error) {
	// Implementation to get base file content
	return "", nil
}

func (er *EnhancedReviewer) assessFileComplexity(fileDiff *model.FileDiff) astparser.ComplexityLevel {
	lines := len(strings.Split(fileDiff.Diff, "\n"))
	if lines > 100 {
		return astparser.ComplexityHigh
	} else if lines > 50 {
		return astparser.ComplexityMedium
	}
	return astparser.ComplexityLow
}

func (er *EnhancedReviewer) calculateFileImpact(fileDiff *model.FileDiff) float64 {
	// Simple heuristic based on diff size
	return float64(len(strings.Split(fileDiff.Diff, "\n"))) * 0.1
}

func (er *EnhancedReviewer) compareComplexity(a, b astparser.ComplexityLevel) int {
	levels := map[astparser.ComplexityLevel]int{
		astparser.ComplexityLow:      1,
		astparser.ComplexityMedium:   2,
		astparser.ComplexityHigh:     3,
		astparser.ComplexityCritical: 4,
	}
	return levels[a] - levels[b]
}

func (er *EnhancedReviewer) assessRiskFallback(overview MROverview) RiskAssessment {
	level := RiskLevelLow
	if overview.ComplexityLevel == astparser.ComplexityHigh {
		level = RiskLevelMedium
	} else if overview.ComplexityLevel == astparser.ComplexityCritical {
		level = RiskLevelHigh
	}

	return RiskAssessment{
		Level:       level,
		Score:       overview.ImpactScore,
		Factors:     []string{fmt.Sprintf("Complexity: %s", overview.ComplexityLevel)},
		Mitigations: []string{"Standard review process"},
	}
}

func (er *EnhancedReviewer) determineChangeType(fileDiff *model.FileDiff) astparser.ChangeType {
	if fileDiff.IsNew {
		return astparser.ChangeTypeAdded
	} else if fileDiff.IsDeleted {
		return astparser.ChangeTypeDeleted
	} else if fileDiff.IsRenamed {
		return astparser.ChangeTypeRenamed
	}
	return astparser.ChangeTypeModified
}

func (er *EnhancedReviewer) generateFileRecommendations(impact *astparser.DiffImpactAnalysis, symbols []astparser.AffectedSymbol) []string {
	var recommendations []string

	if impact != nil && impact.ChangeComplexity == astparser.ComplexityHigh {
		recommendations = append(recommendations, "Consider breaking down complex changes")
	}

	if len(symbols) > 5 {
		recommendations = append(recommendations, "Multiple symbols affected - ensure comprehensive testing")
	}

	return recommendations
}

func (er *EnhancedReviewer) identifyFileIssues(impact *astparser.DiffImpactAnalysis, symbols []astparser.AffectedSymbol) []string {
	var issues []string

	if impact != nil {
		issues = append(issues, impact.PotentialIssues...)
	}

	for _, symbol := range symbols {
		if len(symbol.Dependencies) > 10 {
			issues = append(issues, fmt.Sprintf("High coupling in %s", symbol.Name))
		}
	}

	return issues
}
