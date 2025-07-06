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

		// Add code snippet if not present
		if comment.CodeSnippet == "" {
			if comment.IsRangeComment() {
				snippet, err := dp.extractRangeSnippet(diff, comment.Line, comment.EndLine)
				if err == nil {
					comment.CodeSnippet = snippet
				}
			} else {
				snippet, err := dp.extractCodeSnippet(diff, comment.Line)
				if err == nil {
					comment.CodeSnippet = snippet
				}
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
