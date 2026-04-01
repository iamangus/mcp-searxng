package urlreader_test

import (
	"testing"

	"github.com/iamangus/mcp-searxng/internal/urlreader"
)

func TestApplyCharacterPagination(t *testing.T) {
	content := "Hello, World! This is a test string."

	t.Run("normal slicing with startChar and maxLength", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 7, 6)
		if result != "World!" {
			t.Errorf("expected %q, got %q", "World!", result)
		}
	})

	t.Run("maxLength exceeds content remainder", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 29, 100)
		if result != "string." {
			t.Errorf("expected %q, got %q", "string.", result)
		}
	})

	t.Run("startChar beyond content length returns empty string", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 1000, 10)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("startChar exactly at content length returns empty string", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, len(content), 10)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("maxLength of 0 returns everything from startChar", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 7, 0)
		expected := content[7:]
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("negative maxLength returns everything from startChar", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 7, -5)
		expected := content[7:]
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("negative startChar clamped to 0", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, -10, 5)
		if result != "Hello" {
			t.Errorf("expected %q, got %q", "Hello", result)
		}
	})

	t.Run("startChar 0 and maxLength 0 returns full content", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination(content, 0, 0)
		if result != content {
			t.Errorf("expected %q, got %q", content, result)
		}
	})

	t.Run("empty content returns empty string", func(t *testing.T) {
		result := urlreader.ApplyCharacterPagination("", 0, 10)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})
}

func TestExtractSection(t *testing.T) {
	markdown := `# Main Title

Some intro text.

## Section A

Content of section A.

### Subsection A1

Sub A1 content.

## Section B

Content of section B.

# Another Title

More text.`

	t.Run("finds section by heading text", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Section A")
		if !contains(result, "## Section A") || !contains(result, "Content of section A.") || !contains(result, "### Subsection A1") || !contains(result, "Sub A1 content.") {
			t.Errorf("section should contain expected content, got:\n%s", result)
		}
		if contains(result, "## Section B") {
			t.Error("section should not contain next same-level section heading")
		}
	})

	t.Run("section ends at next same-level heading", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Main Title")
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		lastLine := ""
		lines := splitLines(result)
		if len(lines) > 0 {
			lastLine = lines[len(lines)-1]
		}
		if lastLine == "# Another Title" {
			t.Errorf("section should not include same-level heading from another section, got:\n%s", result)
		}
	})

	t.Run("case-insensitive heading match", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "section a")
		if result == "" {
			t.Error("expected non-empty result for case-insensitive match")
		}
	})

	t.Run("non-existent section returns empty string", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Non-existent Section")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("nested headings within section are included", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Section A")
		if result == "" {
			t.Fatal("expected non-empty result")
		}
		if !contains(result, "### Subsection A1") {
			t.Error("nested heading should be included in section")
		}
		if !contains(result, "Sub A1 content.") {
			t.Error("nested content should be included in section")
		}
	})

	t.Run("h1 section does not include subsequent h1", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Another Title")
		expected := `# Another Title

More text.`
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})

	t.Run("h2 section does not include subsequent h2", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "Section B")
		if !contains(result, "## Section B") || !contains(result, "Content of section B.") {
			t.Errorf("section should contain expected content, got:\n%s", result)
		}
		if contains(result, "# Another Title") {
			t.Error("section should not contain next higher-level heading")
		}
	})

	t.Run("empty content returns empty string", func(t *testing.T) {
		result := urlreader.ExtractSection("", "anything")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("empty heading returns empty string", func(t *testing.T) {
		result := urlreader.ExtractSection(markdown, "")
		if result == "" {
			t.Error("empty heading should still match headings with regex")
		}
	})
}

func TestExtractParagraphRange(t *testing.T) {
	content := `First paragraph.

Second paragraph.

Third paragraph.

Fourth paragraph.`

	t.Run("single paragraph", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "1")
		if result != "First paragraph." {
			t.Errorf("expected %q, got %q", "First paragraph.", result)
		}
	})

	t.Run("single paragraph middle", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "3")
		if result != "Third paragraph." {
			t.Errorf("expected %q, got %q", "Third paragraph.", result)
		}
	})

	t.Run("range of paragraphs", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "1-3")
		expected := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})

	t.Run("open-ended range", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "2-")
		expected := "Second paragraph.\n\nThird paragraph.\n\nFourth paragraph."
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})

	t.Run("range exceeding paragraph count is clamped", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "3-100")
		expected := "Third paragraph.\n\nFourth paragraph."
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})

	t.Run("invalid range string returns empty string", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "abc")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("invalid range with letters returns empty string", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "1-a")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("out of bounds start returns empty string", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "10")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("zero start returns empty string", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange(content, "0")
		if result != "" {
			t.Errorf("expected empty string for paragraph 0, got %q", result)
		}
	})

	t.Run("empty content returns empty string", func(t *testing.T) {
		result := urlreader.ExtractParagraphRange("", "1")
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single paragraph with trailing content", func(t *testing.T) {
		singleContent := "Only one paragraph here."
		result := urlreader.ExtractParagraphRange(singleContent, "1")
		if result != "Only one paragraph here." {
			t.Errorf("expected %q, got %q", "Only one paragraph here.", result)
		}
	})

	t.Run("content with extra blank lines", func(t *testing.T) {
		multiBlank := "Para one.\n\n\n\nPara two.\n\nPara three."
		result := urlreader.ExtractParagraphRange(multiBlank, "1-2")
		expected := "Para one.\n\nPara two."
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})
}

