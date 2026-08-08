package changelog

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

// Entry is one release's worth of changelog, structured until it is rendered.
type Entry struct {
	Version       string
	Date          time.Time
	CompareURL    string
	Intro         []string
	Sections      []Section
	Outro         []string
	OwnedHeadings []string
}

// Section is a level-3 heading and its lines. Nested sections carry the child
// targets of a derived entry; nesting depth does not map to heading level.
type Section struct {
	Heading  string
	Lines    []string
	Sections []Section
}

// Render writes an entry as changelog Markdown.
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

// RenderBody writes an entry without its release heading.
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

// RenderSections writes a run of sections without an entry heading, for callers
// that frame them in something other than a changelog entry.
func RenderSections(sections []Section) string {
	return renderSections(sections)
}

func renderFreeform(lines []string) string {
	return strings.Join(trimBlankEdges(lines), "\n")
}

// Prepend splices a rendered entry into an existing changelog at the first
// release heading, copying everything already in the file through verbatim.
func Prepend(existing, newEntry string) string {
	const header = "# Changelog\n\n"

	entry := strings.TrimRight(newEntry, "\n")

	if existing == "" {
		return header + entry + "\n"
	}

	releaseStart := strings.Index(existing, "\n## ")
	if releaseStart >= 0 {
		releaseStart++
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

// A section carrying child targets closes with a blank line of its own. Every
// changelog yeet has published for a derived target contains that spacing, so
// it is part of the format rather than an artifact of how it was written.
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
