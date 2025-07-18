package astparser

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	sitter "github.com/smacker/go-tree-sitter"
)

// TestInfo represents information about test file structure
type TestInfo struct {
	TestCount     int      `json:"test_count"`
	TestFunctions []string `json:"test_functions"`
	TestSuites    []string `json:"test_suites"`
	TestType      string   `json:"test_type"` // "unit", "integration", "e2e"
}

// Helper methods for special cases analysis

// extractContentFromDiff extracts file content from added lines in diff
func (sch *SpecialCasesHandler) extractContentFromDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var content []string

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			content = append(content, line[1:]) // Remove + prefix
		}
	}

	return strings.Join(content, "\n")
}

// extractRemovedContentFromDiff extracts removed content from diff
func (sch *SpecialCasesHandler) extractRemovedContentFromDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var content []string

	for _, line := range lines {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			content = append(content, line[1:]) // Remove - prefix
		}
	}

	return strings.Join(content, "\n")
}

// findAllSymbolsInFile finds all symbols in a file using AST parsing
func (sch *SpecialCasesHandler) findAllSymbolsInFile(filePath, content string) ([]AffectedSymbol, error) {
	if content == "" {
		return []AffectedSymbol{}, nil
	}

	rootNode, err := sch.astParser.GetFileAST(context.Background(), filePath, content)
	if err != nil {
		return nil, err
	}

	var symbols []AffectedSymbol
	sch.walkASTForSymbols(rootNode, filePath, content, &symbols)

	return symbols, nil
}

// walkASTForSymbols walks AST to find all symbols
func (sch *SpecialCasesHandler) walkASTForSymbols(node *sitter.Node, filePath, content string, symbols *[]AffectedSymbol) {
	if sch.astParser.IsSymbolNode(node.Type()) {
		symbol := sch.astParser.ExtractSymbolFromNode(node, filePath, content)
		if symbol.Name != "" {
			*symbols = append(*symbols, symbol)
		}
	}

	// Recursively check children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			sch.walkASTForSymbols(child, filePath, content, symbols)
		}
	}
}

// identifyExportedSymbols identifies symbols that are exported/public
func (sch *SpecialCasesHandler) identifyExportedSymbols(symbols []AffectedSymbol) []AffectedSymbol {
	var exported []AffectedSymbol

	for _, symbol := range symbols {
		if sch.isSymbolExported(symbol) {
			exported = append(exported, symbol)
		}
	}

	return exported
}

// isSymbolExported checks if a symbol is exported based on naming conventions
func (sch *SpecialCasesHandler) isSymbolExported(symbol AffectedSymbol) bool {
	// Go naming convention: exported symbols start with uppercase
	if strings.HasSuffix(symbol.FilePath, ".go") {
		return len(symbol.Name) > 0 && symbol.Name[0] >= 'A' && symbol.Name[0] <= 'Z'
	}

	// JavaScript/TypeScript: look for export keyword in signature
	if strings.HasSuffix(symbol.FilePath, ".js") || strings.HasSuffix(symbol.FilePath, ".ts") {
		return strings.Contains(strings.Split(symbol.FullCode, "\n")[0], "export")
	}

	// Python: symbols not starting with underscore are generally public
	if strings.HasSuffix(symbol.FilePath, ".py") {
		return !strings.HasPrefix(symbol.Name, "_")
	}

	// Default: assume exported
	return true
}

// extractDependencies extracts dependencies from file content
func (sch *SpecialCasesHandler) extractDependencies(content, filePath string) []string {
	var dependencies []string

	// Extract import statements based on file type
	if strings.HasSuffix(filePath, ".go") {
		dependencies = append(dependencies, sch.extractGoDependencies(content)...)
	} else if strings.HasSuffix(filePath, ".js") || strings.HasSuffix(filePath, ".ts") {
		dependencies = append(dependencies, sch.extractJSDependencies(content)...)
	} else if strings.HasSuffix(filePath, ".py") {
		dependencies = append(dependencies, sch.extractPythonDependencies(content)...)
	}

	return sch.removeDuplicates(dependencies)
}

