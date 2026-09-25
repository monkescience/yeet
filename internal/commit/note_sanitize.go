package commit

import (
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

func SanitizeNoteText(note string) string {
	note = StripControlCharacters(note)
	code := make([]bool, len(note))

	_ = ast.Walk(parser.New().Parse([]byte(note)), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		var indices []text.Index

		switch node := node.(type) {
		case *ast.CodeBlock:
			for _, segment := range node.Value.Segments() {
				indices = append(indices, text.Index{Start: segment.Start, Stop: segment.Stop})
			}
		case *ast.CodeSpan:
			indices = node.Value.Indices()
		default:
			return ast.WalkContinue, nil
		}

		for _, index := range indices {
			for offset := index.Start; offset < index.Stop; offset++ {
				code[offset] = true
			}
		}

		return ast.WalkSkipChildren, nil
	})

	var sanitized strings.Builder

	for offset := 0; offset < len(note); {
		switch {
		case !code[offset] && strings.HasPrefix(note[offset:], "<!--"):
			sanitized.WriteString("&lt;!--")

			offset += len("<!--")
		case !code[offset] && strings.HasPrefix(note[offset:], "-->"):
			sanitized.WriteString("--&gt;")

			offset += len("-->")
		default:
			sanitized.WriteByte(note[offset])
			offset++
		}
	}

	return sanitized.String()
}
