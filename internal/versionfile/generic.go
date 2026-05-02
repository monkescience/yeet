// Package versionfile updates version references in configured files.
package versionfile

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/version"
)

type markerScope string

const (
	markerScopeVersion markerScope = "version"
	markerScopeMajor   markerScope = "major"
	markerScopeMinor   markerScope = "minor"
	markerScopePatch   markerScope = "patch"
	markerScopeYear    markerScope = "year"
	markerScopeMonth   markerScope = "month"
	markerScopeWeek    markerScope = "week"
	markerScopeDay     markerScope = "day"
	markerScopeMicro   markerScope = "micro"

	semVerPartCount = 3
)

// allMarkerScopes is the single source of truth for the supported scope names.
// The marker regex and the value-extraction switch both derive from it.
var allMarkerScopes = []markerScope{
	markerScopeMajor,
	markerScopeMinor,
	markerScopePatch,
	markerScopeVersion,
	markerScopeYear,
	markerScopeMonth,
	markerScopeWeek,
	markerScopeDay,
	markerScopeMicro,
}

var (
	// ErrUnclosedBlockMarker is returned when an x-yeet-start-* block has no matching x-yeet-end.
	ErrUnclosedBlockMarker = errors.New("unclosed x-yeet-start block")
	// ErrNestedBlockMarker is returned when an x-yeet-start-* appears inside an already-open block.
	ErrNestedBlockMarker = errors.New("nested x-yeet-start inside open block")
	// ErrMarkerNoMatch is returned when an inline marker's line has no value matching the expected pattern.
	ErrMarkerNoMatch = errors.New("yeet marker on line without matching version pattern")
	// ErrNoMarkersFound is returned when a configured version file has no yeet markers at all.
	ErrNoMarkersFound = errors.New("file has no yeet markers")
	// ErrMarkerSchemeMismatch is returned when a marker's scope is not valid for the
	// configured versioning scheme (or, for CalVer, not present in the configured format).
	ErrMarkerSchemeMismatch = errors.New("yeet marker scope not valid for configured scheme")
	// ErrInvalidNextVersion is returned when the next version cannot be parsed under the configured scheme.
	ErrInvalidNextVersion = errors.New("invalid next version")
)

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+)+(?:-[\w.]+)?(?:\+[-\w.]+)?`)

var majorPattern = regexp.MustCompile(`\d+\b`)

var minorPatchPattern = regexp.MustCompile(`\b\d+\b`)

// commentPrefix requires a real comment opener (not arbitrary text like
// backticks or list markers) before a yeet marker. This keeps prose mentions
// of marker names in READMEs from being interpreted as live markers.
const commentPrefix = `(?:#+|//+|/\*+|--+|;+|<!--)[ \t]*`

var scopeAlternation = buildScopeAlternation()

var inlineMarkerPattern = regexp.MustCompile(commentPrefix + `x-yeet-` + scopeAlternation + `\b`)

var blockStartPattern = regexp.MustCompile(commentPrefix + `x-yeet-start-` + scopeAlternation + `\b`)

var blockEndPattern = regexp.MustCompile(commentPrefix + `x-yeet-end\b`)

func buildScopeAlternation() string {
	parts := make([]string, len(allMarkerScopes))
	for i, scope := range allMarkerScopes {
		parts[i] = string(scope)
	}

	return "(" + strings.Join(parts, "|") + ")"
}

// Scheme tells ApplyGenericMarkers which marker scopes are valid and how to
// extract token values from the next-version string. Use SemVerScheme or
// CalVerScheme to construct.
type Scheme struct {
	calver *version.CalVerScheme
}

// SemVerScheme returns the scheme for SemVer repositories.
func SemVerScheme() Scheme {
	return Scheme{}
}

// CalVerScheme returns the scheme for CalVer repositories using the given
// compiled CalVer format. The compiled format is reused across files so the
// caller pays the compilation cost once per target.
func CalVerScheme(calver *version.CalVerScheme) Scheme {
	return Scheme{calver: calver}
}

// ApplyGenericMarkers applies yeet marker-based version replacements to file content.
// It returns the updated content, whether anything changed, and an error describing any
// structural problem (unclosed/nested blocks, inline markers without a matching pattern,
// markers with a scope not valid for the scheme, or a file with no markers at all).
func ApplyGenericMarkers(content, nextVersion string, scheme Scheme) (string, bool, error) {
	if content == "" {
		return content, false, nil
	}

	values, allowed, err := scheme.markerValues(nextVersion)
	if err != nil {
		return content, false, err
	}

	lines := strings.Split(content, "\n")
	updated := make([]string, 0, len(lines))

	parser := &markerParser{
		nextVersion: nextVersion,
		values:      values,
		allowed:     allowed,
		scheme:      scheme,
	}

	for i, line := range lines {
		newLine, lineErr := parser.processLine(line, i+1)
		if lineErr != nil {
			return content, false, lineErr
		}

		updated = append(updated, newLine)
	}

	if parser.blockScope != "" {
		return content, false, fmt.Errorf(
			"%w: started at line %d", ErrUnclosedBlockMarker, parser.blockStartLine,
		)
	}

	if parser.markerCount == 0 {
		return content, false, ErrNoMarkersFound
	}

	result := strings.Join(updated, "\n")

	return result, result != content, nil
}

// markerValues parses nextVersion under the scheme and returns the per-scope
// substitution values, plus the set of scopes that are valid for this scheme.
func (s Scheme) markerValues(nextVersion string) (map[markerScope]string, map[markerScope]bool, error) {
	if s.calver != nil {
		return s.calverValues(nextVersion)
	}

	return semverValues(nextVersion)
}

func semverValues(nextVersion string) (map[markerScope]string, map[markerScope]bool, error) {
	stripped := nextVersion
	if idx := strings.IndexAny(stripped, "-+"); idx >= 0 {
		stripped = stripped[:idx]
	}

	parts := strings.Split(stripped, ".")
	if len(parts) < semVerPartCount {
		return nil, nil, fmt.Errorf(
			"%w: semver next version %q must have at least %d dot-separated parts",
			ErrInvalidNextVersion, nextVersion, semVerPartCount,
		)
	}

	values := map[markerScope]string{
		markerScopeVersion: nextVersion,
		markerScopeMajor:   parts[0],
		markerScopeMinor:   parts[1],
		markerScopePatch:   parts[2],
	}

	allowed := map[markerScope]bool{
		markerScopeVersion: true,
		markerScopeMajor:   true,
		markerScopeMinor:   true,
		markerScopePatch:   true,
	}

	return values, allowed, nil
}

func (s Scheme) calverValues(nextVersion string) (map[markerScope]string, map[markerScope]bool, error) {
	// CalVer never carries prerelease/build suffixes (see README "Prerelease
	// channels"), so the next version is parsed verbatim.
	tokens, err := s.calver.MarkerValues(nextVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInvalidNextVersion, err)
	}

	values := map[markerScope]string{
		markerScopeVersion: nextVersion,
	}

	for token, rendered := range tokens {
		values[calverMarkerScope(token)] = rendered
	}

	allowed := map[markerScope]bool{
		markerScopeVersion: true,
		markerScopeYear:    true,
		markerScopeMicro:   true,
	}

	if s.calver.HasMonth() {
		allowed[markerScopeMonth] = true
	}

	if s.calver.HasWeek() {
		allowed[markerScopeWeek] = true
	}

	if s.calver.HasDay() {
		allowed[markerScopeDay] = true
	}

	return values, allowed, nil
}