// extractGoDependencies extracts Go import dependencies
func (sch *SpecialCasesHandler) extractGoDependencies(content string) []string {
	var deps []string

	// Simple regex to find import statements
	importRegex := regexp.MustCompile(`import\s+"([^"]+)"`)
	matches := importRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			deps = append(deps, match[1])
		}
	}

	// Handle multi-line imports
	blockImportRegex := regexp.MustCompile(`import\s*\(\s*([^)]+)\s*\)`)
	blockMatches := blockImportRegex.FindAllStringSubmatch(content, -1)

	for _, blockMatch := range blockMatches {
		if len(blockMatch) > 1 {
			lines := strings.Split(blockMatch[1], "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "\"") && strings.HasSuffix(line, "\"") {
					dep := strings.Trim(line, "\"")
					deps = append(deps, dep)
				}
			}
		}
	}

	return deps
}

// extractJSDependencies extracts JavaScript/TypeScript dependencies
func (sch *SpecialCasesHandler) extractJSDependencies(content string) []string {
	var deps []string

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`import.*from\s+['"]([^'"]+)['"]`),
		regexp.MustCompile(`require\(['"]([^'"]+)['"]\)`),
		regexp.MustCompile(`import\(['"]([^'"]+)['"]\)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				deps = append(deps, match[1])
			}
		}
	}

	return deps
}

// extractPythonDependencies extracts Python import dependencies
func (sch *SpecialCasesHandler) extractPythonDependencies(content string) []string {
	var deps []string

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`import\s+([^\s\n]+)`),
		regexp.MustCompile(`from\s+([^\s\n]+)\s+import`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				deps = append(deps, match[1])
			}
		}
	}

	return deps
}

// findPotentialConsumers finds files that might consume the new file
func (sch *SpecialCasesHandler) findPotentialConsumers(ctx context.Context, request model.ReviewRequest, filePath string, exportedSymbols []AffectedSymbol) ([]string, error) {
	// This is a simplified implementation
	// In practice, you would search the codebase for import statements or references
	var consumers []string

	// Look for files that might import this new file
	packageName := sch.extractPackageFromPath(filePath)
	if packageName != "" {
		// Search for imports of this package (implementation would vary by language)
		// For now, return empty slice
	}

	return consumers, nil
}

// getBaseFileContent gets file content from the base branch
func (sch *SpecialCasesHandler) getBaseFileContent(ctx context.Context, request model.ReviewRequest, filePath string) (string, error) {
	// This would need to get the file from the base branch
	// For now, we'll try to get it from the current commit as a fallback
	return sch.provider.GetFileContent(ctx, request.ProjectID, filePath, request.MergeRequest.SHA)
}

// findBrokenReferences finds references to symbols that no longer exist
func (sch *SpecialCasesHandler) findBrokenReferences(ctx context.Context, request model.ReviewRequest, deletedSymbols []AffectedSymbol) ([]BrokenReference, error) {
	var brokenRefs []BrokenReference

	// This would search the codebase for references to the deleted symbols
	// // Implementation would depend on having a search mechanism

	// for _, symbol := range deletedSymbols {
	// 	// Search for usages of this symbol (simplified)
	// 	usages, err := sch.symbolAnalyzer.FindSymbolCallers(ctx, request.RepoDataHead, symbol)
	// 	if err != nil {
	// 		continue
	// 	}

	// 	// Convert callers to broken references
	// 	for _, caller := range usages.Callers {
	// 		brokenRef := BrokenReference{
	// 			FilePath:         caller.FilePath,
	// 			LineNumber:       caller.LineNumber,
	// 			ReferencedSymbol: symbol.Name,
	// 			Context:          caller.CodeSnippet,
	// 			Severity:         "error",
	// 		}
	// 		brokenRefs = append(brokenRefs, brokenRef)
	// 	}
	// }

	return brokenRefs, nil
}

// parseConfigChanges parses configuration file changes
func (sch *SpecialCasesHandler) parseConfigChanges(diff, configFormat string) []ConfigSetting {
	var settings []ConfigSetting

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			setting := sch.parseConfigLine(line, configFormat)
			if setting.Key != "" {
				settings = append(settings, setting)
			}
		}
	}

	return settings
}

// parseConfigLine parses a single configuration line
func (sch *SpecialCasesHandler) parseConfigLine(line, configFormat string) ConfigSetting {
	setting := ConfigSetting{}

	// Remove diff prefix
	content := line[1:]
	changeType := "added"
	if strings.HasPrefix(line, "-") {
		changeType = "removed"
	}

	// Parse based on format
	switch configFormat {
	case "yaml", "yml":
		setting = sch.parseYAMLConfigLine(content, changeType)
	case "json":
		setting = sch.parseJSONConfigLine(content, changeType)
	case "env":
		setting = sch.parseEnvConfigLine(content, changeType)
	}

	return setting
}

// parseYAMLConfigLine parses a YAML configuration line
func (sch *SpecialCasesHandler) parseYAMLConfigLine(line, changeType string) ConfigSetting {
	setting := ConfigSetting{ChangeType: changeType}

	// Simple YAML parsing: key: value
	if colonIndex := strings.Index(line, ":"); colonIndex != -1 {
		key := strings.TrimSpace(line[:colonIndex])
		value := strings.TrimSpace(line[colonIndex+1:])

		setting.Key = key
		if changeType == "added" {
			setting.NewValue = value
		} else {
			setting.OldValue = value
		}

		setting.Impact = sch.assessConfigSettingImpact(key, value)
	}

	return setting
}

// parseJSONConfigLine parses a JSON configuration line
func (sch *SpecialCasesHandler) parseJSONConfigLine(line, changeType string) ConfigSetting {
	setting := ConfigSetting{ChangeType: changeType}

	// Simple JSON parsing: "key": value
	if strings.Contains(line, "\":") {
		parts := strings.Split(line, "\":")
		if len(parts) >= 2 {
			key := strings.Trim(parts[0], "\"")
			value := strings.TrimSpace(parts[1])
			value = strings.TrimSuffix(value, ",")

			setting.Key = key
			if changeType == "added" {
				setting.NewValue = value
			} else {
				setting.OldValue = value
			}

			setting.Impact = sch.assessConfigSettingImpact(key, value)
		}
	}

	return setting
}

// parseEnvConfigLine parses an environment variable configuration line
func (sch *SpecialCasesHandler) parseEnvConfigLine(line, changeType string) ConfigSetting {
	setting := ConfigSetting{ChangeType: changeType}

	// Environment format: KEY=value
	if eqIndex := strings.Index(line, "="); eqIndex != -1 {
		key := strings.TrimSpace(line[:eqIndex])
		value := strings.TrimSpace(line[eqIndex+1:])

		setting.Key = key
		if changeType == "added" {
			setting.NewValue = value
		} else {
			setting.OldValue = value
		}

		setting.Impact = sch.assessConfigSettingImpact(key, value)
	}

	return setting
}

// findConfigConsumers finds components that consume configuration
func (sch *SpecialCasesHandler) findConfigConsumers(ctx context.Context, request model.ReviewRequest, configPath string) ([]ComponentReference, error) {
	var components []ComponentReference

	// This would search for files that read this configuration file
	// Implementation would depend on search capabilities

	_ = filepath.Base(configPath) // configFileName - would be used in real implementation

	// Simple heuristic: look for files that mention the config file name
	// In a real implementation, you would search the codebase

	return components, nil
}

// detectConfigFormat detects the format of a configuration file
func (sch *SpecialCasesHandler) detectConfigFormat(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	filename := strings.ToLower(filepath.Base(filePath))

	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".ini":
		return "ini"
	case ".env":
		return "env"
	case ".properties":
		return "properties"
	default:
		if filename == "makefile" {
			return "makefile"
		}
		if filename == "dockerfile" {
			return "dockerfile"
		}
		return "unknown"
	}
}

// assessConfigBackwardsCompatibility assesses if config changes are backwards compatible
func (sch *SpecialCasesHandler) assessConfigBackwardsCompatibility(settings []ConfigSetting) bool {
	for _, setting := range settings {
		if setting.ChangeType == "removed" {
			return false // Removing settings breaks backwards compatibility
		}

		// Changing certain critical settings might break compatibility
		criticalKeys := []string{"database", "host", "port", "api_key", "secret"}
		for _, key := range criticalKeys {
			if strings.Contains(strings.ToLower(setting.Key), key) && setting.ChangeType == "modified" {
				return false
			}
		}
	}

	return true
}

// Assessment and recommendation methods

func (sch *SpecialCasesHandler) assessNewFileImpact(exportedSymbols []AffectedSymbol, dependencies []string, consumers []string) string {
	if len(exportedSymbols) == 0 {
		return "Low impact - no exported symbols"
	}

	if len(consumers) > 0 {
		return fmt.Sprintf("High impact - %d exported symbols with %d potential consumers", len(exportedSymbols), len(consumers))
	}

	return fmt.Sprintf("Medium impact - %d exported symbols available for use", len(exportedSymbols))
}

func (sch *SpecialCasesHandler) assessDeletedFileImpact(symbols []AffectedSymbol, brokenRefs []BrokenReference) string {
	if len(brokenRefs) == 0 {
		return fmt.Sprintf("Low impact - %d symbols deleted with no broken references", len(symbols))
	}

	return fmt.Sprintf("High impact - %d symbols deleted with %d broken references", len(symbols), len(brokenRefs))
}

func (sch *SpecialCasesHandler) assessConfigFileImpact(settings []ConfigSetting, components []ComponentReference, backwardsCompatible bool) string {
	if !backwardsCompatible {
		return fmt.Sprintf("High impact - %d settings changed, breaking backwards compatibility", len(settings))
	}

	if len(components) > 5 {
		return fmt.Sprintf("Medium impact - %d settings changed affecting %d components", len(settings), len(components))
	}

	return fmt.Sprintf("Low impact - %d settings changed", len(settings))
}

func (sch *SpecialCasesHandler) assessConfigSettingImpact(key, value string) string {
	lowerKey := strings.ToLower(key)

	criticalKeys := []string{"database", "password", "secret", "api_key", "host", "port"}
	for _, criticalKey := range criticalKeys {
		if strings.Contains(lowerKey, criticalKey) {
			return "high"
		}
	}

	importantKeys := []string{"timeout", "retry", "cache", "log", "feature", "flag"}
	for _, importantKey := range importantKeys {
		if strings.Contains(lowerKey, importantKey) {
			return "medium"
		}
	}

	return "low"
}

func (sch *SpecialCasesHandler) determineBinaryType(fileExt, filePath string) string {
	switch fileExt {
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return "image"
	case ".pdf", ".doc", ".docx":
		return "document"
	case ".exe", ".bin", ".dll", ".so":
		return "executable"
	case ".zip", ".tar", ".gz", ".7z":
		return "archive"
	case ".mp4", ".avi", ".mov":
		return "video"
	case ".mp3", ".wav", ".flac":
		return "audio"
	default:
		return "unknown"
	}
}

func (sch *SpecialCasesHandler) assessBinaryFileImpact(binaryType string) string {
	switch binaryType {
	case "executable":
		return "High - executable changes affect runtime behavior"
	case "image", "document":
		return "Low - asset changes don't affect functionality"
	default:
		return "Medium - binary file changes may affect application behavior"
	}
}

// Test analysis methods

func (sch *SpecialCasesHandler) analyzeTestStructure(content, filePath string) TestInfo {
	testInfo := TestInfo{
		TestType: sch.determineTestType(filePath),
	}

	// Find test functions based on file type
	if strings.HasSuffix(filePath, ".go") {
		testInfo.TestFunctions = sch.findGoTestFunctions(content)
	} else if strings.HasSuffix(filePath, ".js") || strings.HasSuffix(filePath, ".ts") {
		testInfo.TestFunctions = sch.findJSTestFunctions(content)
	} else if strings.HasSuffix(filePath, ".py") {
		testInfo.TestFunctions = sch.findPythonTestFunctions(content)
	}

	testInfo.TestCount = len(testInfo.TestFunctions)

	return testInfo
}

func (sch *SpecialCasesHandler) determineTestType(filePath string) string {
	path := strings.ToLower(filePath)

	if strings.Contains(path, "integration") || strings.Contains(path, "e2e") {
		return "integration"
	}

	if strings.Contains(path, "unit") {
		return "unit"
	}

	return "unit" // Default assumption
}

func (sch *SpecialCasesHandler) findGoTestFunctions(content string) []string {
	testRegex := regexp.MustCompile(`func\s+(Test\w+|Benchmark\w+|Example\w+)\s*\(`)
	matches := testRegex.FindAllStringSubmatch(content, -1)

	var functions []string
	for _, match := range matches {
		if len(match) > 1 {
			functions = append(functions, match[1])
		}
	}

	return functions
}

func (sch *SpecialCasesHandler) findJSTestFunctions(content string) []string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`test\s*\(\s*['"]([^'"]+)['"]`),
		regexp.MustCompile(`it\s*\(\s*['"]([^'"]+)['"]`),
		regexp.MustCompile(`describe\s*\(\s*['"]([^'"]+)['"]`),
	}

	var functions []string
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				functions = append(functions, match[1])
			}
		}
	}

	return functions
}

func (sch *SpecialCasesHandler) findPythonTestFunctions(content string) []string {
	testRegex := regexp.MustCompile(`def\s+(test_\w+)\s*\(`)
	matches := testRegex.FindAllStringSubmatch(content, -1)

	var functions []string
	for _, match := range matches {
		if len(match) > 1 {
			functions = append(functions, match[1])
		}
	}

	return functions
}

func (sch *SpecialCasesHandler) findTestedFiles(testFilePath, content string) []string {
	var testedFiles []string

	// Simple heuristic: look for import statements and derive tested files
	dependencies := sch.extractDependencies(content, testFilePath)

	for _, dep := range dependencies {
		// If it's a local import, it might be the file being tested
		if !strings.Contains(dep, "/") && !strings.Contains(dep, ".") {
			// Convert test file path to source file path
			sourceFile := sch.convertTestPathToSourcePath(testFilePath)
			if sourceFile != "" {
				testedFiles = append(testedFiles, sourceFile)
			}
		}
	}

	return testedFiles
}

func (sch *SpecialCasesHandler) convertTestPathToSourcePath(testPath string) string {
	// Simple conversion heuristics
	if strings.Contains(testPath, "_test.go") {
		return strings.Replace(testPath, "_test.go", ".go", 1)
	}

	if strings.Contains(testPath, ".test.js") {
		return strings.Replace(testPath, ".test.js", ".js", 1)
	}

	if strings.Contains(testPath, "test_") && strings.HasSuffix(testPath, ".py") {
		base := filepath.Base(testPath)
		if strings.HasPrefix(base, "test_") {
			sourceFile := strings.TrimPrefix(base, "test_")
			return filepath.Join(filepath.Dir(testPath), sourceFile)
		}
	}

	return ""
}

func (sch *SpecialCasesHandler) assessTestFileImpact(testInfo TestInfo) string {
	if testInfo.TestCount == 0 {
		return "Low impact - no test cases found"
	}

	if testInfo.TestType == "integration" {
		return fmt.Sprintf("High impact - %d integration tests", testInfo.TestCount)
	}

	return fmt.Sprintf("Medium impact - %d test cases", testInfo.TestCount)
}

// Recommendation generation methods

func (sch *SpecialCasesHandler) generateNewFileRecommendations(exportedSymbols []AffectedSymbol, dependencies []string, consumers []string) []string {
	var recommendations []string

	if len(exportedSymbols) > 0 {
		recommendations = append(recommendations, "Add documentation for exported symbols")
		recommendations = append(recommendations, "Consider adding unit tests for new functionality")
	}

	if len(dependencies) > 10 {
		recommendations = append(recommendations, "High number of dependencies - consider reducing coupling")
	}

	recommendations = append(recommendations, "Verify that the new file follows project conventions")

	return recommendations
}

func (sch *SpecialCasesHandler) generateDeletedFileRecommendations(symbols []AffectedSymbol, brokenRefs []BrokenReference, migrationRequired bool) []string {
	var recommendations []string

	if len(brokenRefs) > 0 {
		recommendations = append(recommendations, "Fix all broken references before merging")
		recommendations = append(recommendations, "Consider providing migration guide for deprecated functionality")
	}

	if migrationRequired {
		recommendations = append(recommendations, "Create migration documentation")
	}

	recommendations = append(recommendations, "Update any documentation that references deleted functionality")

	return recommendations
}

func (sch *SpecialCasesHandler) generateConfigFileRecommendations(settings []ConfigSetting, components []ComponentReference, backwardsCompatible bool) []string {
	var recommendations []string

	if !backwardsCompatible {
		recommendations = append(recommendations, "Document breaking changes in configuration")
		recommendations = append(recommendations, "Provide migration path for existing configurations")
	}

	if len(components) > 0 {
		recommendations = append(recommendations, "Test configuration changes with affected components")
	}

	recommendations = append(recommendations, "Update configuration documentation")
	recommendations = append(recommendations, "Consider versioning configuration schema")

	return recommendations
}

func (sch *SpecialCasesHandler) generateBinaryFileRecommendations(binaryType string) []string {
	var recommendations []string

	switch binaryType {
	case "executable":
		recommendations = append(recommendations, "Verify executable is from trusted source")
		recommendations = append(recommendations, "Document purpose and version of executable")
	case "image", "document":
		recommendations = append(recommendations, "Optimize file size if possible")
		recommendations = append(recommendations, "Ensure appropriate licensing for assets")
	default:
		recommendations = append(recommendations, "Document purpose of binary file")
	}

	return recommendations
}

func (sch *SpecialCasesHandler) generateTestFileRecommendations(testInfo TestInfo) []string {
	var recommendations []string

	if testInfo.TestCount == 0 {
		recommendations = append(recommendations, "Add test cases to improve coverage")
	}

	if testInfo.TestType == "unit" {
		recommendations = append(recommendations, "Ensure tests are isolated and fast")
	} else if testInfo.TestType == "integration" {
		recommendations = append(recommendations, "Verify test environment setup")
		recommendations = append(recommendations, "Consider test data management")
	}

	recommendations = append(recommendations, "Follow naming conventions for test functions")

	return recommendations
}

// Issue identification methods

func (sch *SpecialCasesHandler) identifyNewFileIssues(symbols []AffectedSymbol, dependencies []string) []string {
	var issues []string

	if len(symbols) > 20 {
		issues = append(issues, "Large file with many symbols - consider splitting")
	}

	if len(dependencies) > 15 {
		issues = append(issues, "High number of dependencies - tight coupling")
	}

	// Check for potential naming issues
	for _, symbol := range symbols {
		if len(symbol.Name) < 2 {
			issues = append(issues, fmt.Sprintf("Symbol '%s' has very short name", symbol.Name))
		}
	}

	return issues
}

func (sch *SpecialCasesHandler) identifyDeletedFileIssues(brokenRefs []BrokenReference) []string {
	var issues []string

	errorCount := 0
	for _, ref := range brokenRefs {
		if ref.Severity == "error" {
			errorCount++
		}
	}

	if errorCount > 0 {
		issues = append(issues, fmt.Sprintf("%d broken references will cause compilation errors", errorCount))
	}

	return issues
}

func (sch *SpecialCasesHandler) identifyConfigFileIssues(settings []ConfigSetting, backwardsCompatible bool) []string {
	var issues []string

	if !backwardsCompatible {
		issues = append(issues, "Breaking changes in configuration")
	}

	criticalChanges := 0
	for _, setting := range settings {
		if setting.Impact == "high" {
			criticalChanges++
		}
	}

	if criticalChanges > 0 {
		issues = append(issues, fmt.Sprintf("%d critical configuration changes", criticalChanges))
	}

	return issues
}

func (sch *SpecialCasesHandler) identifyBinaryFileIssues(binaryType string) []string {
	var issues []string

	if binaryType == "executable" {
		issues = append(issues, "Executable files pose security risks")
	}

	issues = append(issues, "Binary files cannot be reviewed for content")

	return issues
}

func (sch *SpecialCasesHandler) identifyTestFileIssues(testInfo TestInfo) []string {
	var issues []string

	if testInfo.TestCount == 0 {
		issues = append(issues, "No test cases found in test file")
	}

	return issues
}

// Utility methods

func (sch *SpecialCasesHandler) removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

func (sch *SpecialCasesHandler) extractPackageFromPath(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "." {
		return ""
	}

	parts := strings.Split(dir, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}
