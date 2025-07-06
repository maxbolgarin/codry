package reviewer

import (
	"sort"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
)

// QualityScorer evaluates and prioritizes review comments based on various factors
type QualityScorer struct {
	config QualityScoringConfig
}

// QualityScoringConfig contains configuration for quality scoring
type QualityScoringConfig struct {
	MinScore                 float64 // Minimum score for a comment to be included
	MaxCommentsPerFile       int     // Maximum number of comments per file
	PrioritizeHighImpact     bool    // Prioritize high-impact issues
	PrioritizeSecurityIssues bool    // Prioritize security-related issues
}

// ScoredComment represents a comment with its quality score
type ScoredComment struct {
	Comment *model.ReviewAIComment
	Score   float64
	Factors QualityFactors
}

// QualityFactors represents the factors that contribute to the quality score
type QualityFactors struct {
	SeverityScore     float64 // Based on severity level
	ConfidenceScore   float64 // Based on confidence level
	IssueTypeScore    float64 // Based on issue type importance
	ContextScore      float64 // Based on available context
	ComplexityScore   float64 // Based on code complexity
	SecurityScore     float64 // Security-related bonus
	ArchitectureScore float64 // Architecture-related bonus
}

// NewQualityScorer creates a new quality scorer
func NewQualityScorer(config QualityScoringConfig) *QualityScorer {
	return &QualityScorer{
		config: config,
	}
}

// DefaultQualityScoringConfig returns a default configuration
func DefaultQualityScoringConfig() QualityScoringConfig {
	return QualityScoringConfig{
		MinScore:                 0.6, // Only include comments with score >= 0.6
		MaxCommentsPerFile:       5,   // Limit to top 5 issues per file
		PrioritizeHighImpact:     true,
		PrioritizeSecurityIssues: true,
	}
}

// ScoreAndFilterComments scores comments and returns the highest quality ones
func (qs *QualityScorer) ScoreAndFilterComments(comments []*model.ReviewAIComment, enhancedCtx *EnhancedContext) []*model.ReviewAIComment {
	if len(comments) == 0 {
		return comments
	}

	// Score all comments
	var scoredComments []ScoredComment
	for _, comment := range comments {
		score, factors := qs.calculateQualityScore(comment, enhancedCtx)
		if score >= qs.config.MinScore {
			scoredComments = append(scoredComments, ScoredComment{
				Comment: comment,
				Score:   score,
				Factors: factors,
			})
		}
	}

	// Sort by score (highest first)
	sort.Slice(scoredComments, func(i, j int) bool {
		return scoredComments[i].Score > scoredComments[j].Score
	})

	// Apply max comments limit
	maxComments := qs.config.MaxCommentsPerFile
	if len(scoredComments) > maxComments {
		scoredComments = scoredComments[:maxComments]
	}

	// Extract filtered comments
	var filteredComments []*model.ReviewAIComment
	for _, scored := range scoredComments {
		filteredComments = append(filteredComments, scored.Comment)
	}

	return filteredComments
}

// calculateQualityScore calculates the quality score for a review comment
func (qs *QualityScorer) calculateQualityScore(comment *model.ReviewAIComment, enhancedCtx *EnhancedContext) (float64, QualityFactors) {
	factors := QualityFactors{}

	// Severity score (0.0 - 1.0)
	factors.SeverityScore = qs.getSeverityScore(comment.Priority)

	// Confidence score (0.0 - 1.0)
	factors.ConfidenceScore = qs.getConfidenceScore(comment.Confidence)

	// Issue type score (0.0 - 1.0)
	factors.IssueTypeScore = qs.getIssueTypeScore(comment.IssueType)

	// Context score based on available context (0.0 - 1.0)
	factors.ContextScore = qs.getContextScore(comment, enhancedCtx)

	// Complexity score based on code analysis (0.0 - 1.0)
	factors.ComplexityScore = qs.getComplexityScore(comment)

	// Security bonus
	factors.SecurityScore = qs.getSecurityScore(comment, enhancedCtx)

	// Architecture bonus
	factors.ArchitectureScore = qs.getArchitectureScore(comment, enhancedCtx)

	// Calculate weighted total score
	baseScore := (factors.SeverityScore*0.3 +
		factors.ConfidenceScore*0.25 +
		factors.IssueTypeScore*0.25 +
		factors.ContextScore*0.1 +
		factors.ComplexityScore*0.1)

	// Apply bonuses
	totalScore := baseScore + factors.SecurityScore + factors.ArchitectureScore

	// Cap at 1.0
	if totalScore > 1.0 {
		totalScore = 1.0
	}

	return totalScore, factors
}

// getSeverityScore converts severity to numeric score
func (qs *QualityScorer) getSeverityScore(priority model.ReviewPriority) float64 {
	switch priority {
	case model.ReviewPriorityCritical:
		return 1.0
	case model.ReviewPriorityHigh:
		return 0.8
	case model.ReviewPriorityMedium:
		return 0.6
	case model.ReviewPriorityBacklog:
		return 0.4
	default:
		return 0.4
	}
}

