package changelog

import (
	"strings"
	"time"
)

// ParseEntry reads a rendered changelog entry back into structure. It is the
// one seam foreign text enters: an entry read off a release branch, written by
// an older version of yeet, hand-edited, or imported from another tool.
//
// Freeform text before the first level-3 section is recovered as the intro.
// Level-3 headings inside fenced code blocks remain part of that freeform text.
func ParseEntry(text string) Entry {
	document := newMarkdownIndex(text)
	lines := document.lines

	entry, bodyStart := parseEntryHeading(document)
	body := lines[bodyStart:]
	sectionStarts := make([]int, 0)

	for _, heading := range document.headingsAtLevel(sectionHeadingLevel) {
		if heading.line >= bodyStart {
			sectionStarts = append(sectionStarts, heading.line-bodyStart)
		}
	}

	introEnd := len(body)
	if len(sectionStarts) > 0 {
		introEnd = sectionStarts[0]
	}

	entry.Intro = trimBlankEdges(body[:introEnd])
	entry.Sections = parseSections(body, sectionStarts)

	return entry
}

func parseEntryHeading(document markdownIndex) (Entry, int) {
	for idx, line := range document.lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		headings := document.headingsAtLevel(releaseHeadingLevel)
		if len(headings) == 0 || headings[0].line != idx {
			return Entry{}, idx
		}

		entry := Entry{}
		parseHeadingFields(strings.TrimSpace(headings[0].text), &entry)

		return entry, idx + 1
	}

	return Entry{}, len(document.lines)
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

func parseSections(lines []string, starts []int) []Section {
	sections := make([]Section, 0, len(starts))

	for idx, start := range starts {
		end := len(lines)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}

		heading, _ := atxHeadingText(lines[start], sectionHeadingLevel)

		sections = append(sections, Section{
			Heading: trimClosingHeadingHashes(heading),
			Lines:   trimBlankEdges(lines[start+1 : end]),
		})
	}

	return sections
}

func trimClosingHeadingHashes(heading string) string {
	trimmed := strings.TrimRight(heading, " \t")

	hashStart := len(trimmed)
	for hashStart > 0 && trimmed[hashStart-1] == '#' {
		hashStart--
	}

	hasClosingSequence := hashStart < len(trimmed)

	isDetached := hashStart == 0 || trimmed[hashStart-1] == ' ' || trimmed[hashStart-1] == '\t'
	if !hasClosingSequence || !isDetached {
		return strings.TrimSpace(heading)
	}

	return strings.TrimSpace(trimmed[:hashStart])
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
