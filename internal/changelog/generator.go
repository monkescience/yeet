package changelog

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

type Generator struct {
	sections   map[string]string
	include    []string
	repoURL    string
	pathPrefix string
	compareURL func(fromRef, toRef string) string
	references config.ReferencesConfig

	compiledPatterns []compiledPattern
}

// Option configures a Generator.
type Option func(*Generator)

// New builds a changelog Generator from the given options.
func New(opts ...Option) *Generator {
	g := &Generator{}
	for _, opt := range opts {
		opt(g)
	}

	return g
}

// WithSections maps commit types to changelog section headings.
func WithSections(sections map[string]string) Option {
	return func(g *Generator) { g.sections = sections }
}

// WithInclude sets the commit types included in the changelog, in order.
func WithInclude(include []string) Option {
	return func(g *Generator) { g.include = include }
}

// WithRepoURL sets the repository URL used to link commit hashes.
func WithRepoURL(repoURL string) Option {
	return func(g *Generator) { g.repoURL = repoURL }
}

// WithPathPrefix sets the provider-specific path prefix inserted into commit links.
func WithPathPrefix(pathPrefix string) Option {
	return func(g *Generator) { g.pathPrefix = pathPrefix }
}

// WithCompareURL sets the function that builds the version-compare URL.
func WithCompareURL(compareURL func(fromRef, toRef string) string) Option {
	return func(g *Generator) { g.compareURL = compareURL }
}

// WithReferences sets the reference linking configuration.
func WithReferences(references config.ReferencesConfig) Option {
	return func(g *Generator) { g.references = references }
}

type compiledPattern struct {
	re  *regexp.Regexp
	url string
}

type Entry struct {
	Version    string
	Date       time.Time
	Body       string
	CompareURL string
}

func (g *Generator) Generate(ctx context.Context, version string, previousTag string, commits []commit.Commit) Entry {
	g.ensureCompiledPatterns(ctx)

	relevant := commit.FilterByTypes(commits, g.include)

	grouped := g.groupBySection(relevant)

	var sb strings.Builder

	g.writeBreakingChanges(&sb, relevant)

	for _, commitType := range g.include {
		sectionName, ok := g.sections[commitType]
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

	if g.compareURL != nil && previousTag != "" {
		entry.CompareURL = g.compareURL(previousTag, version)
	}

	slog.DebugContext(ctx, "changelog: generated entry",
		slog.String("version", version),
		slog.String("previous_tag", previousTag),
		slog.Int("commits_in", len(commits)),
		slog.Int("commits_included", len(relevant)),
		slog.Int("sections", len(grouped)),
		slog.Int("bytes", len(entry.Body)),
	)

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

	entry := strings.TrimRight(newEntry, "\n")

	if existing == "" {
		return header + entry + "\n"
	}

	if strings.HasPrefix(existing, "# ") {
		idx := strings.Index(existing, "\n\n")
		if idx >= 0 {
			return existing[:idx+2] + entry + "\n\n" + strings.TrimLeft(existing[idx+2:], "\n")
		}
	}

	return header + entry + "\n\n" + strings.TrimLeft(existing, "\n")
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

	if g.repoURL != "" {
		hashRef = fmt.Sprintf("[%s](%s%s/commit/%s)", shortHash, g.repoURL, g.pathPrefix, c.Hash)
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

func (g *Generator) ensureCompiledPatterns(ctx context.Context) {
	if g.compiledPatterns != nil || len(g.references.Patterns) == 0 {
		return
	}

	g.compiledPatterns = make([]compiledPattern, 0, len(g.references.Patterns))

	for _, p := range g.references.Patterns {
		if p.URL == "" {
			continue
		}

		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			slog.WarnContext(ctx, "invalid changelog reference pattern, skipping",
				slog.String("pattern", p.Pattern),
				slog.Any("error", err),
			)

			continue
		}

		g.compiledPatterns = append(g.compiledPatterns, compiledPattern{re: re, url: p.URL})
	}
}

func (g *Generator) linkDescription(description string) string {
	if len(g.compiledPatterns) == 0 {
		return description
	}

	result := description

	for _, cp := range g.compiledPatterns {
		result = cp.re.ReplaceAllStringFunc(result, func(match string) string {
			url := strings.ReplaceAll(cp.url, "{value}", match)

			return fmt.Sprintf("[%s](%s)", match, url)
		})
	}

	return result
}

func (g *Generator) footerReferences(c commit.Commit) string {
	if len(g.references.Footers) == 0 {
		return ""
	}

	var refs []string

	for _, f := range c.Footers {
		pattern, ok := g.references.Footers[f.Key]
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
