// Package changelog generates markdown changelogs from conventional commits.
package changelog

import (
	"fmt"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/commit"
)

type Generator struct {
	Sections map[string]string
	Include  []string
}

type Entry struct {
	Version string
	Date    time.Time
	Body    string
}

func (g *Generator) Generate(version string, commits []commit.Commit) Entry {
	relevant := commit.FilterByTypes(commits, g.Include)

	grouped := g.groupBySection(relevant)

	var sb strings.Builder

	g.writeBreakingChanges(&sb, relevant)

	for _, commitType := range g.Include {
		sectionName, ok := g.Sections[commitType]
		if !ok {
			sectionName = strings.Title(commitType) //nolint:staticcheck // simple title case is fine here
		}

		sectionCommits, exists := grouped[commitType]
		if !exists {
			continue
		}

		writeSectionHeader(&sb, sectionName)

		for _, c := range sectionCommits {
			writeCommitLine(&sb, c)
		}
	}

	return Entry{
		Version: version,
		Date:    time.Now(),
		Body:    sb.String(),
	}
}

func Render(entry Entry) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "## %s (%s)\n\n", entry.Version, entry.Date.Format("2006-01-02"))
	sb.WriteString(entry.Body)

	return sb.String()
}

func Prepend(existing, newEntry string) string {
	const header = "# Changelog\n\n"

	if existing == "" {
		return header + newEntry
	}

	// If there's an existing header, insert after it.
	if strings.HasPrefix(existing, "# ") {
		idx := strings.Index(existing, "\n\n")
		if idx >= 0 {
			return existing[:idx+2] + newEntry + "\n" + existing[idx+2:]
		}
	}

	return header + newEntry + "\n" + existing
}

func (g *Generator) groupBySection(commits []commit.Commit) map[string][]commit.Commit {
	grouped := make(map[string][]commit.Commit)

	for _, c := range commits {
		if c.Type == "" {
			continue
		}

		grouped[c.Type] = append(grouped[c.Type], c)
	}

	return grouped
}

func (g *Generator) writeBreakingChanges(sb *strings.Builder, commits []commit.Commit) {
	var breaking []commit.Commit

	for _, c := range commits {
		if c.Breaking {
			breaking = append(breaking, c)
		}
	}

	if len(breaking) == 0 {
		return
	}

	writeSectionHeader(sb, "Breaking Changes")

	for _, c := range breaking {
		desc := c.Description

		// Check for BREAKING CHANGE footer with more detail.
		for _, f := range c.Footers {
			if f.Key == "BREAKING CHANGE" || f.Key == "BREAKING-CHANGE" {
				desc = f.Value

				break
			}
		}

		writeFormattedLine(sb, c, desc)
	}
}

func writeSectionHeader(sb *strings.Builder, name string) {
	fmt.Fprintf(sb, "### %s\n\n", name)
}

func writeCommitLine(sb *strings.Builder, c commit.Commit) {
	writeFormattedLine(sb, c, c.Description)
}

func writeFormattedLine(sb *strings.Builder, c commit.Commit, description string) {
	shortHash := c.Hash
	if len(shortHash) > 7 { //nolint:mnd // standard short hash length
		shortHash = shortHash[:7]
	}

	if c.Scope != "" {
		fmt.Fprintf(sb, "- **%s**: %s (%s)\n", c.Scope, description, shortHash)
	} else {
		fmt.Fprintf(sb, "- %s (%s)\n", description, shortHash)
	}
}
