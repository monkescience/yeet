package changelog

import (
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"go.yaml.in/yaml/v4"
)

const (
	titleHeadingLevel       = 1
	releaseHeadingLevel     = 2
	sectionHeadingLevel     = 3
	maxHeadingIndent        = 3
	utf8BOM                 = "\xef\xbb\xbf"
	yamlFrontMatterFence    = "---"
	yamlFrontMatterEndFence = "..."
)

type markdownHeading struct {
	level int
	line  int
	text  string
}

type markdownIndex struct {
	lines            []string
	headings         []markdownHeading
	frontMatterLines int
	titleLine        int
}

func newMarkdownIndex(source string) markdownIndex {
	normalized := strings.ReplaceAll(strings.TrimPrefix(source, utf8BOM), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	frontMatterLines := yamlFrontMatterLines(lines)

	body := normalized
	if frontMatterLines > 0 {
		body = strings.Join(lines[frontMatterLines:], "\n")
	}

	root := parser.New().Parse([]byte(body))

	headings := make([]markdownHeading, 0)
	titleLine := -1
	line := frontMatterLines
	lineStart := 0

	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok {
			continue
		}

		for line+1 < len(lines) && lineStart+len(lines[line]) < heading.Pos() {
			lineStart += len(lines[line]) + 1
			line++
		}

		if heading.Level == titleHeadingLevel && titleLine == -1 && heading.FirstChild() != nil {
			titleLine = line
		}

		headingText, ok := atxHeadingText(lines[line], heading.Level)
		if !ok {
			continue
		}

		headings = append(headings, markdownHeading{
			level: heading.Level,
			line:  line,
			text:  headingText,
		})
	}

	return markdownIndex{
		lines:            lines,
		headings:         headings,
		frontMatterLines: frontMatterLines,
		titleLine:        titleLine,
	}
}

func (m markdownIndex) headingsAtLevel(level int) []markdownHeading {
	headings := make([]markdownHeading, 0)

	for _, heading := range m.headings {
		if heading.level == level {
			headings = append(headings, heading)
		}
	}

	return headings
}

func (m markdownIndex) hasHeader(releaseLine int) bool {
	if m.frontMatterLines > 0 {
		return true
	}

	if m.titleLine < 0 {
		return false
	}

	return releaseLine < 0 || m.titleLine < releaseLine
}

func yamlFrontMatterLines(lines []string) int {
	if strings.TrimRight(lines[0], " \t") != yamlFrontMatterFence {
		return 0
	}

	for idx := 1; idx < len(lines); idx++ {
		trimmed := strings.TrimRight(lines[idx], " \t")
		if trimmed != yamlFrontMatterFence && trimmed != yamlFrontMatterEndFence {
			continue
		}

		payload := strings.Join(lines[1:idx], "\n")
		if isYAMLFrontMatterPayload(payload) {
			return idx + 1
		}

		return 0
	}

	return 0
}

func isYAMLFrontMatterPayload(payload string) bool {
	if strings.TrimSpace(payload) == "" {
		return true
	}

	var mapping map[string]any

	err := yaml.Unmarshal([]byte(payload), &mapping)
	if err != nil {
		return false
	}

	return len(mapping) > 0
}

func splitUTF8BOM(text string) (string, string) {
	if body, hasBOM := strings.CutPrefix(text, utf8BOM); hasBOM {
		return utf8BOM, body
	}

	return "", text
}

func atxHeadingText(line string, level int) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}

	if indent > maxHeadingIndent {
		return "", false
	}

	prefix := strings.Repeat("#", level) + " "
	heading, ok := strings.CutPrefix(line[indent:], prefix)

	return heading, ok
}
