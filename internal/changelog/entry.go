package changelog

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

type Entry struct {
	Version       string
	Date          time.Time
	CompareURL    string
	Intro         []string
	Sections      []Section
	Outro         []string
	OwnedHeadings []string
}

type Section struct {
	Heading        string
	Lines          []string
	Sections       []Section
	includedTarget bool
}

func Render(entry Entry) string {
	var sb strings.Builder

	if entry.CompareURL != "" {
		fmt.Fprintf(&sb, "## [%s](%s) (%s)\n\n", entry.Version, entry.CompareURL, entry.Date.Format(dateLayout))
	} else {
		fmt.Fprintf(&sb, "## %s (%s)\n\n", entry.Version, entry.Date.Format(dateLayout))
	}

	sb.WriteString(RenderBody(entry))

	return sb.String()
}

func RenderBody(entry Entry) string {
	if len(entry.Intro) == 0 && len(entry.Outro) == 0 {
		return renderSections(entry.Sections)
	}

	var blocks []string
	if intro := renderFreeform(entry.Intro); intro != "" {
		blocks = append(blocks, intro)
	}

	if sections := strings.TrimRight(renderSections(entry.Sections), "\n"); sections != "" {
		blocks = append(blocks, sections)
	}

	if outro := renderFreeform(entry.Outro); outro != "" {
		blocks = append(blocks, outro)
	}

	if len(blocks) == 0 {
		return ""
	}

	return strings.Join(blocks, "\n\n") + "\n"
}

func RenderSections(sections []Section) string {
	return renderSections(sections)
}

func renderFreeform(lines []string) string {
	return strings.Join(trimBlankEdges(lines), "\n")
}

func PrependEntry(existing, newEntry string) string {
	if strings.TrimSpace(existing) == "" {
		return Prepend("", newEntry)
	}

	if strings.HasPrefix(existing, "# ") {
		return Prepend(existing, newEntry)
	}

	combined := strings.TrimRight(newEntry, "\n") + "\n\n" + strings.TrimLeft(existing, "\n")

	return Prepend("", combined)
}

func DirectSections(sections []Section) []Section {
	direct := make([]Section, 0, len(sections))

	for _, section := range sections {
		if !section.includedTarget {
			direct = append(direct, section)
		}
	}

	return direct
}

func DerivedEntry(direct Entry, ownedHeadings []string, children []Section) Entry {
	sections := make([]Section, 0, len(direct.Sections)+len(children))
	sections = append(sections, direct.Sections...)

	for _, child := range children {
		child.includedTarget = true
		sections = append(sections, child)
	}

	owned := make([]string, 0, len(direct.OwnedHeadings)+len(ownedHeadings))
	owned = append(owned, direct.OwnedHeadings...)
	owned = append(owned, ownedHeadings...)

	derived := direct
	derived.CompareURL = ""
	derived.Sections = sections
	derived.OwnedHeadings = owned

	return derived
}

func Prepend(existing, newEntry string) string {
	const header = "# Changelog\n\n"

	entry := strings.TrimRight(newEntry, "\n")

	if existing == "" {
		return header + entry + "\n"
	}

	releaseStart := -1

	for _, heading := range newMarkdownIndex(existing).headingsAtLevel(releaseHeadingLevel) {
		if isReleaseHeading(heading.text) {
			releaseStart = lineStartOffset(existing, heading.line)

			break
		}
	}

	if strings.HasPrefix(existing, "# ") {
		if releaseStart >= 0 {
			preamble := strings.TrimRight(existing[:releaseStart], "\n")
			releases := strings.TrimLeft(existing[releaseStart:], "\n")

			return preamble + "\n\n" + entry + "\n\n" + releases
		}

		return strings.TrimRight(existing, "\n") + "\n\n" + entry + "\n"
	}

	return header + entry + "\n\n" + strings.TrimLeft(existing, "\n")
}

func isReleaseHeading(heading string) bool {
	const (
		minimumVersionParts = 2
		semverParts         = 3
	)

	entry := Entry{}
	parseHeadingFields(strings.TrimSpace(trimClosingHeadingHashes(heading)), &entry)

	for start := range len(entry.Version) {
		if entry.Version[start] < '0' || entry.Version[start] > '9' {
			continue
		}

		parts := 0
		position := start

		for position < len(entry.Version) {
			partStart := position
			for position < len(entry.Version) && entry.Version[position] >= '0' && entry.Version[position] <= '9' {
				position++
			}

			if position == partStart {
				break
			}

			parts++
			if position == len(entry.Version) {
				return parts >= minimumVersionParts
			}

			if parts >= semverParts && (entry.Version[position] == '-' || entry.Version[position] == '+') {
				return position+1 < len(entry.Version)
			}

			if entry.Version[position] != '.' {
				break
			}

			position++
		}
	}

	return false
}

func lineStartOffset(text string, line int) int {
	offset := 0

	for range line {
		newline := strings.IndexByte(text[offset:], '\n')
		if newline == -1 {
			return len(text)
		}

		offset += newline + 1
	}

	return offset
}

func renderSections(sections []Section) string {
	var sb strings.Builder

	for idx, section := range sections {
		if idx > 0 {
			sb.WriteString(sectionSeparator(sections[idx-1]))
		}

		writeSection(&sb, section)
	}

	return sb.String()
}

func sectionSeparator(previous Section) string {
	if len(previous.Sections) > 0 {
		return "\n\n"
	}

	return "\n"
}

func writeSection(sb *strings.Builder, section Section) {
	fmt.Fprintf(sb, "### %s\n\n", section.Heading)

	for _, line := range section.Lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString(renderSections(section.Sections))
}
