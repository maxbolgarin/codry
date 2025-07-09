package astparser

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	sitter "github.com/smacker/go-tree-sitter"
)

// diffLineType represents the type of diff line
type diffLineType string

const (
	diffHeaderLine  diffLineType = "header"
	diffContextLine diffLineType = "context"
	DiffAddedLine   diffLineType = "added"
	diffRemovedLine diffLineType = "removed"
)

// DiffLine represents a single line in a diff with its context
type DiffLine struct {
	Type     diffLineType
	Content  string
	OldLine  int
	NewLine  int
	Position int
}

// SemanticChange represents a high-level change with business impact
type SemanticChange struct {
	Type        SemanticChangeType
	Impact      ChangeImpact
	Description string
	Lines       []int
	Context     string
}

// SemanticChangeType categorizes the type of semantic change
type SemanticChangeType string

const (
	SemanticChangeAPIContract         SemanticChangeType = "api_contract"
	SemanticChangeBusinessLogic       SemanticChangeType = "business_logic"
	SemanticChangeErrorHandling       SemanticChangeType = "error_handling"
	SemanticChangeSecurityLogic       SemanticChangeType = "security_logic"
	SemanticChangePerformanceCritical SemanticChangeType = "performance_critical"
	SemanticChangeDataFlow            SemanticChangeType = "data_flow"
	SemanticChangeConfiguration       SemanticChangeType = "configuration"
	SemanticChangeValidation          SemanticChangeType = "validation"
)

// ChangeImpact represents the potential impact of a change
type ChangeImpact string

const (
	ChangeImpactHigh   ChangeImpact = "high"
	ChangeImpactMedium ChangeImpact = "medium"
	ChangeImpactLow    ChangeImpact = "low"
)

// DiffParser parses unified diff format
type DiffParser struct {
	hunkHeaderRegex *regexp.Regexp
}

