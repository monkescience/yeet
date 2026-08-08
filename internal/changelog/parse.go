package changelog

import (
	"strings"
	"time"
)

// ParseEntry reads a rendered changelog entry back into structure. It is the
// one seam foreign text enters: an entry read off a release branch, written by
// an older version of yeet, hand-edited, or imported from another tool.
//
// Only level-3 sections are recovered. Freeform text directly under the version
// heading, and headings deeper than level 3, are dropped.
func ParseEntry(text string) Entry {
	lines := splitLines(text)

	entry, bodyStart := parseEntryHeading(lines)
	entry.Sections = parseSections(lines[bodyStart:])

	return entry
}

func parseEntryHeading(lines []string) (Entry, int) {
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		heading, isEntryHeading := strings.CutPrefix(trimmed, "## ")
		if !isEntryHeading {
			return Entry{}, idx
		}

		entry := Entry{}
		parseHeadingFields(strings.TrimSpace(heading), &entry)

		return entry, idx + 1
	}

	return Entry{}, len(lines)
}

func parseHeadingFields(heading string, entry *Entry) {
	rest := heading

	if strings.HasPrefix(rest, "[") {
		version, compareURL, remaining, ok := parseLinkedHeading(rest)
		if ok {
			entry.Version = version
			entry.CompareURL = compareURL
			rest = remaining
		}
	} else if fields := strings.Fields(rest); len(fields) > 0 {
		entry.Version = fields[0]
		rest = strings.TrimSpace(strings.TrimPrefix(rest, entry.Version))
	}

	if !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return
	}

	date, err := time.Parse(dateLayout, strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rest, "("), ")")))
	if err != nil {
		return
	}

	entry.Date = date
}

func parseLinkedHeading(heading string) (string, string, string, bool) {
	version, linkPortion, ok := bracketTag(heading)
	if !ok {
		return "", "", heading, false
	}

	afterOpen, found := strings.CutPrefix(linkPortion, "(")
	if !found {
		return "", "", heading, false
	}

	compareURL, remaining, found := strings.Cut(afterOpen, ")")
	if !found {
		return "", "", heading, false
	}

	return version, compareURL, strings.TrimSpace(remaining), true
}

func parseSections(lines []string) []Section {
	starts := make([]int, 0)

	for idx, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			starts = append(starts, idx)
		}
	}

	sections := make([]Section, 0, len(starts))

	for idx, start := range starts {
		end := len(lines)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}

		heading := strings.TrimPrefix(strings.TrimSpace(lines[start]), "### ")

		sections = append(sections, Section{
			Heading: strings.TrimSpace(heading),
			Lines:   trimBlankEdges(lines[start+1 : end]),
		})
	}

	return sections
}

func trimBlankEdges(lines []string) []string {
	first := 0
	for first < len(lines) && strings.TrimSpace(lines[first]) == "" {
		first++
	}

	last := len(lines)
	for last > first && strings.TrimSpace(lines[last-1]) == "" {
		last--
	}

	if first == last {
		return nil
	}

	return append([]string(nil), lines[first:last]...)
}

// splitLines normalizes CRLF to LF and splits the text into lines.
func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

// bracketTag extracts the tag from a "[tag]..." prefix, returning the tag and
// the portion following the closing bracket. It reports false when the text
// does not open with a non-empty "[tag]".
func bracketTag(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "[") {
		return "", "", false
	}

	closeIdx := strings.Index(text, "]")
	if closeIdx <= 1 {
		return "", "", false
	}

	return text[1:closeIdx], text[closeIdx+1:], true
}
