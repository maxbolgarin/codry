package reviewer

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/lang"
)

const (
	defaultMaxFiles          = 100
	defaultMaxFileSizeTokens = 1_000_000
	defaultPoolSize          = 100
	defaultLanguage          = model.LanguageEnglish
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
	Generate GenerateConfig `yaml:"generate"`
	Filter   Filter         `yaml:"filter"`
	Scoring  ScoringConfig  `yaml:"scoring"`

	Verbose          bool          `yaml:"verbose" env:"REVIEW_VERBOSE"`
	SingleReviewMode bool          `yaml:"single_mode" env:"REVIEW_SINGLE_MODE"`
	TrackingTTL      time.Duration `yaml:"tracking_ttl"`

	// Get from code
	Language model.Language `yaml:"-"`
}

type GenerateConfig struct {
	// Generate reviews
	Description        bool `yaml:"description" env:"REVIEW_DESCRIPTION"`
	ChangesOverview    bool `yaml:"changes_overview" env:"REVIEW_CHANGES_OVERVIEW"`
	ArchitectureReview bool `yaml:"architecture" env:"REVIEW_ARCHITECTURE"`
	CodeReview         bool `yaml:"code" env:"REVIEW_CODE"`
}

// Filter represents criteria for filtering files to review
type Filter struct {
	MaxFiles          int      `yaml:"max_files"`
	MaxFileSizeTokens int      `yaml:"max_file_size_tokens"`
	MaxOverallTokens  int      `yaml:"max_overall_tokens"` // TODO: count this
	AllowedExtensions []string `yaml:"allowed_extensions"`
	ExcludedPaths     []string `yaml:"excluded_paths"`
}

// ScoringMode defines the scoring strategy
type ScoringMode string

const (
	ScoringModeStrict     ScoringMode = "strict"     // Use strict scoring configuration
	ScoringModeMedium     ScoringMode = "medium"     // Use medium scoring configuration
	ScoringModeEverything ScoringMode = "everything" // Use everything scoring configuration
)

// ScoringConfig represents configuration for the issue scoring model
type ScoringConfig struct {
	// Scoring mode: "strict", "medium", "everything"
	Mode ScoringMode `yaml:"mode" env:"REVIEW_SCORING_MODE"`

	// Allowed issue types, impacts, fix priorities, and model confidences
	IssueTypes       []model.IssueType       `yaml:"issue_types"` // TODO: remove this
	IssueImpacts     []model.IssueImpact     `yaml:"issue_impacts"`
	FixPriorities    []model.FixPriority     `yaml:"fix_priorities"`
	ModelConfidences []model.ModelConfidence `yaml:"model_confidences"`
}

func (c *Config) PrepareAndValidate() error {
	c.Language = lang.Check(c.Language, defaultLanguage)
	c.TrackingTTL = lang.Check(c.TrackingTTL, 24*time.Hour)
	c.Scoring.Mode = lang.Check(c.Scoring.Mode, ScoringModeEverything)

	c.Filter.MaxFiles = lang.Check(c.Filter.MaxFiles, defaultMaxFiles)
	c.Filter.MaxFileSizeTokens = lang.Check(c.Filter.MaxFileSizeTokens, defaultMaxFileSizeTokens)

	switch c.Scoring.Mode {
	case ScoringModeStrict:
		c.Scoring = strictScoringConfig()
	case ScoringModeMedium:
		c.Scoring = mediumScoringConfig()
	case ScoringModeEverything:
		fallthrough
	default:
		c.Scoring = everythingScoringConfig()
	}

	return nil
}

// strictScoringConfig returns strict scoring configuration
func strictScoringConfig() ScoringConfig {
	return ScoringConfig{
		Mode: ScoringModeStrict,
		IssueTypes: []model.IssueType{
			model.IssueTypeFailure,
			model.IssueTypeBug,
			model.IssueTypeSecurity,
		},
		IssueImpacts: []model.IssueImpact{
			model.IssueImpactCritical,
			model.IssueImpactHigh,
		},
		FixPriorities: []model.FixPriority{
			model.FixPriorityHotfix,
			model.FixPriorityFirst,
		},
		ModelConfidences: []model.ModelConfidence{
			model.ModelConfidenceVeryHigh,
			model.ModelConfidenceHigh,
		},
	}
}

// mediumScoringConfig returns medium scoring configuration
func mediumScoringConfig() ScoringConfig {
	return ScoringConfig{
		Mode: ScoringModeMedium,
		IssueImpacts: []model.IssueImpact{
			model.IssueImpactCritical,
			model.IssueImpactHigh,
			model.IssueImpactMedium,
		},
		FixPriorities: []model.FixPriority{
			model.FixPriorityHotfix,
			model.FixPriorityFirst,
			model.FixPrioritySecond,
		},
		ModelConfidences: []model.ModelConfidence{
			model.ModelConfidenceVeryHigh,
			model.ModelConfidenceHigh,
			model.ModelConfidenceMedium,
		},
	}
}

// everythingScoringConfig returns everything scoring configuration
func everythingScoringConfig() ScoringConfig {
	return ScoringConfig{
		Mode: ScoringModeEverything,
	}
}

func (s *Config) isAllowedExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return slices.Contains(s.Filter.AllowedExtensions, ext)
}

func (s *Config) isExcludedPath(filePath string) bool {
	for _, pattern := range s.Filter.ExcludedPaths {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}
