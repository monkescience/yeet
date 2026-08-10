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
	lines := splitLines(document)

	start, end := entryHeadingRange(lines, tag)
	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	entry := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if entry == "" {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	return entry, nil
}

func entryHeadingRange(lines []string, tag string) (int, int) {
	starts := findEntryStarts(lines)

	for pos, start := range starts {
		entryTag, ok := headingTag(strings.TrimSpace(lines[start]))
		if !ok {
			continue
		}

		if entryTag != tag {
			continue
		}

		end := len(lines)
		if pos+1 < len(starts) {
			end = starts[pos+1]
		}

		return start, end
	}

	return -1, -1
}

func headingTag(line string) (string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "## "))
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
