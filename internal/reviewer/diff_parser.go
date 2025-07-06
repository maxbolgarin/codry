package reviewer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
)

// diffLineType represents the type of diff line
type diffLineType string

const (
	diffHeaderLine  diffLineType = "header"
	diffContextLine diffLineType = "context"
	diffAddedLine   diffLineType = "added"
	diffRemovedLine diffLineType = "removed"
)

// diffLine represents a single line in a diff with its context
type diffLine struct {
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

// diffParser parses unified diff format
type diffParser struct {
	hunkHeaderRegex *regexp.Regexp
}

// newDiffParser creates a new diff parser
func newDiffParser() *diffParser {
	return &diffParser{
		hunkHeaderRegex: regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`),
	}
}

// parseDiffToLines parses a unified diff and returns line information
func (dp *diffParser) parseDiffToLines(diff string) ([]*diffLine, error) {
	lines := strings.Split(diff, "\n")
	result := make([]*diffLine, 0, len(lines))

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

			result = append(result, &diffLine{
				Type:     diffHeaderLine,
				Content:  line,
				Position: position,
			})
			continue
		}

		switch line[0] {
		case '+':
			result = append(result, &diffLine{
				Type:     diffAddedLine,
				Content:  line[1:],
				NewLine:  newLine,
				Position: position,
			})
			newLine++

		case '-':
			result = append(result, &diffLine{
				Type:     diffRemovedLine,
				Content:  line[1:],
				OldLine:  oldLine,
				Position: position,
			})
			oldLine++

		case ' ':
			result = append(result, &diffLine{
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
			result = append(result, &diffLine{
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

// enhanceReviewComments enhances review comments with line positions and context
func (dp *diffParser) enhanceReviewComments(diff string, comments []*model.ReviewAIComment) error {
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
func (dp *diffParser) getContextForLine(diff string, targetLine int, contextSize int) ([]*diffLine, error) {
	allLines, err := dp.parseDiffToLines(diff)
	if err != nil {
		return nil, err
	}

	var targetIndex = -1
	for i, line := range allLines {
		if line.NewLine == targetLine && line.Type == diffAddedLine {
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
func (dp *diffParser) extractCodeSnippet(diff string, targetLine int) (string, error) {
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
		if line.Type == diffAddedLine {
			prefix = "+"
		} else if line.Type == diffRemovedLine {
			prefix = "-"
		}
		snippet.WriteString(prefix + line.Content + "\n")
	}

	return snippet.String(), nil
}

// createLineMapping creates a mapping from line numbers to positions
func (dp *diffParser) createLineMapping(diff string) (map[int]int, error) {
	lines, err := dp.parseDiffToLines(diff)
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
func (dp *diffParser) extractRangeSnippet(diff string, startLine, endLine int) (string, error) {
	allLines, err := dp.parseDiffToLines(diff)
	if err != nil {
		return "", err
	}

	var snippet strings.Builder
	for _, line := range allLines {
		// Include lines within the range
		if line.NewLine >= startLine && line.NewLine <= endLine {
			prefix := " "
			if line.Type == diffAddedLine {
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
func (dp *diffParser) GenerateCleanDiff(diff string) (string, error) {
	lines, err := dp.parseDiffToLines(diff)
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
		case diffAddedLine:
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
func (dp *diffParser) applyDiffToContent(originalContent string, diffLines []*diffLine) (string, error) {
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
		case diffAddedLine:
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
func (dp *diffParser) extractContentFromAddedLines(diffLines []*diffLine) (string, error) {
	var result []string
	for _, line := range diffLines {
		if line.Type == diffAddedLine {
			result = append(result, line.Content)
		}
	}
	return strings.Join(result, "\n"), nil
}

// AnalyzeSemanticChanges analyzes the semantic meaning of code changes
func (dp *diffParser) AnalyzeSemanticChanges(diff, fileContent string) ([]SemanticChange, error) {
	lines, err := dp.parseDiffToLines(diff)
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
func (dp *diffParser) groupRelatedChanges(lines []*diffLine) [][]*diffLine {
	var groups [][]*diffLine
	var currentGroup []*diffLine

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
func (dp *diffParser) analyzeChangeGroup(group []*diffLine, fileContent string) *SemanticChange {
	if len(group) == 0 {
		return nil
	}

	// Combine all change content
	var addedContent, removedContent strings.Builder
	var lineNumbers []int

	for _, line := range group {
		switch line.Type {
		case diffAddedLine:
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
func (dp *diffParser) classifySemanticChange(added, removed, fileContent string) (SemanticChangeType, ChangeImpact) {
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
func (dp *diffParser) generateChangeDescription(changeType SemanticChangeType, added, removed string) string {
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
func (dp *diffParser) extractSurroundingContext(group []*diffLine, fileContent string) string {
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
func (dp *diffParser) containsPatterns(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}