// NewDiffParser creates a new diff parser
func NewDiffParser() *DiffParser {
	return &DiffParser{
		hunkHeaderRegex: regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`),
	}
}

// ParseDiffToLines parses a unified diff and returns line information
func (dp *DiffParser) ParseDiffToLines(diff string) ([]*DiffLine, error) {
	lines := strings.Split(diff, "\n")
	result := make([]*DiffLine, 0, len(lines))

	var oldLine, newLine, position int

	for i, line := range lines {
		position = i + 1

		// Parse diff content
		if len(line) == 0 {
			continue
		}

		// Skip file headers
		if strings.HasPrefix(line, "diff --git") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") {
			continue
		}

		// Parse hunk header
		if strings.HasPrefix(line, "@@") {
			matches := dp.hunkHeaderRegex.FindStringSubmatch(line)
			if len(matches) >= 4 {
				oldLine, _ = strconv.Atoi(matches[1])
				newLine, _ = strconv.Atoi(matches[3])
			}

			result = append(result, &DiffLine{
				Type:     diffHeaderLine,
				Content:  line,
				Position: position,
			})
			continue
		}

		switch line[0] {
		case '+':
			result = append(result, &DiffLine{
				Type:     DiffAddedLine,
				Content:  line[1:],
				NewLine:  newLine,
				Position: position,
			})
			newLine++

		case '-':
			result = append(result, &DiffLine{
				Type:     diffRemovedLine,
				Content:  line[1:],
				OldLine:  oldLine,
				Position: position,
			})
			oldLine++

		case ' ':
			result = append(result, &DiffLine{
				Type:     diffContextLine,
				Content:  line[1:],
				OldLine:  oldLine,
				NewLine:  newLine,
				Position: position,
			})
			oldLine++
			newLine++

		default:
			// Context line without space prefix
			result = append(result, &DiffLine{
				Type:     diffContextLine,
				Content:  line,
				OldLine:  oldLine,
				NewLine:  newLine,
				Position: position,
			})
			oldLine++
			newLine++

		}
	}

	return result, nil
}

// EnhanceReviewComments enhances review comments with line positions and context
func (dp *DiffParser) EnhanceReviewComments(diff string, comments []*model.ReviewAIComment) error {
	lineMapping, err := dp.createLineMapping(diff)
	if err != nil {
		return err
	}

	for _, comment := range comments {
		// Set position for start line
		if position, exists := lineMapping[comment.Line]; exists {
			comment.Position = position
		}

		// For range comments, validate that end line is reasonable
		if comment.IsRangeComment() {
			// Ensure end line doesn't exceed available lines
			maxLine := 0
			for line := range lineMapping {
				if line > maxLine {
					maxLine = line
				}
			}
			if comment.EndLine > maxLine {
				comment.EndLine = maxLine
			}
		}
	}

	return nil
}

// GetContextForLine returns context lines around a specific line
func (dp *DiffParser) getContextForLine(diff string, targetLine int, contextSize int) ([]*DiffLine, error) {
	allLines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	var targetIndex = -1
	for i, line := range allLines {
		if line.NewLine == targetLine && line.Type == DiffAddedLine {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return nil, nil
	}

	start := targetIndex - contextSize
	if start < 0 {
		start = 0
	}

	end := targetIndex + contextSize + 1
	if end > len(allLines) {
		end = len(allLines)
	}

	return allLines[start:end], nil
}

// ExtractCodeSnippet extracts a code snippet around a specific line
func (dp *DiffParser) extractCodeSnippet(diff string, targetLine int) (string, error) {
	contextLines, err := dp.getContextForLine(diff, targetLine, 3)
	if err != nil {
		return "", err
	}

	if len(contextLines) == 0 {
		return "", nil
	}

	var snippet strings.Builder
	for _, line := range contextLines {
		prefix := " "
		if line.Type == DiffAddedLine {
			prefix = "+"
		} else if line.Type == diffRemovedLine {
			prefix = "-"
		}
		snippet.WriteString(prefix + line.Content + "\n")
	}

	return snippet.String(), nil
}

// createLineMapping creates a mapping from line numbers to positions
func (dp *DiffParser) createLineMapping(diff string) (map[int]int, error) {
	lines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	mapping := make(map[int]int)
	for _, line := range lines {
		if line.NewLine > 0 {
			mapping[line.NewLine] = line.Position
		}
	}

	return mapping, nil
}

// ExtractRangeSnippet extracts a code snippet for a range of lines
func (dp *DiffParser) extractRangeSnippet(diff string, startLine, endLine int) (string, error) {
	allLines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return "", err
	}

	var snippet strings.Builder
	for _, line := range allLines {
		// Include lines within the range
		if line.NewLine >= startLine && line.NewLine <= endLine {
			prefix := " "
			if line.Type == DiffAddedLine {
				prefix = "+"
			} else if line.Type == diffRemovedLine {
				prefix = "-"
			}
			snippet.WriteString(prefix + line.Content + "\n")
		}
	}

	return snippet.String(), nil
}

// GenerateCleanDiff creates a clean diff format with explicit line numbers and logical grouping
func (dp *DiffParser) GenerateCleanDiff(diff string) (string, error) {
	lines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return "", err
	}

	var cleanDiff strings.Builder
	var lastLineNumber int
	var hasContent bool
	const lineGapThreshold = 3 // Add break if gap between lines is > 3

	for _, line := range lines {
		var currentLineNumber int
		var lineText string

		switch line.Type {
		case DiffAddedLine:
			currentLineNumber = line.NewLine
			lineText = fmt.Sprintf("+ %d: %s", line.NewLine, line.Content)
		case diffRemovedLine:
			currentLineNumber = line.OldLine
			lineText = fmt.Sprintf("- %d: %s", line.OldLine, line.Content)
		default:
			continue // Skip context lines and headers
		}

		// Add line break between logical groups (when there's a significant line gap)
		if hasContent && currentLineNumber > lastLineNumber+lineGapThreshold {
			cleanDiff.WriteString("\n")
		}

		// Add the current line
		cleanDiff.WriteString(lineText)
		cleanDiff.WriteString("\n")

		lastLineNumber = currentLineNumber
		hasContent = true
	}

	// Remove the trailing newline if present
	result := cleanDiff.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// applyDiffToContent applies a diff to original content to get the final content
func (dp *DiffParser) applyDiffToContent(originalContent string, diffLines []*DiffLine) (string, error) {
	if originalContent == "" {
		// For new files, extract content from added lines
		return dp.extractContentFromAddedLines(diffLines)
	}

	originalLines := strings.Split(originalContent, "\n")
	var result []string

	// Build a map of line changes
	addedLines := make(map[int]string, len(diffLines)) // newLine -> content
	removedLines := make(map[int]bool, len(diffLines)) // oldLine -> true

	for _, line := range diffLines {
		switch line.Type {
		case DiffAddedLine:
			if line.NewLine > 0 {
				addedLines[line.NewLine] = line.Content
			}
		case diffRemovedLine:
			if line.OldLine > 0 {
				removedLines[line.OldLine] = true
			}
		}
	}

	// Apply changes line by line
	newLineNum := 1
	for oldLineNum := 1; oldLineNum <= len(originalLines); oldLineNum++ {
		// Skip removed lines
		if removedLines[oldLineNum] {
			continue
		}

		// Add any inserted lines before this line
		for {
			if addedContent, exists := addedLines[newLineNum]; exists {
				result = append(result, addedContent)
				newLineNum++
			} else {
				break
			}
		}

		// Add original line (if not removed)
		if !removedLines[oldLineNum] {
			result = append(result, originalLines[oldLineNum-1])
			newLineNum++
		}
	}

	// Add any remaining added lines at the end
	for {
		if addedContent, exists := addedLines[newLineNum]; exists {
			result = append(result, addedContent)
			newLineNum++
		} else {
			break
		}
	}

	return strings.Join(result, "\n"), nil
}

// extractContentFromAddedLines extracts content from added lines for new files
func (dp *DiffParser) extractContentFromAddedLines(diffLines []*DiffLine) (string, error) {
	var result []string
	for _, line := range diffLines {
		if line.Type == DiffAddedLine {
			result = append(result, line.Content)
		}
	}
	return strings.Join(result, "\n"), nil
}

// AnalyzeSemanticChanges analyzes the semantic meaning of code changes
func (dp *DiffParser) AnalyzeSemanticChanges(diff, fileContent string) ([]SemanticChange, error) {
	lines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	var changes []SemanticChange

	// Group related changes together
	changeGroups := dp.groupRelatedChanges(lines)

	for _, group := range changeGroups {
		semanticChange := dp.analyzeChangeGroup(group, fileContent)
		if semanticChange != nil {
			changes = append(changes, *semanticChange)
		}
	}

	return changes, nil
}

// groupRelatedChanges groups diff lines that are logically related
func (dp *DiffParser) groupRelatedChanges(lines []*DiffLine) [][]*DiffLine {
	var groups [][]*DiffLine
	var currentGroup []*DiffLine

	for i, line := range lines {
		if line.Type == diffHeaderLine {
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
				currentGroup = nil
			}
			continue
		}

		// Start new group if there's a significant gap
		if len(currentGroup) > 0 {
			lastLine := currentGroup[len(currentGroup)-1]
			if line.NewLine > 0 && lastLine.NewLine > 0 && line.NewLine-lastLine.NewLine > 5 {
				groups = append(groups, currentGroup)
				currentGroup = nil
			}
		}

		currentGroup = append(currentGroup, line)

		// End group at the last line
		if i == len(lines)-1 && len(currentGroup) > 0 {
			groups = append(groups, currentGroup)
		}
	}

	return groups
}

// analyzeChangeGroup analyzes a group of related changes to determine semantic meaning
func (dp *DiffParser) analyzeChangeGroup(group []*DiffLine, fileContent string) *SemanticChange {
	if len(group) == 0 {
		return nil
	}

	// Combine all change content
	var addedContent, removedContent strings.Builder
	var lineNumbers []int

	for _, line := range group {
		switch line.Type {
		case DiffAddedLine:
			addedContent.WriteString(line.Content + "\n")
			if line.NewLine > 0 {
				lineNumbers = append(lineNumbers, line.NewLine)
			}
		case diffRemovedLine:
			removedContent.WriteString(line.Content + "\n")
			if line.OldLine > 0 {
				lineNumbers = append(lineNumbers, line.OldLine)
			}
		}
	}

	added := addedContent.String()
	removed := removedContent.String()

	// Analyze the semantic meaning
	changeType, impact := dp.classifySemanticChange(added, removed, fileContent)
	if changeType == "" {
		return nil
	}

	return &SemanticChange{
		Type:        changeType,
		Impact:      impact,
		Description: dp.generateChangeDescription(changeType, added, removed),
		Lines:       lineNumbers,
		Context:     dp.extractSurroundingContext(group, fileContent),
	}
}

// classifySemanticChange determines the type and impact of a change
func (dp *DiffParser) classifySemanticChange(added, removed, fileContent string) (SemanticChangeType, ChangeImpact) {
	combinedChange := added + " " + removed
	lowerChange := strings.ToLower(combinedChange)

	// API Contract changes
	if dp.containsPatterns(lowerChange, []string{"func ", "method", "interface", "struct", "type"}) &&
		dp.containsPatterns(added, []string{"func "}) {
		return SemanticChangeAPIContract, ChangeImpactHigh
	}

	// Security-related changes
	if dp.containsPatterns(lowerChange, []string{"auth", "password", "token", "session", "validate", "sanitize", "permission"}) {
		return SemanticChangeSecurityLogic, ChangeImpactHigh
	}

	// Error handling changes
	if dp.containsPatterns(lowerChange, []string{"error", "err", "panic", "recover", "try", "catch", "throw"}) {
		if dp.containsPatterns(added, []string{"error", "err"}) {
			return SemanticChangeErrorHandling, ChangeImpactMedium
		}
		return SemanticChangeErrorHandling, ChangeImpactHigh
	}

	// Performance-critical changes
	if dp.containsPatterns(lowerChange, []string{"query", "database", "db", "cache", "loop", "for ", "while", "goroutine", "async"}) {
		return SemanticChangePerformanceCritical, ChangeImpactMedium
	}

	// Configuration changes
	if dp.containsPatterns(lowerChange, []string{"config", "setting", "env", "flag", "const", "var"}) {
		return SemanticChangeConfiguration, ChangeImpactMedium
	}

	// Validation changes
	if dp.containsPatterns(lowerChange, []string{"validate", "check", "verify", "assert", "ensure"}) {
		return SemanticChangeValidation, ChangeImpactMedium
	}

	// Business logic changes (fallback for substantial changes)
	if len(added) > 100 || len(removed) > 100 {
		return SemanticChangeBusinessLogic, ChangeImpactMedium
	}

	return "", ChangeImpactLow
}

// generateChangeDescription creates a human-readable description of the change
func (dp *DiffParser) generateChangeDescription(changeType SemanticChangeType, added, removed string) string {
	switch changeType {
	case SemanticChangeAPIContract:
		if strings.Contains(added, "func ") {
			return "API contract modified - new function or method signature"
		}
		return "API contract modified - interface or type definition changed"
	case SemanticChangeSecurityLogic:
		return "Security-related logic modified - authentication, authorization, or validation changes"
	case SemanticChangeErrorHandling:
		if len(added) > len(removed) {
			return "Error handling enhanced - additional error checking or recovery"
		}
		return "Error handling modified - changes to error processing logic"
	case SemanticChangePerformanceCritical:
		return "Performance-critical code modified - database queries, loops, or async operations"
	case SemanticChangeConfiguration:
		return "Configuration changes - settings, constants, or environment variables"
	case SemanticChangeValidation:
		return "Validation logic modified - input checking or data verification"
	case SemanticChangeBusinessLogic:
		return "Business logic modified - core application functionality changed"
	case SemanticChangeDataFlow:
		return "Data flow modified - how data moves through the system"
	default:
		return "Code logic modified"
	}
}

// extractSurroundingContext extracts context around the changes
func (dp *DiffParser) extractSurroundingContext(group []*DiffLine, fileContent string) string {
	if len(group) == 0 {
		return ""
	}

	// Find the function or struct this change belongs to
	firstLine := group[0]
	lines := strings.Split(fileContent, "\n")

	// Look backwards for function/method/struct definition
	startLine := firstLine.NewLine
	if startLine <= 0 {
		startLine = firstLine.OldLine
	}
	if startLine <= 0 || startLine > len(lines) {
		return ""
	}

	for i := startLine - 1; i >= 0 && i < len(lines); i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "func ") ||
			strings.HasPrefix(line, "type ") ||
			strings.Contains(line, "struct {") ||
			strings.Contains(line, "interface {") {
			return line
		}
		// Don't go too far back
		if startLine-i > 20 {
			break
		}
	}

	return ""
}

// Helper function to check if text contains any of the patterns
func (dp *DiffParser) containsPatterns(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

// ParsedFileDiff represents a file diff with enhanced context information
type ParsedFileDiff struct {
	FilePath        string           `json:"file_path"`
	ChangeType      ChangeType       `json:"change_type"`
	DiffLines       []*DiffLine      `json:"diff_lines"`
	AddedLines      []int            `json:"added_lines"`
	RemovedLines    []int            `json:"removed_lines"`
	ModifiedLines   []int            `json:"modified_lines"`
	AffectedSymbols []AffectedSymbol `json:"affected_symbols"`
	LineMapping     map[int]int      `json:"line_mapping"` // old line -> new line
}

// ParsedDiffContext represents the complete parsed diff with context
type ParsedDiffContext struct {
	Files []ParsedFileDiff `json:"files"`
}

// parseDiffWithContext parses a diff and enriches it with AST context
func (dp *DiffParser) parseDiffWithContext(diff string, filePath string, astParser *Parser) (*ParsedFileDiff, error) {
	// Parse basic diff structure
	diffLines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	// Extract line number information
	parsedDiff := &ParsedFileDiff{
		FilePath:        filePath,
		DiffLines:       diffLines,
		AddedLines:      make([]int, 0),
		RemovedLines:    make([]int, 0),
		ModifiedLines:   make([]int, 0),
		AffectedSymbols: make([]AffectedSymbol, 0),
		LineMapping:     make(map[int]int),
	}

	// Categorize lines and build mappings
	for _, line := range diffLines {
		switch line.Type {
		case DiffAddedLine:
			if line.NewLine > 0 {
				parsedDiff.AddedLines = append(parsedDiff.AddedLines, line.NewLine)
			}
		case diffRemovedLine:
			if line.OldLine > 0 {
				parsedDiff.RemovedLines = append(parsedDiff.RemovedLines, line.OldLine)
			}
		case diffContextLine:
			// Build line mapping for context lines
			if line.OldLine > 0 && line.NewLine > 0 {
				parsedDiff.LineMapping[line.OldLine] = line.NewLine
			}
		}
	}

	// Determine change type based on line patterns
	parsedDiff.ChangeType = dp.inferChangeType(parsedDiff.AddedLines, parsedDiff.RemovedLines, filePath)

	return parsedDiff, nil
}

// inferChangeType infers the change type from line patterns
func (dp *DiffParser) inferChangeType(addedLines, removedLines []int, filePath string) ChangeType {
	if len(addedLines) > 0 && len(removedLines) == 0 {
		// Only additions - could be a new file or additions to existing file
		return ChangeTypeAdded
	} else if len(addedLines) == 0 && len(removedLines) > 0 {
		// Only deletions - could be a deleted file or deletions from existing file
		return ChangeTypeDeleted
	} else if len(addedLines) > 0 && len(removedLines) > 0 {
		// Both additions and deletions - modified file
		return ChangeTypeModified
	}

	return ChangeTypeModified
}

// mapLinesToSymbols maps changed lines to their containing symbols using AST
func (dp *DiffParser) mapLinesToSymbols(filePath, content string, changedLines []int, astParser *Parser) ([]AffectedSymbol, error) {
	if astParser == nil {
		return nil, fmt.Errorf("AST parser not available")
	}

	return astParser.FindAffectedSymbols(context.Background(), filePath, content, changedLines)
}

// Enhanced diff analysis methods

// AnalyzeDiffImpact analyzes the impact of changes using both diff and AST information
func (dp *DiffParser) AnalyzeDiffImpact(diff, filePath, content string, astParser *Parser) (*DiffImpactAnalysis, error) {
	parsedDiff, err := dp.parseDiffWithContext(diff, filePath, astParser)
	if err != nil {
		return nil, err
	}

	impact := &DiffImpactAnalysis{
		FilePath:          filePath,
		TotalLinesChanged: len(parsedDiff.AddedLines) + len(parsedDiff.RemovedLines),
		AddedLinesCount:   len(parsedDiff.AddedLines),
		RemovedLinesCount: len(parsedDiff.RemovedLines),
		ChangeComplexity:  dp.calculateChangeComplexity(parsedDiff),
		ImpactScore:       0.0,
		AffectedSymbols:   make([]AffectedSymbol, 0),
		PotentialIssues:   make([]string, 0),
	}

	// If AST parser is available and we have content, analyze symbols
	if astParser != nil && content != "" {
		// Get all changed lines (added + removed mapped to new line numbers)
		var allChangedLines []int
		allChangedLines = append(allChangedLines, parsedDiff.AddedLines...)

		// Map removed lines to their approximate new positions
		for _, oldLine := range parsedDiff.RemovedLines {
			if newLine, exists := parsedDiff.LineMapping[oldLine]; exists {
				allChangedLines = append(allChangedLines, newLine)
			}
		}

		// Find affected symbols
		affectedSymbols, err := dp.mapLinesToSymbols(filePath, content, allChangedLines, astParser)
		if err == nil {
			impact.AffectedSymbols = affectedSymbols
			impact.SymbolsAffectedCount = len(affectedSymbols)
		}

		// Calculate impact score based on symbols and change patterns
		impact.ImpactScore = dp.calculateImpactScore(impact, affectedSymbols)

		// Identify potential issues
		impact.PotentialIssues = dp.identifyPotentialIssues(parsedDiff, affectedSymbols, content)
	}

	return impact, nil
}

// DiffImpactAnalysis represents comprehensive analysis of diff impact
type DiffImpactAnalysis struct {
	FilePath             string           `json:"file_path"`
	TotalLinesChanged    int              `json:"total_lines_changed"`
	AddedLinesCount      int              `json:"added_lines_count"`
	RemovedLinesCount    int              `json:"removed_lines_count"`
	SymbolsAffectedCount int              `json:"symbols_affected_count"`
	ChangeComplexity     ComplexityLevel  `json:"change_complexity"`
	ImpactScore          float64          `json:"impact_score"`
	AffectedSymbols      []AffectedSymbol `json:"affected_symbols"`
	PotentialIssues      []string         `json:"potential_issues"`
}

// ComplexityLevel represents the complexity level of changes
type ComplexityLevel string

const (
	ComplexityLow      ComplexityLevel = "low"
	ComplexityMedium   ComplexityLevel = "medium"
	ComplexityHigh     ComplexityLevel = "high"
	ComplexityCritical ComplexityLevel = "critical"
)

// calculateChangeComplexity calculates the complexity of changes
func (dp *DiffParser) calculateChangeComplexity(parsedDiff *ParsedFileDiff) ComplexityLevel {
	totalChanges := len(parsedDiff.AddedLines) + len(parsedDiff.RemovedLines)

	// Simple heuristic based on number of changed lines
	if totalChanges <= 5 {
		return ComplexityLow
	} else if totalChanges <= 20 {
		return ComplexityMedium
	} else if totalChanges <= 50 {
		return ComplexityHigh
	} else {
		return ComplexityCritical
	}
}

// calculateImpactScore calculates a numerical impact score
func (dp *DiffParser) calculateImpactScore(impact *DiffImpactAnalysis, symbols []AffectedSymbol) float64 {
	score := 0.0

	// Base score from line changes
	score += float64(impact.TotalLinesChanged) * 0.1

	// Symbol impact multiplier
	for _, symbol := range symbols {
		symbolScore := 1.0

		// Higher impact for different symbol types
		switch symbol.Type {
		case SymbolTypeFunction, SymbolTypeMethod:
			symbolScore *= 2.0
		case SymbolTypeClass, SymbolTypeInterface:
			symbolScore *= 3.0
		case SymbolTypeStruct, SymbolTypeType:
			symbolScore *= 2.5
		}

		// Consider dependencies
		symbolScore += float64(len(symbol.Dependencies)) * 0.5

		score += symbolScore
	}

	// Complexity multiplier
	switch impact.ChangeComplexity {
	case ComplexityMedium:
		score *= 1.5
	case ComplexityHigh:
		score *= 2.0
	case ComplexityCritical:
		score *= 3.0
	}

	return score
}

// identifyPotentialIssues identifies potential issues based on change patterns
func (dp *DiffParser) identifyPotentialIssues(parsedDiff *ParsedFileDiff, symbols []AffectedSymbol, content string) []string {
	var issues []string

	// Check for large changes
	if len(parsedDiff.AddedLines)+len(parsedDiff.RemovedLines) > 100 {
		issues = append(issues, "Large change set - consider breaking into smaller changes")
	}

	// Check for changes to critical symbols
	for _, symbol := range symbols {
		if symbol.Type == SymbolTypeInterface {
			issues = append(issues, fmt.Sprintf("Interface %s modified - may break compatibility", symbol.Name))
		}

		if len(symbol.Dependencies) > 10 {
			issues = append(issues, fmt.Sprintf("Function %s has many dependencies - high coupling", symbol.Name))
		}
	}

	// Check for deletions without corresponding additions
	if len(parsedDiff.RemovedLines) > len(parsedDiff.AddedLines)*2 {
		issues = append(issues, "More lines removed than added - potential functionality loss")
	}

	// Pattern-based issue detection
	contentLower := strings.ToLower(content)
	for _, pattern := range riskPatterns {
		if strings.Contains(contentLower, pattern.pattern) {
			issues = append(issues, pattern.warning)
		}
	}

	return issues
}

// riskPattern represents a pattern that might indicate potential issues
type riskPattern struct {
	pattern string
	warning string
}

// riskPatterns defines patterns that might indicate potential issues
var riskPatterns = []riskPattern{
	{"todo", "TODO comments found - incomplete implementation"},
	{"fixme", "FIXME comments found - known issues exist"},
	{"hack", "HACK comments found - non-standard implementation"},
	{"panic", "Panic statements found - potential runtime crashes"},
	{"unsafe", "Unsafe operations found - potential memory issues"},
	{"deprecated", "Deprecated code found - consider modernization"},
}

// GetImpactfulChanges returns the most impactful changes from a set of diffs
func (dp *DiffParser) GetImpactfulChanges(diffAnalyses []*DiffImpactAnalysis, maxCount int) []*DiffImpactAnalysis {
	if len(diffAnalyses) <= maxCount {
		return diffAnalyses
	}

	// Sort by impact score (descending)
	sortedAnalyses := make([]*DiffImpactAnalysis, len(diffAnalyses))
	copy(sortedAnalyses, diffAnalyses)

	// Simple bubble sort for small arrays
	for i := 0; i < len(sortedAnalyses)-1; i++ {
		for j := 0; j < len(sortedAnalyses)-i-1; j++ {
			if sortedAnalyses[j].ImpactScore < sortedAnalyses[j+1].ImpactScore {
				sortedAnalyses[j], sortedAnalyses[j+1] = sortedAnalyses[j+1], sortedAnalyses[j]
			}
		}
	}

	return sortedAnalyses[:maxCount]
}

// Enhanced semantic change analysis

// AnalyzeSemanticChangesWithAST analyzes semantic changes using both diff and AST
func (dp *DiffParser) AnalyzeSemanticChangesWithAST(diff, fileContent string, astParser *Parser) ([]EnhancedSemanticChange, error) {
	// Get basic semantic changes
	basicChanges, err := dp.AnalyzeSemanticChanges(diff, fileContent)
	if err != nil {
		return nil, err
	}

	// Enhance with AST information
	var enhancedChanges []EnhancedSemanticChange

	for _, change := range basicChanges {
		enhanced := EnhancedSemanticChange{
			SemanticChange:  change,
			AffectedSymbols: make([]AffectedSymbol, 0),
			Dependencies:    make([]string, 0),
			Callers:         make([]string, 0),
		}

		// If we have AST parser and the change has line information
		if astParser != nil && len(change.Lines) > 0 {
			// Find symbols affected by this semantic change
			symbols, err := astParser.FindAffectedSymbols(context.Background(), "", fileContent, change.Lines)
			if err == nil {
				enhanced.AffectedSymbols = symbols

				// Extract dependency information from symbols
				for _, symbol := range symbols {
					for _, dep := range symbol.Dependencies {
						enhanced.Dependencies = append(enhanced.Dependencies, dep.Name)
					}
				}
			}
		}

		enhancedChanges = append(enhancedChanges, enhanced)
	}

	return enhancedChanges, nil
}

// EnhancedSemanticChange extends SemanticChange with AST-derived information
type EnhancedSemanticChange struct {
	SemanticChange  `json:",inline"`
	AffectedSymbols []AffectedSymbol `json:"affected_symbols"`
	Dependencies    []string         `json:"dependencies"`
	Callers         []string         `json:"callers"`
}

// Line-to-Symbol mapping utilities

// CreateLineToSymbolMap creates a mapping from line numbers to symbols in a file
func (dp *DiffParser) CreateLineToSymbolMap(filePath, content string, astParser *Parser) (map[int][]AffectedSymbol, error) {
	if astParser == nil {
		return nil, fmt.Errorf("AST parser required")
	}

	lineToSymbols := make(map[int][]AffectedSymbol)

	// Parse file to get all symbols
	rootNode, err := astParser.ParseFileToAST(context.Background(), filePath, content)
	if err != nil {
		return nil, err
	}

	// Walk the AST to find all symbols and their line ranges
	dp.walkASTToMapLines(rootNode, filePath, content, astParser, lineToSymbols)

	return lineToSymbols, nil
}

// walkASTToMapLines walks the AST and maps line numbers to symbols
func (dp *DiffParser) walkASTToMapLines(node *sitter.Node, filePath, content string, astParser *Parser, lineToSymbols map[int][]AffectedSymbol) {
	if astParser.IsSymbolNode(node.Type()) {
		symbol := astParser.ExtractSymbolFromNode(node, filePath, content)
		if symbol.Name != "" {
			// Map all lines in the symbol's range to this symbol
			for line := symbol.StartLine; line <= symbol.EndLine; line++ {
				if lineToSymbols[line] == nil {
					lineToSymbols[line] = make([]AffectedSymbol, 0)
				}
				lineToSymbols[line] = append(lineToSymbols[line], symbol)
			}
		}
	}

	// Recursively process children
	childCount := int(node.ChildCount())
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child != nil {
			dp.walkASTToMapLines(child, filePath, content, astParser, lineToSymbols)
		}
	}
}

// GetSymbolsForChangedLines returns symbols that contain the changed lines
func (dp *DiffParser) GetSymbolsForChangedLines(diff, filePath, content string, astParser *Parser) ([]AffectedSymbol, error) {
	// Parse diff to get changed line numbers
	lines, err := dp.ParseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	var changedLines []int
	for _, line := range lines {
		if line.Type == DiffAddedLine && line.NewLine > 0 {
			changedLines = append(changedLines, line.NewLine)
		}
	}

	if len(changedLines) == 0 {
		return []AffectedSymbol{}, nil
	}

	// Create line-to-symbol mapping
	lineToSymbols, err := dp.CreateLineToSymbolMap(filePath, content, astParser)
	if err != nil {
		return nil, err
	}

	// Collect unique symbols for changed lines
	symbolMap := make(map[string]AffectedSymbol)
	for _, lineNum := range changedLines {
		if symbols, exists := lineToSymbols[lineNum]; exists {
			for _, symbol := range symbols {
				key := fmt.Sprintf("%s:%d:%d", symbol.Name, symbol.StartLine, symbol.EndLine)
				symbolMap[key] = symbol
			}
		}
	}

	// Convert map to slice
	var result []AffectedSymbol
	for _, symbol := range symbolMap {
		result = append(result, symbol)
	}

	return result, nil
}