func calverMarkerScope(token version.MarkerToken) markerScope {
	switch token {
	case version.MarkerTokenYear:
		return markerScopeYear
	case version.MarkerTokenMonth:
		return markerScopeMonth
	case version.MarkerTokenWeek:
		return markerScopeWeek
	case version.MarkerTokenDay:
		return markerScopeDay
	case version.MarkerTokenMicro:
		return markerScopeMicro
	default:
		return ""
	}
}

type markerParser struct {
	nextVersion    string
	values         map[markerScope]string
	allowed        map[markerScope]bool
	scheme         Scheme
	blockScope     markerScope
	blockStartLine int
	markerCount    int
}

func (p *markerParser) processLine(line string, lineNo int) (string, error) {
	if p.blockScope != "" {
		return p.processBlockLine(line, lineNo)
	}

	if scope, isInline := markerScopeFromLine(line, inlineMarkerPattern); isInline {
		err := p.checkAllowed(scope, lineNo)
		if err != nil {
			return line, err
		}

		p.markerCount++

		newLine, matched := replaceForScope(line, scope, p.values)
		if !matched {
			return line, fmt.Errorf("%w: line %d (%s)", ErrMarkerNoMatch, lineNo, scope)
		}

		return newLine, nil
	}

	if scope, isBlockStart := markerScopeFromLine(line, blockStartPattern); isBlockStart {
		err := p.checkAllowed(scope, lineNo)
		if err != nil {
			return line, err
		}

		p.blockScope = scope
		p.blockStartLine = lineNo
		p.markerCount++
	}

	return line, nil
}

