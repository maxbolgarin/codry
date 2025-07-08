package reviewer

import (
	"time"

	"github.com/maxbolgarin/codry/internal/model"
)

const (
	startMarkerDesc = "<!-- Codry: ai-desc-start -->"
	endMarkerDesc   = "<!-- Codry: ai-desc-end -->"

	startMarkerOverview = "<!-- Codry: ai-overview-start -->"
	endMarkerOverview   = "<!-- Codry: ai-overview-end -->"

	startMarkerArchitecture = "<!-- Codry: ai-architecture-start -->"
	endMarkerArchitecture   = "<!-- Codry: ai-architecture-end -->"
)

type Config struct {
	FileFilter             FileFilter    `yaml:"file_filter"`
	MaxFilesPerMR          int           `yaml:"max_files_per_mr" env:"REVIEW_MAX_FILES_PER_MR"`
	MinFilesForDescription int           `yaml:"min_files_for_description" env:"REVIEW_MIN_FILES_FOR_DESCRIPTION"`
	ProcessingDelay        time.Duration `yaml:"processing_delay" env:"REVIEW_PROCESSING_DELAY"`

	UpdateDescriptionOnMR           bool `yaml:"update_description_on_mr" env:"REVIEW_UPDATE_DESCRIPTION_ON_MR"`
	EnableDescriptionGeneration     bool `yaml:"enable_description_generation" env:"REVIEW_ENABLE_DESCRIPTION_GENERATION"`
	EnableChangesOverviewGeneration bool `yaml:"enable_changes_overview_generation" env:"REVIEW_ENABLE_CHANGES_OVERVIEW_GENERATION"`
	EnableArchitectureReview        bool `yaml:"enable_architecture_review" env:"REVIEW_ENABLE_ARCHITECTURE_REVIEW"`
	EnableCodeReview                bool `yaml:"enable_code_review" env:"REVIEW_ENABLE_CODE_REVIEW"`

	// Scoring configuration for filtering low-quality issues
	Scoring ScoringConfig `yaml:"scoring"`

	// Single review tracking - only review each MR once unless there are new changes
	EnableSingleReviewMode bool          `yaml:"enable_single_review_mode" env:"REVIEW_ENABLE_SINGLE_REVIEW_MODE"`
	ReviewTrackingTTL      time.Duration `yaml:"review_tracking_ttl" env:"REVIEW_TRACKING_TTL"`

	Language model.Language `yaml:"language" env:"REVIEW_LANGUAGE"`
	Verbose  bool           `yaml:"verbose" env:"REVIEW_VERBOSE"`
}

// FileFilter represents criteria for filtering files to review
type FileFilter struct {
	MaxFileSize       int      `yaml:"max_file_size" env:"REVIEW_FILE_FILTER_MAX_FILE_SIZE"`
	AllowedExtensions []string `yaml:"allowed_extensions" env:"REVIEW_FILE_FILTER_ALLOWED_EXTENSIONS"`
	ExcludedPaths     []string `yaml:"excluded_paths" env:"REVIEW_FILE_FILTER_EXCLUDED_PATHS"`
	IncludeOnlyCode   bool     `yaml:"include_only_code" env:"REVIEW_FILE_FILTER_INCLUDE_ONLY_CODE"`
}

// ScoringMode defines the scoring strategy
type ScoringMode string

const (
	ScoringModeDisabled ScoringMode = "disabled" // No scoring/filtering
	ScoringModeCheap    ScoringMode = "cheap"    // Use existing comment fields for scoring
	ScoringModeAI       ScoringMode = "ai"       // Use additional AI prompt for scoring
)

// ScoringConfig represents configuration for the issue scoring model
type ScoringConfig struct {
	// Scoring mode: "disabled", "cheap", or "ai"
	Mode ScoringMode `yaml:"mode" env:"SCORING_MODE"`

	// Minimum scores for issues to not be filtered
	MinOverallScore       float64 `yaml:"min_overall_score" env:"SCORING_MIN_OVERALL"`
	MinSeverityScore      float64 `yaml:"min_severity_score" env:"SCORING_MIN_SEVERITY"`
	MinConfidenceScore    float64 `yaml:"min_confidence_score" env:"SCORING_MIN_CONFIDENCE"`
	MinRelevanceScore     float64 `yaml:"min_relevance_score" env:"SCORING_MIN_RELEVANCE"`
	MinActionabilityScore float64 `yaml:"min_actionability_score" env:"SCORING_MIN_ACTIONABILITY"`

	// Verbose scoring for debugging
	VerboseScoring bool `yaml:"verbose_scoring" env:"SCORING_VERBOSE"`
}

// defaultScoringConfig returns default scoring configuration
func defaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		Mode:                  ScoringModeCheap, // Default to cheap scoring
		MinOverallScore:       0.3,              // Filter out issues with overall score < 0.3
		MinSeverityScore:      0.2,              // Allow info-level issues if they're otherwise good
		MinConfidenceScore:    0.4,              // Filter out low-confidence issues
		MinRelevanceScore:     0.3,              // Filter out issues not relevant to changes
		MinActionabilityScore: 0.3,              // Filter out vague, non-actionable feedback
		VerboseScoring:        false,
	}
}
