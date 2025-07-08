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
