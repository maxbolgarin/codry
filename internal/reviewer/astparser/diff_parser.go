package astparser

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
