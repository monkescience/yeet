package changelog

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/monkescience/yeet/internal/commit"
)

const breakingChangesHeading = "⚠ BREAKING CHANGES"

type Generator struct {
	sections   map[string]string
	include    []string
	repoURL    string
	pathPrefix string
	compareURL func(fromRef, toRef string) string
	references References
	date       time.Time

	compiledPatterns []compiledPattern
}

type Option func(*Generator)

type References struct {
	Patterns []ReferencePattern
	Footers  map[string]string
}

type ReferencePattern struct {
	Pattern string
	URL     string
}

type compiledPattern struct {
	re  *regexp.Regexp
	url string
}

func New(opts ...Option) *Generator {
	g := &Generator{}
	for _, opt := range opts {
		opt(g)
	}

	return g
}

func WithSections(sections map[string]string) Option {
	return func(g *Generator) { g.sections = sections }
}

func WithInclude(include []string) Option {
	return func(g *Generator) { g.include = include }
}

func WithRepoURL(repoURL string) Option {
	return func(g *Generator) { g.repoURL = repoURL }
}

func WithPathPrefix(pathPrefix string) Option {
	return func(g *Generator) { g.pathPrefix = pathPrefix }
}

func WithCompareURL(compareURL func(fromRef, toRef string) string) Option {
	return func(g *Generator) { g.compareURL = compareURL }
}

func WithReferences(references References) Option {
	return func(g *Generator) { g.references = references }
}

func WithDate(date time.Time) Option {
	return func(g *Generator) { g.date = date }
}

func (g *Generator) Generate(ctx context.Context, version string, previousTag string, commits []commit.Commit) Entry {
	g.ensureCompiledPatterns(ctx)

	relevant := commit.FilterByTypes(commits, g.include)

	sections := g.buildSections(relevant)

	date := g.date
	if date.IsZero() {
		date = time.Now()
	}

	entry := Entry{
		Version:       version,
		Date:          date,
		Sections:      sections,
		OwnedHeadings: g.OwnedHeadings(),
	}

	if g.compareURL != nil && previousTag != "" {
		entry.CompareURL = g.compareURL(previousTag, version)
	}

	slog.DebugContext(ctx, "changelog: generated entry",
		slog.String("version", version),
		slog.String("previous_tag", previousTag),
		slog.Int("commits_in", len(commits)),
		slog.Int("commits_included", len(relevant)),
		slog.Int("sections", len(sections)),
		slog.Int("bytes", len(renderSections(sections))),
	)

	return entry
}

func (g *Generator) OwnedHeadings() []string {
	breakingHeading := g.breakingHeading()
	headings := make([]string, 0, len(g.sections)+len(g.include)+1)

	headings = append(headings, breakingHeading)
	if breakingHeading != breakingChangesHeading {
		headings = append(headings, breakingChangesHeading)
	}

	mapped := make([]string, 0, len(g.sections))
	for _, name := range g.sections {
		mapped = append(mapped, name)
	}

	slices.Sort(mapped)
	headings = append(headings, mapped...)

	for _, commitType := range g.include {
		if _, isMapped := g.sections[commitType]; isMapped {
			continue
		}

		headings = append(headings, capitalizeFirst(commitType))
	}

	return dedupe(headings)
}

func (g *Generator) buildSections(relevant []commit.Commit) []Section {
	grouped := groupBySection(relevant)

	sections := make([]Section, 0, len(g.include)+1)

	if breaking, ok := g.breakingSection(relevant); ok {
		sections = append(sections, breaking)
	}

	for _, commitType := range g.include {
		sectionName, ok := g.sections[commitType]
		if !ok {
			sectionName = capitalizeFirst(commitType)
		}

		sectionCommits, exists := grouped[commitType]
		if !exists {
			continue
		}

		lines := make([]string, 0, len(sectionCommits))
		for _, c := range sectionCommits {
			lines = append(lines, g.formattedLine(c, c.Description))
		}

		sections = append(sections, Section{Heading: sectionName, Lines: lines})
	}

	return sections
}

func (g *Generator) breakingSection(commits []commit.Commit) (Section, bool) {
	lines := make([]string, 0, len(commits))

	for _, c := range commits {
		if !c.Breaking {
			continue
		}

		lines = append(lines, g.formattedLine(c, breakingDescription(c)))
	}

	if len(lines) == 0 {
		return Section{}, false
	}

	return Section{Heading: g.breakingHeading(), Lines: lines}, true
}

func (g *Generator) breakingHeading() string {
	if heading := g.sections["breaking"]; heading != "" {
		return heading
	}

	return breakingChangesHeading
}

func (g *Generator) formattedLine(c commit.Commit, description string) string {
	shortHash := c.Hash
	if len(shortHash) > 7 { //nolint:mnd // standard short hash length
		shortHash = shortHash[:7]
	}

	hashRef := shortHash

	if g.repoURL != "" {
		hashRef = fmt.Sprintf("[%s](%s%s/commit/%s)", shortHash, g.repoURL, g.pathPrefix, c.Hash)
	}

	linked := g.linkDescription(sanitizeCommitText(description))
	scope := sanitizeCommitText(c.Scope)

	var sb strings.Builder

	if scope != "" {
		fmt.Fprintf(&sb, "- **%s:** %s (%s)", scope, linked, hashRef)
	} else {
		fmt.Fprintf(&sb, "- %s (%s)", linked, hashRef)
	}

	if refs := g.footerReferences(c); refs != "" {
		fmt.Fprintf(&sb, " (%s)", refs)
	}

	return sb.String()
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
			slog.WarnContext(ctx, "skipping invalid changelog reference pattern",
				slog.String("pattern", p.Pattern),
			)
			slog.DebugContext(ctx, "changelog reference pattern did not compile",
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
		pattern, ok := g.referencePattern(f.Key)
		if !ok {
			continue
		}

		value := sanitizeCommitText(strings.TrimSpace(f.Value))
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

func (g *Generator) referencePattern(key string) (string, bool) {
	for configured, pattern := range g.references.Footers {
		if strings.EqualFold(configured, key) {
			return pattern, true
		}
	}

	return "", false
}

func groupBySection(commits []commit.Commit) map[string][]commit.Commit {
	grouped := make(map[string][]commit.Commit)

	for _, c := range commits {
		if c.Type == "" {
			continue
		}

		grouped[c.Type] = append(grouped[c.Type], c)
	}

	return grouped
}

func breakingDescription(c commit.Commit) string {
	for _, f := range c.Footers {
		if f.Key == "BREAKING CHANGE" || f.Key == "BREAKING-CHANGE" {
			return strings.TrimRightFunc(f.Value, unicode.IsSpace)
		}
	}

	return c.Description
}

func sanitizeCommitText(s string) string {
	stripped := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}

		if r == '\t' {
			return r
		}

		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, s)

	return strings.NewReplacer("<!--", "&lt;!--", "-->", "--&gt;").Replace(stripped)
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}

	r, size := utf8.DecodeRuneInString(s)

	return string(unicode.ToUpper(r)) + s[size:]
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))

	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	return unique
}
