package changelog

import (
	"strings"
	"time"
)

const markdownFenceSize = 3

// ParseEntry reads a rendered changelog entry back into structure. It is the
// one seam foreign text enters: an entry read off a release branch, written by
// an older version of yeet, hand-edited, or imported from another tool.
//
// Freeform text before the first level-3 section is recovered as the intro.
// Level-3 headings inside fenced code blocks remain part of that freeform text.
func ParseEntry(text string) Entry {
	lines := splitLines(text)

	entry, bodyStart := parseEntryHeading(lines)
	body := lines[bodyStart:]
	sectionStarts := findSectionStarts(body)

	introEnd := len(body)
	if len(sectionStarts) > 0 {
		introEnd = sectionStarts[0]
	}

	entry.Intro = trimBlankEdges(body[:introEnd])
	entry.Sections = parseSections(body, sectionStarts)

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

func findSectionStarts(lines []string) []int {
	return findHeadingStarts(lines, "### ")
}

func findEntryStarts(lines []string) []int {
	return findHeadingStarts(lines, "## ")
}

func findHeadingStarts(lines []string, prefix string) []int {
	starts := make([]int, 0)
	fenceMarker := byte(0)
	fenceSize := 0

	for idx, line := range lines {
		marker, size, rest, isFence := markdownFence(line)
		if fenceMarker != 0 {
			if isFence && marker == fenceMarker && size >= fenceSize && strings.TrimSpace(rest) == "" {
				fenceMarker = 0
				fenceSize = 0
			}

			continue
		}

		if isFence {
			fenceMarker = marker
			fenceSize = size

			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			starts = append(starts, idx)
		}
	}

	return starts
}

func markdownFence(line string) (byte, int, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, "", false
	}

	marker := trimmed[0]

	size := 0
	for size < len(trimmed) && trimmed[size] == marker {
		size++
	}

	if size < markdownFenceSize {
		return 0, 0, "", false
	}

	return marker, size, trimmed[size:], true
}

func parseSections(lines []string, starts []int) []Section {
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
