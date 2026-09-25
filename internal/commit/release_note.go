package commit

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/v2/ast"
)

const (
	releaseNoteInfo       = "release-note"
	maxReleaseNoteHeading = 3
)

var (
	ErrReleaseNoteUnclosed          = errors.New("release-note fence has no closing fence")
	ErrReleaseNoteEmpty             = errors.New("release-note fence is empty")
	ErrReleaseNoteHeading           = errors.New("release-note fence contains a level 1, 2, or 3 heading")
	ErrReleaseNoteUnclosedCodeBlock = errors.New("release-note fence contains an unclosed code block")
	ErrReleaseNoteUnclosedHTML      = errors.New("release-note fence contains an unclosed HTML block")
)

type markdownDocument struct {
	src        []byte
	root       ast.Node
	lines      []string
	lineStarts []int
	closed     map[ast.Node]int
}

type fencedBlock struct {
	info       string
	content    string
	openOffset int
	openLine   int
	closeLine  int
	closed     bool
}

func ParseWithReleaseNote(ctx context.Context, hash, message string) (Commit, error) {
	if !strings.Contains(message, releaseNoteInfo) {
		return Parse(ctx, hash, message), nil
	}

	message = strings.ReplaceAll(message, "\r\n", "\n")
	note, ignored, err := extractReleaseNote(message)
	parsed := parseIgnoringLines(ctx, hash, message, ignored)
	parsed.Note = note

	return parsed, err
}

func extractReleaseNote(message string) (string, []bool, error) {
	if !strings.Contains(message, releaseNoteInfo) {
		return "", nil, nil
	}

	doc := parseMarkdown(message)
	ignored := make([]bool, len(doc.lines))

	var (
		notes    []string
		firstErr error
	)

	for _, block := range fencedBlocks(doc) {
		if block.info != releaseNoteInfo || block.openLine == 0 {
			continue
		}

		note, err := validReleaseNote(block)
		if err != nil && firstErr == nil {
			firstErr = err
		}

		for line := block.openLine; line <= block.closeLine; line++ {
			ignored[line] = true
		}

		if err == nil {
			notes = append(notes, note)
		}
	}

	return strings.Join(notes, "\n\n"), ignored, firstErr
}

func ReleaseNoteLines(text string) []bool {
	return releaseNoteLines(text, false)
}

func ClosedReleaseNoteLines(text string) []bool {
	return releaseNoteLines(text, true)
}

func releaseNoteLines(text string, onlyClosed bool) []bool {
	if !strings.Contains(text, releaseNoteInfo) {
		return nil
	}

	doc := parseMarkdown(text)
	inside := make([]bool, len(doc.lines))

	for _, block := range fencedBlocks(doc) {
		if block.info != releaseNoteInfo || (onlyClosed && !block.closed) {
			continue
		}

		for line := block.openLine; line <= block.closeLine; line++ {
			inside[line] = true
		}
	}

	return inside
}

func UnclosedBlockOffsets(text string) []int {
	doc := parseMarkdown(text)
	openings := unclosedHTMLOffsets(doc)

	for _, block := range fencedBlocks(doc) {
		if !block.closed {
			openings = append(openings, block.openOffset)
		}
	}

	slices.Sort(openings)

	return openings
}

func validReleaseNote(block fencedBlock) (string, error) {
	if !block.closed {
		return "", ErrReleaseNoteUnclosed
	}

	note := strings.Trim(SanitizeNoteText(block.content), "\n")
	if strings.TrimSpace(note) == "" {
		return "", ErrReleaseNoteEmpty
	}

	doc := parseMarkdown(note)

	var noteErr error

	_ = ast.Walk(doc.root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if heading, ok := node.(*ast.Heading); ok && heading.Level <= maxReleaseNoteHeading {
			noteErr = ErrReleaseNoteHeading

			return ast.WalkStop, nil
		}

		return ast.WalkContinue, nil
	})

	if noteErr != nil {
		return "", noteErr
	}

	for _, inner := range fencedBlocks(doc) {
		if !inner.closed {
			return "", ErrReleaseNoteUnclosedCodeBlock
		}
	}

	if len(unclosedHTMLOffsets(doc)) > 0 {
		return "", ErrReleaseNoteUnclosedHTML
	}

	return note, nil
}

func StripControlCharacters(text string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}

		return r
	}, text)
}

func parseMarkdown(text string) markdownDocument {
	src := []byte(text)
	lines := strings.Split(text, "\n")
	root, closed := parseFencedMarkdown(src)

	return markdownDocument{
		src:        src,
		root:       root,
		lines:      lines,
		lineStarts: lineStartOffsets(lines),
		closed:     closed,
	}
}

func fencedBlocks(doc markdownDocument) []fencedBlock {
	var codeBlocks []*ast.CodeBlock

	_ = ast.Walk(doc.root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if block, ok := node.(*ast.CodeBlock); ok && entering && block.CodeBlockKind == ast.CodeBlockKindFenced {
			codeBlocks = append(codeBlocks, block)
		}

		return ast.WalkContinue, nil
	})

	blocks := make([]fencedBlock, 0, len(codeBlocks))
	for _, block := range codeBlocks {
		openLine := lineAt(doc.lineStarts, block.Pos())
		closeLine := openLine

		if segments := block.Value.Segments(); len(segments) > 0 {
			closeLine = lineAt(doc.lineStarts, segments[len(segments)-1].Start)
		}

		closingOffset, closed := doc.closed[block]
		if closed {
			closeLine = lineAt(doc.lineStarts, closingOffset)
		}

		blocks = append(blocks, fencedBlock{
			info:       strings.TrimSpace(block.Info.Value(doc.src)),
			content:    block.Value.Str(doc.src),
			openOffset: block.Pos(),
			openLine:   openLine,
			closeLine:  closeLine,
			closed:     closed,
		})
	}

	return blocks
}

func unclosedHTMLOffsets(doc markdownDocument) []int {
	var openings []int

	_ = ast.Walk(doc.root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		block, ok := node.(*ast.HTMLBlock)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}

		segments := block.Value.Segments()
		if len(segments) == 0 || closesHTMLBlock(block.HTMLBlockKind, segments[len(segments)-1].Str(doc.src)) {
			return ast.WalkContinue, nil
		}

		start := segments[0].Start
		for start < len(doc.src) && (doc.src[start] == ' ' || doc.src[start] == '\t') {
			start++
		}

		openings = append(openings, start)

		return ast.WalkContinue, nil
	})

	return openings
}

func closesHTMLBlock(kind ast.HTMLBlockKind, line string) bool {
	switch kind {
	case ast.HTMLBlockKind1:
		lower := strings.ToLower(line)

		return strings.Contains(lower, "</script>") || strings.Contains(lower, "</pre>") ||
			strings.Contains(lower, "</style>") || strings.Contains(lower, "</textarea>")
	case ast.HTMLBlockKind2:
		return strings.Contains(line, "-->")
	case ast.HTMLBlockKind3:
		return strings.Contains(line, "?>")
	case ast.HTMLBlockKind4:
		return strings.Contains(line, ">")
	case ast.HTMLBlockKind5:
		return strings.Contains(line, "]]>")
	case ast.HTMLBlockKind6, ast.HTMLBlockKind7:
		return true
	}

	return true
}

func lineStartOffsets(lines []string) []int {
	starts := make([]int, len(lines))
	offset := 0

	for idx, line := range lines {
		starts[idx] = offset
		offset += len(line) + 1
	}

	return starts
}

func lineAt(lineStarts []int, offset int) int {
	idx, _ := slices.BinarySearch(lineStarts, offset+1)

	return max(idx-1, 0)
}
