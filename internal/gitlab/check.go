package gitlab

import (
	"path/filepath"
	"strings"
)

var (
	// Code file extensions
	codeExtensions = map[string]bool{
		".go":     true,
		".py":     true,
		".js":     true,
		".ts":     true,
		".jsx":    true,
		".tsx":    true,
		".java":   true,
		".cpp":    true,
		".cc":     true,
		".cxx":    true,
		".c":      true,
		".h":      true,
		".hpp":    true,
		".cs":     true,
		".php":    true,
		".rb":     true,
		".swift":  true,
		".kt":     true,
		".kts":    true,
		".rs":     true,
		".scala":  true,
		".clj":    true,
		".cljs":   true,
		".hs":     true,
		".ml":     true,
		".fs":     true,
		".elm":    true,
		".dart":   true,
		".lua":    true,
		".r":      true,
		".jl":     true,
		".nim":    true,
		".zig":    true,
		".cr":     true,
		".ex":     true,
		".exs":    true,
		".erl":    true,
		".hrl":    true,
		".pl":     true,
		".pm":     true,
		".ps1":    true,
		".sh":     true,
		".bat":    true,
		".cmd":    true,
		".sql":    true,
		".html":   true,
		".htm":    true,
		".css":    true,
		".scss":   true,
		".sass":   true,
		".less":   true,
		".vue":    true,
		".svelte": true,
		".astro":  true,
	}

	// CI/Config file extensions
	configExtensions = map[string]bool{
		".yml":  true,
		".yaml": true,
		".json": true,
		".toml": true,
		".ini":  true,
		".conf": true,
		".env":  true,
	}

	configFileNames = []string{
		"gitlab",
		"golangci",
		"deploy",
		"cm",
		"ingress",
		"service",
		"job",
		"provisioning",
		"rbac",
		"pv",
		"secret",
		"role",
		"sa",
		"package",
		"helm",
		"kustomize",
		"k8s",
		"kubernetes",
		"vite",
		"tsconfig",
		"docker",
	}

	// Special filenames (without extension)
	specialFiles = map[string]bool{
		"dockerfile": true,
		"makefile":   true,
	}
)

// isCodeFile determines if a file should be processed based on its extension
func isCodeFile(filePath string) bool {
	if filePath == "" {
		return false
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	fileName := strings.ToLower(filepath.Base(filePath))
	fileNameWithoutExt := strings.TrimSuffix(fileName, ext) // Получаем имя без расширения

	// Check by extension
	if codeExtensions[ext] {
		return true
	}

	if configExtensions[ext] {
		for _, name := range configFileNames {
			if strings.Contains(fileNameWithoutExt, name) {
				return true
			}
		}
		return false
	}

	// Check special filenames
	if specialFiles[fileName] {
		return true
	}

	return false
}
