package changelog

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"
)

const (
	releaseHeadingLevel = 2
	sectionHeadingLevel = 3
	maxHeadingIndent    = 3
)

type markdownHeading struct {
	level int
	line  int
	text  string
}

type markdownIndex struct {
	lines    []string
	headings []markdownHeading
}

func newMarkdownIndex(source string) markdownIndex {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	root := goldmark.DefaultParser().Parse(goldmarktext.NewReader([]byte(normalized)))

	headings := make([]markdownHeading, 0)
	line := 0
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

	return markdownIndex{lines: lines, headings: headings}
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
