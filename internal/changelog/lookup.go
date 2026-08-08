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

	start := entryHeadingIndex(lines, tag)
	if start == -1 {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	end := len(lines)

	for idx := start + 1; idx < len(lines); idx++ {
		if strings.HasPrefix(strings.TrimSpace(lines[idx]), "## ") {
			end = idx

			break
		}
	}

	entry := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if entry == "" {
		return "", fmt.Errorf("%w: %s", ErrEntryNotFound, tag)
	}

	return entry, nil
}

func entryHeadingIndex(lines []string, tag string) int {
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}

		entryTag, ok := headingTag(trimmed)
		if !ok {
			continue
		}

		if entryTag == tag {
			return idx
		}
	}

	return -1
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