// processBlockLine substitutes values inside an open block. Block markers
// intentionally tolerate non-matching lines (e.g. yaml structure) — only the
// inline marker form requires a numeric value on the marker line itself.
func (p *markerParser) processBlockLine(line string, lineNo int) (string, error) {
	if _, isNested := markerScopeFromLine(line, blockStartPattern); isNested {
		return line, fmt.Errorf(
			"%w: open at line %d, nested at line %d",
			ErrNestedBlockMarker, p.blockStartLine, lineNo,
		)
	}

	if blockEndPattern.MatchString(line) {
		p.blockScope = ""

		return line, nil
	}

	newLine, _ := replaceForScope(line, p.blockScope, p.values)

	return newLine, nil
}

func (p *markerParser) checkAllowed(scope markerScope, lineNo int) error {
	if p.allowed[scope] {
		return nil
	}

	return fmt.Errorf(
		"%w: %q at line %d is not valid for %s; %s",
		ErrMarkerSchemeMismatch,
		"x-yeet-"+string(scope),
		lineNo,
		p.schemeDescription(),
		p.suggestion(scope),
	)
}

func (p *markerParser) schemeDescription() string {
	if p.scheme.calver != nil {
		return fmt.Sprintf("calver format %q", p.scheme.calver.Format())
	}

	return "semver"
}

func (p *markerParser) suggestion(scope markerScope) string {
	if p.scheme.calver == nil {
		return semVerSuggestion(scope)
	}

	return calVerSuggestion(scope, p.scheme.calver)
}

// semVerSuggestion returns hint text for a marker scope rejected by a semver
// scheme. Only calver-only scopes can land here in practice; the default
// branch covers the remaining enum values to keep the exhaustive linter happy.
func semVerSuggestion(scope markerScope) string {
	switch scope {
	case markerScopeYear:
		return `use "x-yeet-major"`
	case markerScopeMonth, markerScopeWeek:
		return `use "x-yeet-minor"`
	case markerScopeDay, markerScopeMicro:
		return `use "x-yeet-patch"`
	case markerScopeVersion, markerScopeMajor, markerScopeMinor, markerScopePatch:
		return `valid scopes are "version", "major", "minor", "patch"`
	default:
		return `valid scopes are "version", "major", "minor", "patch"`
	}
}

// calVerSuggestion returns hint text for a marker scope rejected by a calver
// scheme. Most rejections are semver-only scopes or calver scopes whose
// corresponding token is absent from the configured format.
func calVerSuggestion(scope markerScope, calver *version.CalVerScheme) string {
	switch scope {
	case markerScopeMajor:
		return `use "x-yeet-year"`
	case markerScopeMinor:
		switch {
		case calver.HasWeek():
			return `use "x-yeet-week"`
		case calver.HasMonth():
			return `use "x-yeet-month"`
		default:
			return `the configured calver format has no addressable second segment`
		}
	case markerScopePatch:
		return `use "x-yeet-micro"`
	case markerScopeMonth:
		return `the configured calver format has no month token`
	case markerScopeWeek:
		return `the configured calver format has no week token`
	case markerScopeDay:
		return `the configured calver format has no day token`
	case markerScopeVersion, markerScopeYear, markerScopeMicro:
		return `check the configured calver format`
	default:
		return `check the configured calver format`
	}
}

func markerScopeFromLine(line string, pattern *regexp.Regexp) (markerScope, bool) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) < 2 { //nolint:mnd // marker regex has one capture group for scope
		return "", false
	}

	return markerScope(matches[1]), true
}

// replaceForScope returns the possibly-updated line along with whether the
// numeric pattern for this scope found a replacement target on the line.
// The bool lets callers distinguish "marker present but value missing" from
// "no numeric target on the line" — only the latter indicates user error
// for inline markers.
func replaceForScope(line string, scope markerScope, values map[markerScope]string) (string, bool) {
	value, ok := values[scope]
	if !ok {
		return line, true
	}

	switch scope {
	case markerScopeVersion:
		return replaceFirst(versionPattern, line, value)
	case markerScopeMajor, markerScopeYear:
		return replaceFirst(majorPattern, line, value)
	case markerScopeMinor, markerScopePatch,
		markerScopeMonth, markerScopeWeek, markerScopeDay, markerScopeMicro:
		return replaceFirst(minorPatchPattern, line, value)
	default:
		return line, true
	}
}

func replaceFirst(pattern *regexp.Regexp, line, replacement string) (string, bool) {
	match := pattern.FindStringIndex(line)
	if match == nil {
		return line, false
	}

	return line[:match[0]] + replacement + line[match[1]:], true
}