func TestExtractHeadings(t *testing.T) {
	markdown := `# Title One

Some text.

## Subtitle A

More text.

### Sub-subtitle

Even more.

# Title Two

## Subtitle B

End.`

	t.Run("extracts all markdown headings", func(t *testing.T) {
		result := urlreader.ExtractHeadings(markdown)
		expected := `# Title One
## Subtitle A
### Sub-subtitle
# Title Two
## Subtitle B`
		if result != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, result)
		}
	})

	t.Run("content without headings", func(t *testing.T) {
		result := urlreader.ExtractHeadings("Just some plain text.\nNo headings here.")
		if result != "No headings found in the content." {
			t.Errorf("expected %q, got %q", "No headings found in the content.", result)
		}
	})

	t.Run("handles mixed heading levels", func(t *testing.T) {
		mixed := `###### H6
##### H5
#### H4
### H3
## H2
# H1`
		result := urlreader.ExtractHeadings(mixed)
		if result == "" {
			t.Error("expected non-empty result")
		}
		lines := splitLines(result)
		if len(lines) != 6 {
			t.Errorf("expected 6 headings, got %d", len(lines))
		}
	})

	t.Run("empty content returns no headings message", func(t *testing.T) {
		result := urlreader.ExtractHeadings("")
		if result != "No headings found in the content." {
			t.Errorf("expected %q, got %q", "No headings found in the content.", result)
		}
	})

	t.Run("lines starting with # but no space are not headings", func(t *testing.T) {
		notHeading := "#NotAHeading\n# Real Heading"
		result := urlreader.ExtractHeadings(notHeading)
		if result != "# Real Heading" {
			t.Errorf("expected %q, got %q", "# Real Heading", result)
		}
	})
}

func TestApplyPaginationOptions(t *testing.T) {
	content := `# Introduction

Welcome to the guide.

## Getting Started

First step is to install.

Second step is to configure.

## Advanced Usage

Advanced content here.

Even more advanced details.`

	t.Run("ReadHeadings mode", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			ReadHeadings: true,
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if result == "" {
			t.Error("expected non-empty headings result")
		}
		if !contains(result, "# Introduction") {
			t.Error("headings should include Introduction")
		}
		if !contains(result, "## Getting Started") {
			t.Error("headings should include Getting Started")
		}
	})

	t.Run("Section extraction", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			Section: "Getting Started",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if !contains(result, "## Getting Started") {
			t.Error("section result should include the heading")
		}
		if !contains(result, "First step is to install.") {
			t.Error("section result should include section content")
		}
	})

	t.Run("Section not found returns message", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			Section: "Non-existent",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		expected := `Section "Non-existent" not found in the content.`
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Paragraph range with section", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			Section:        "Advanced Usage",
			ParagraphRange: "2",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if result != "Advanced content here." {
			t.Errorf("expected %q, got %q", "Advanced content here.", result)
		}
	})

	t.Run("Invalid paragraph range returns message", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			ParagraphRange: "abc",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		expected := `Paragraph range "abc" is invalid or out of bounds.`
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Out of bounds paragraph range returns message", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			ParagraphRange: "999",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		expected := `Paragraph range "999" is invalid or out of bounds.`
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Character pagination with HasStartChar", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			HasStartChar: true,
			StartChar:    0,
			HasMaxLength: true,
			MaxLength:    6,
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if result != "# Intr" {
			t.Errorf("expected %q, got %q", "# Intr", result)
		}
	})

	t.Run("Character pagination with HasMaxLength only", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			HasMaxLength: true,
			MaxLength:    10,
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		expected := content[:10]
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Combined section and character pagination", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			Section:      "Getting Started",
			HasStartChar: true,
			StartChar:    0,
			HasMaxLength: true,
			MaxLength:    10,
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		section := urlreader.ExtractSection(content, "Getting Started")
		expected := section[:10]
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("No options returns full content", func(t *testing.T) {
		opts := urlreader.PaginationOptions{}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if result != content {
			t.Errorf("expected full content, got different result")
		}
	})

	t.Run("ReadHeadings takes priority over other options", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			ReadHeadings:   true,
			Section:        "Getting Started",
			ParagraphRange: "1",
			HasStartChar:   true,
			StartChar:      0,
			HasMaxLength:   true,
			MaxLength:      5,
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if contains(result, "First step") {
			t.Error("ReadHeadings should take priority, should not contain paragraph content")
		}
		if !contains(result, "# Introduction") {
			t.Error("ReadHeadings should return headings list")
		}
	})

	t.Run("Section with paragraph range out of bounds", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			Section:        "Getting Started",
			ParagraphRange: "100",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		expected := `Paragraph range "100" is invalid or out of bounds.`
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("Paragraph range open-ended on full content", func(t *testing.T) {
		opts := urlreader.PaginationOptions{
			ParagraphRange: "2-",
		}
		result := urlreader.ApplyPaginationOptions(content, opts)
		if result == "" {
			t.Error("expected non-empty result for open-ended range")
		}
		if contains(result, "# Introduction") {
			t.Error("open-ended range starting at 2 should not include first paragraph")
		}
	})
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
