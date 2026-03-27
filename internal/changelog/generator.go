// Package changelog generates markdown changelogs from conventional commits.
package changelog

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

type Generator struct {
	Sections   map[string]string
	Include    []string
	RepoURL    string
	PathPrefix string
	References config.ReferencesConfig
}

type Entry struct {
	Version    string
	Date       time.Time
	Body       string
	CompareURL string
}

func (g *Generator) Generate(version string, previousTag string, commits []commit.Commit) Entry {
	relevant := commit.FilterByTypes(commits, g.Include)

	grouped := g.groupBySection(relevant)

	var sb strings.Builder

	g.writeBreakingChanges(&sb, relevant)

	for _, commitType := range g.Include {
		sectionName, ok := g.Sections[commitType]
		if !ok {
			sectionName = capitalizeFirst(commitType)
		}

		sectionCommits, exists := grouped[commitType]
		if !exists {
			continue
		}

		writeSectionHeader(&sb, sectionName)

		for _, c := range sectionCommits {
			g.writeCommitLine(&sb, c)
		}
	}

	entry := Entry{
		Version: version,
		Date:    time.Now(),
		Body:    sb.String(),
	}

	if g.RepoURL != "" && previousTag != "" {
		entry.CompareURL = fmt.Sprintf("%s%s/compare/%s...%s", g.RepoURL, g.PathPrefix, previousTag, version)
	}

	return entry
}

func Render(entry Entry) string {
	var sb strings.Builder

	if entry.CompareURL != "" {
		fmt.Fprintf(&sb, "## [%s](%s) (%s)\n\n", entry.Version, entry.CompareURL, entry.Date.Format("2006-01-02"))
	} else {
		fmt.Fprintf(&sb, "## %s (%s)\n\n", entry.Version, entry.Date.Format("2006-01-02"))
	}

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

	writeSectionHeader(sb, "\u26a0 BREAKING CHANGES")

	for _, c := range breaking {
		desc := c.Description

		// Check for BREAKING CHANGE footer with more detail.
		for _, f := range c.Footers {
			if f.Key == "BREAKING CHANGE" || f.Key == "BREAKING-CHANGE" {
				desc = f.Value

				break
			}
		}

		g.writeFormattedLine(sb, c, desc)
	}
}

func writeSectionHeader(sb *strings.Builder, name string) {
	fmt.Fprintf(sb, "### %s\n\n", name)
}

func (g *Generator) writeCommitLine(sb *strings.Builder, c commit.Commit) {
	g.writeFormattedLine(sb, c, c.Description)
}

func (g *Generator) writeFormattedLine(sb *strings.Builder, c commit.Commit, description string) {
	shortHash := c.Hash
	if len(shortHash) > 7 { //nolint:mnd // standard short hash length
		shortHash = shortHash[:7]
	}

	hashRef := shortHash

	if g.RepoURL != "" {
		hashRef = fmt.Sprintf("[%s](%s%s/commit/%s)", shortHash, g.RepoURL, g.PathPrefix, c.Hash)
	}

	linked := g.linkDescription(description)

	if c.Scope != "" {
		fmt.Fprintf(sb, "- **%s:** %s (%s)", c.Scope, linked, hashRef)
	} else {
		fmt.Fprintf(sb, "- %s (%s)", linked, hashRef)
	}

	if refs := g.footerReferences(c); refs != "" {
		fmt.Fprintf(sb, " (%s)", refs)
	}

	sb.WriteString("\n")
}

func (g *Generator) linkDescription(description string) string {
	if len(g.References.Patterns) == 0 {
		return description
	}

	result := description

	for _, p := range g.References.Patterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			continue
		}

		if p.URL == "" {
			continue
		}

		result = re.ReplaceAllStringFunc(result, func(match string) string {
			url := strings.ReplaceAll(p.URL, "{value}", match)

			return fmt.Sprintf("[%s](%s)", match, url)
		})
	}

	return result
}

func (g *Generator) footerReferences(c commit.Commit) string {
	if len(g.References.Footers) == 0 {
		return ""
	}

	var refs []string

	for _, f := range c.Footers {
		pattern, ok := g.References.Footers[f.Key]
		if !ok {
			continue
		}

		value := strings.TrimSpace(f.Value)
		if value == "" {
			continue
		}

		if pattern == "" {
			refs = append(refs, value)
		} else {
			url := strings.ReplaceAll(pattern, "{value}", value)
			refs = append(refs, fmt.Sprintf("[%s](%s)", value, url))
		}
	}

	if len(refs) == 0 {
		return ""
	}

	return strings.Join(refs, ", ")
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}

	r, size := utf8.DecodeRuneInString(s)

	return string(unicode.ToUpper(r)) + s[size:]
}
