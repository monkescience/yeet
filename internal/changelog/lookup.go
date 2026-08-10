package changelog

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEntryNotFound reports that a changelog holds no entry for a tag.
var ErrEntryNotFound = errors.New("changelog entry not found")

// EntryByTag returns the raw text of the entry a changelog holds for a tag,
// bounded by the next release heading. The slice is returned verbatim because
// its consumers publish it as release notes, and re-rendering it would reformat
// entries this process did not write.
func EntryByTag(document, tag string) (string, error) {
	index := newMarkdownIndex(document)

	start, end := entryHeadingRange(index, tag)
	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	entry := strings.TrimSpace(strings.Join(index.lines[start:end], "\n"))
	if entry == "" {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	return entry, nil
}

func entryHeadingRange(document markdownIndex, tag string) (int, int) {
	headings := document.headingsAtLevel(releaseHeadingLevel)

	for pos, heading := range headings {
		entryTag, ok := headingTag(heading.text)
		if !ok {
			continue
		}

		if entryTag != tag {
			continue
		}

		end := len(document.lines)
		if pos+1 < len(headings) {
			end = headings[pos+1].line
		}

		return heading.line, end
	}

	return -1, -1
}

func headingTag(heading string) (string, bool) {
	rest := strings.TrimSpace(heading)
	if rest == "" {
		return "", false
	}

	if strings.HasPrefix(rest, "[") {
		tag, _, ok := bracketTag(rest)

		return tag, ok
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}

	return fields[0], true
}
