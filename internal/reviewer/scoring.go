package reviewer

import (
	"context"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/lang"
	"github.com/maxbolgarin/logze/v2"
)

// scoreAndFilterComments scores review comments and filters out low-quality ones
func (s *Reviewer) scoreAndFilterComments(ctx context.Context, comments []*model.ReviewAIComment, change *model.FileDiff, log logze.Logger) []*model.ReviewAIComment {
	if len(comments) == 0 {
		return comments
	}

	var scores []model.IssueScore

	switch s.cfg.Scoring.Mode {
	case ScoringModeCheap:
		// Use existing comment fields for "cheap" scoring
		scores = s.generateCheapScores(comments, log)
	case ScoringModeAI:
		// Use AI agent for detailed scoring
		var err error
		scores, err = s.agent.ScoreReviewComments(ctx, comments, change.NewPath, change.Diff)
		if err != nil {
			log.Warn("failed to score comments with AI, falling back to cheap scoring", "error", err)
			scores = s.generateCheapScores(comments, log)
		}
	default:
		log.Warn("unknown scoring mode, using cheap scoring", "mode", s.cfg.Scoring.Mode)
		scores = s.generateCheapScores(comments, log)
	}

	// Filter comments based on scores and configuration
	var filteredComments []*model.ReviewAIComment
	for i, comment := range comments {
		var score model.IssueScore
		if i < len(scores) {
			score = scores[i]
		} else {
			// Default score if scoring failed
			score = model.IssueScore{
				OverallScore:       0.5,
				SeverityScore:      0.5,
				ConfidenceScore:    0.5,
				RelevanceScore:     0.5,
				ActionabilityScore: 0.5,
				ShouldFilter:       false,
				FilterReason:       "default_score",
			}
		}

		// Apply filtering logic based on configuration thresholds
		shouldFilter := s.shouldFilterComment(score)

		log.DebugIf(s.cfg.Verbose,
			lang.If(shouldFilter, "throw comment after scoring", "accept comment after scoring"),
			"line", comment.Line,
			"scoring_mode", string(s.cfg.Scoring.Mode),
			"overall_score", score.OverallScore,
			"severity_score", score.SeverityScore,
			"confidence_score", score.ConfidenceScore,
			"relevance_score", score.RelevanceScore,
			"actionability_score", score.ActionabilityScore,
			"should_filter", shouldFilter,
			"filter_reason", score.FilterReason,
			"title", comment.Title)

		if !shouldFilter {
			filteredComments = append(filteredComments, comment)
		}
	}

	return filteredComments
}

// generateCheapScores creates scores based on existing comment fields without additional AI calls
func (s *Reviewer) generateCheapScores(comments []*model.ReviewAIComment, log logze.Logger) []model.IssueScore {
	scores := make([]model.IssueScore, len(comments))

	for i, comment := range comments {
		scores[i] = s.calculateCheapScore(comment)
	}

	return scores
}

// calculateCheapScore calculates a score based on existing comment fields
func (s *Reviewer) calculateCheapScore(comment *model.ReviewAIComment) model.IssueScore {
	// Map issue types to severity scores
	severityScore := s.mapIssueTypeToSeverity(comment.IssueType)

	// Map confidence levels to scores
	confidenceScore := s.mapConfidenceToScore(comment.Confidence)

	// Map priority levels to scores (affects overall calculation)
	priorityScore := s.mapPriorityToScore(comment.Priority)

	// Relevance score - assume high for cheap scoring (since we can't analyze context)
	relevanceScore := 0.8

	// Actionability score - estimate based on description length and suggestion presence
	actionabilityScore := s.estimateActionabilityScore(comment)

	// Calculate overall score with weights
	// Priority and severity have higher weight in cheap mode since they're explicit
	overallScore := (severityScore*0.35 + confidenceScore*0.25 +
		priorityScore*0.25 + relevanceScore*0.1 + actionabilityScore*0.05)

	// Determine if should filter based on simple rules
	shouldFilter, filterReason := s.shouldFilterCheapScore(comment, overallScore, confidenceScore, severityScore)

	return model.IssueScore{
		OverallScore:       overallScore,
		SeverityScore:      severityScore,
		ConfidenceScore:    confidenceScore,
		RelevanceScore:     relevanceScore,
		ActionabilityScore: actionabilityScore,
		ShouldFilter:       shouldFilter,
		FilterReason:       filterReason,
	}
}

// estimateActionabilityScore estimates actionability based on comment content
func (s *Reviewer) estimateActionabilityScore(comment *model.ReviewAIComment) float64 {
	score := 0.5 // Base score

	// Has suggestion = more actionable
	if comment.Suggestion != "" && len(comment.Suggestion) > 20 {
		score += 0.3
	}

	// Has code snippet = more actionable
	if comment.CodeSnippet != "" && len(comment.CodeSnippet) > 10 {
		score += 0.2
	}

	// Description length indicates effort/detail
	if len(comment.Description) > 50 {
		score += 0.1
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// shouldFilterCheapScore determines if a comment should be filtered in cheap mode
func (s *Reviewer) shouldFilterCheapScore(comment *model.ReviewAIComment, overallScore, confidenceScore, severityScore float64) (bool, string) {
	// Filter very low confidence issues
	if confidenceScore < 0.3 {
		return true, "very_low_confidence"
	}

	// Filter low priority + low confidence combination
	if comment.Priority == model.ReviewPriorityBacklog && confidenceScore < 0.6 {
		return true, "backlog_low_confidence"
	}

	// Filter very low overall score
	if overallScore < 0.25 {
		return true, "very_low_overall_score"
	}

	return false, ""
}

// shouldFilterComment determines if a comment should be filtered based on its score and configuration
func (s *Reviewer) shouldFilterComment(score model.IssueScore) bool {
	cfg := s.cfg.Scoring

	// If the AI explicitly marked it for filtering, respect that decision
	if score.ShouldFilter {
		return true
	}

	// Apply threshold-based filtering
	if score.OverallScore < cfg.MinOverallScore {
		return true
	}

	if score.SeverityScore < cfg.MinSeverityScore {
		return true
	}

	if score.ConfidenceScore < cfg.MinConfidenceScore {
		return true
	}

	if score.RelevanceScore < cfg.MinRelevanceScore {
		return true
	}

	if score.ActionabilityScore < cfg.MinActionabilityScore {
		return true
	}

	return false
}

// mapIssueTypeToSeverity maps issue types to severity scores
func (s *Reviewer) mapIssueTypeToSeverity(issueType model.IssueType) float64 {
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
		return 0.4
	case model.IssueTypeOther:
		return 0.3
	default:
		return 0.5
	}
}

// mapConfidenceToScore maps confidence levels to scores
func (s *Reviewer) mapConfidenceToScore(confidence model.ReviewConfidence) float64 {
	switch confidence {
	case model.ConfidenceVeryHigh:
		return 0.95
	case model.ConfidenceHigh:
		return 0.8
	case model.ConfidenceMedium:
		return 0.6
	case model.ConfidenceLow:
		return 0.3
	default:
		return 0.5
	}
}

// mapPriorityToScore maps priority levels to scores
func (s *Reviewer) mapPriorityToScore(priority model.ReviewPriority) float64 {
	switch priority {
	case model.ReviewPriorityCritical:
		return 1.0
	case model.ReviewPriorityHigh:
		return 0.8
	case model.ReviewPriorityMedium:
		return 0.6
	case model.ReviewPriorityBacklog:
		return 0.2
	default:
		return 0.5
	}
}
