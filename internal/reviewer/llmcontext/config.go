package llmcontext

import (
	"path/filepath"
	"slices"
	"strings"
)

// Filter represents criteria for filtering files to review
type Filter struct {
	MaxFiles          int      `yaml:"max_files"`
	MaxFileSizeTokens int      `yaml:"max_file_size_tokens"`
	MaxOverallTokens  int      `yaml:"max_overall_tokens"` // TODO: count this
	AllowedExtensions []string `yaml:"allowed_extensions"`
	ExcludedPaths     []string `yaml:"excluded_paths"`
}

func (s *Filter) isAllowedExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return slices.Contains(s.AllowedExtensions, ext)
}

func (s *Filter) isExcludedPath(filePath string) bool {
	for _, pattern := range s.ExcludedPaths {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
	}
	return false
}
