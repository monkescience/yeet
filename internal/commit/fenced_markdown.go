package commit

import (
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

const trackedFencePriority = 699

type trackedFenceParser struct {
	parser.BlockParser
	closed map[ast.Node]int
}

func (p *trackedFenceParser) Continue(node ast.Node, reader text.Reader, ctx parser.Context) parser.State {
	_, segment := reader.PeekLine()
	state := p.BlockParser.Continue(node, reader, ctx)

	if state == parser.Close {
		p.closed[node] = segment.Start
	}

	return state
}

func parseFencedMarkdown(source []byte) (ast.Node, map[ast.Node]int) {
	closed := make(map[ast.Node]int)
	fences := &trackedFenceParser{
		BlockParser: parser.NewFencedCodeBlockParser(),
		closed:      closed,
	}
	options := parser.WithBlockParsers(util.Prioritized[parser.BlockParser](fences, trackedFencePriority))
	root := parser.New(options).Parse(source)

	return root, closed
}