// getConfidenceScore converts confidence to numeric score
func (qs *QualityScorer) getConfidenceScore(confidence model.ReviewConfidence) float64 {
	switch confidence {
	case model.ConfidenceVeryHigh:
		return 1.0
	case model.ConfidenceHigh:
		return 0.8
	case model.ConfidenceMedium:
		return 0.6
	case model.ConfidenceLow:
		return 0.4
	default:
		return 0.4
	}
}

// getIssueTypeScore assigns importance scores to different issue types
func (qs *QualityScorer) getIssueTypeScore(issueType model.IssueType) float64 {
	switch issueType {
	case model.IssueTypeCritical:
		return 1.0
	case model.IssueTypeSecurity:
		return 0.95
	case model.IssueTypeBug:
		return 0.8
	case model.IssueTypePerformance:
		return 0.7
	case model.IssueTypeRefactor:
		return 0.5
	case model.IssueTypeOther:
		return 0.3
	default:
		return 0.3
	}
}

// getContextScore evaluates how much context supports the issue
func (qs *QualityScorer) getContextScore(comment *model.ReviewAIComment, enhancedCtx *EnhancedContext) float64 {
	score := 0.5 // Base score

	// Boost score if we have semantic changes that align with the issue
	for _, change := range enhancedCtx.SemanticChanges {
		if qs.alignsWithSemanticChange(comment, change) {
			score += 0.2
			break
		}
	}

	// Boost score if it's in a function we have context for
	for _, fn := range enhancedCtx.FunctionSignatures {
		if strings.Contains(comment.Description, fn.Name) || strings.Contains(comment.Suggestion, fn.Name) {
			score += 0.1
			break
		}
	}

	// Boost score if security context supports security-related issues
	if comment.IssueType == model.IssueTypeSecurity && qs.hasRelevantSecurityContext(enhancedCtx) {
		score += 0.2
	}

	return minFloat64(score, 1.0)
}

// getComplexityScore evaluates the complexity of the suggested fix
func (qs *QualityScorer) getComplexityScore(comment *model.ReviewAIComment) float64 {
	// Higher scores for more substantial, thoughtful suggestions

	if len(comment.CodeSnippet) == 0 {
		return 0.2 // No code provided
	}

	codeLength := len(comment.CodeSnippet)
	suggestionLength := len(comment.Suggestion)

	// Good balance of explanation and code
	if suggestionLength > 100 && codeLength > 50 && codeLength < 500 {
		return 0.9
	}

	// Substantial code with decent explanation
	if suggestionLength > 50 && codeLength > 30 {
		return 0.7
	}

	// Basic code with explanation
	if suggestionLength > 30 && codeLength > 10 {
		return 0.5
	}

	return 0.3
}

// getSecurityScore provides bonus for security-related issues
func (qs *QualityScorer) getSecurityScore(comment *model.ReviewAIComment, enhancedCtx *EnhancedContext) float64 {
	if !qs.config.PrioritizeSecurityIssues {
		return 0.0
	}

	if comment.IssueType == model.IssueTypeSecurity {
		return 0.15 // Security bonus
	}

	// Check for security-related keywords in description
	securityKeywords := []string{"vulnerability", "security", "injection", "xss", "csrf", "authentication", "authorization"}
	description := strings.ToLower(comment.Description + " " + comment.Suggestion)

	for _, keyword := range securityKeywords {
		if strings.Contains(description, keyword) {
			return 0.1
		}
	}

	return 0.0
}

// getArchitectureScore provides bonus for architecture-related issues
func (qs *QualityScorer) getArchitectureScore(comment *model.ReviewAIComment, enhancedCtx *EnhancedContext) float64 {
	if !qs.config.PrioritizeHighImpact {
		return 0.0
	}

	// Check for architectural keywords
	archKeywords := []string{"architecture", "design pattern", "solid", "coupling", "cohesion", "dependency", "interface", "abstraction"}
	description := strings.ToLower(comment.Description + " " + comment.Suggestion)

	for _, keyword := range archKeywords {
		if strings.Contains(description, keyword) {
			return 0.1
		}
	}

	return 0.0
}

// Helper functions

func (qs *QualityScorer) alignsWithSemanticChange(comment *model.ReviewAIComment, change ContextSemanticChange) bool {
	// Check if the comment's line numbers overlap with semantic change lines
	for _, line := range change.Lines {
		if line == comment.Line || (comment.EndLine > 0 && line >= comment.Line && line <= comment.EndLine) {
			return true
		}
	}
	return false
}

func (qs *QualityScorer) hasRelevantSecurityContext(enhancedCtx *EnhancedContext) bool {
	ctx := enhancedCtx.SecurityContext
	return ctx.HasAuthenticationLogic || ctx.HandlesUserInput || ctx.AccessesDatabase ||
		ctx.NetworkOperations || ctx.CryptographicOperations
}

func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
