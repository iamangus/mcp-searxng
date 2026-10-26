package urlreader

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type PaginationOptions struct {
	StartChar      int
	MaxLength      int
	Section        string
	ParagraphRange string
	ReadHeadings   bool
	HasStartChar   bool
	HasMaxLength   bool
}

func ApplyCharacterPagination(content string, startChar int, maxLength int) string {
	runes := []rune(content)
	if startChar >= len(runes) {
		return ""
	}

	start := startChar
	if start < 0 {
		start = 0
	}

	if maxLength > 0 {
		length := min(maxLength, len(runes)-start)
		return string(runes[start : start+length])
	}

	return string(runes[start:])
}

func ExtractSection(markdownContent string, sectionHeading string) string {
	lines := strings.Split(markdownContent, "\n")
	sectionRegex, err := regexp.Compile(`(?i)^#{1,6}\s*.*` + regexp.QuoteMeta(sectionHeading) + `.*$`)
	if err != nil {
		return ""
	}

	startIndex := -1
	currentLevel := 0

	for i, line := range lines {
		if sectionRegex.MatchString(line) {
			startIndex = i
			headingPrefix := headingPrefixRegex
			match := headingPrefix.FindString(line)
			currentLevel = len(match)
			break
		}
	}

	if startIndex == -1 {
		return ""
	}

	endIndex := len(lines)
	for i := startIndex + 1; i < len(lines); i++ {
		line := lines[i]
		headingMatch := headingPrefixRegex.FindString(line)
		if len(headingMatch) > 0 && len(headingMatch) <= currentLevel {
			endIndex = i
			break
		}
	}

	return strings.Join(lines[startIndex:endIndex], "\n")
}

func ExtractParagraphRange(markdownContent string, rangeStr string) string {
	paragraphs := filterParagraphs(markdownContent)

	matches := rangeRegex.FindStringSubmatch(rangeStr)
	if matches == nil {
		return ""
	}

	first, err := strconv.Atoi(matches[1])
	if err != nil || first < 1 {
		return ""
	}
	start := first - 1
	endStr := matches[2]

	if start < 0 || start >= len(paragraphs) {
		return ""
	}

	if endStr == "" && !strings.Contains(rangeStr, "-") {
		return paragraphs[start]
	}

	if endStr == "" {
		return strings.Join(paragraphs[start:], "\n\n")
	}

	end, err := strconv.Atoi(endStr)
	if err != nil || end < first {
		return ""
	}
	if end > len(paragraphs) {
		end = len(paragraphs)
	}
	return strings.Join(paragraphs[start:end], "\n\n")
}

func ExtractHeadings(markdownContent string) string {
	lines := strings.Split(markdownContent, "\n")

	var headings []string
	for _, line := range lines {
		if headingRegex.MatchString(line) {
			headings = append(headings, line)
		}
	}

	if len(headings) == 0 {
		return "No headings found in the content."
	}

	return strings.Join(headings, "\n")
}

func ApplyPaginationOptions(markdownContent string, opts PaginationOptions) string {
	result := markdownContent

	if opts.ReadHeadings {
		return ExtractHeadings(result)
	}

	if opts.Section != "" {
		result = ExtractSection(result, opts.Section)
		if result == "" {
			return fmt.Sprintf("Section %q not found in the content.", opts.Section)
		}
	}

	if opts.ParagraphRange != "" {
		result = ExtractParagraphRange(result, opts.ParagraphRange)
		if result == "" {
			return fmt.Sprintf("Paragraph range %q is invalid or out of bounds.", opts.ParagraphRange)
		}
	}

	if opts.HasStartChar || opts.HasMaxLength {
		maxLen := 0
		if opts.HasMaxLength {
			maxLen = opts.MaxLength
		}
		result = ApplyCharacterPagination(result, opts.StartChar, maxLen)
	}

	return result
}

func filterParagraphs(content string) []string {
	raw := strings.Split(content, "\n\n")
	var result []string
	for _, p := range raw {
		if strings.TrimSpace(p) != "" {
			result = append(result, p)
		}
	}
	return result
}

var headingPrefixRegex = regexp.MustCompile(`^#+`)
var headingRegex = regexp.MustCompile(`^#{1,6}\s`)
var rangeRegex = regexp.MustCompile(`^(\d+)(?:-(\d*))?$`)
