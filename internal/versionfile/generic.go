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
	ErrUnclosedBlockMarker  = errors.New("unclosed x-yeet-start block")
	ErrNestedBlockMarker    = errors.New("nested x-yeet-start inside open block")
	ErrMarkerNoMatch        = errors.New("yeet marker on line without matching version pattern")
	ErrNoMarkersFound       = errors.New("file has no yeet markers")
	ErrMarkerSchemeMismatch = errors.New("yeet marker scope not valid for configured scheme")
	ErrInvalidNextVersion   = errors.New("invalid next version")
	ErrInvalidScheme        = errors.New("invalid versioning scheme")
)

type MarkerError struct {
	Name        string
	Suggestions []string
	Line        int
	Err         error
}

func (e *MarkerError) Error() string {
	return e.Err.Error()
}

func (e *MarkerError) Unwrap() error {
	return e.Err
}

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+)+(?:-[\w.]+)?(?:\+[-\w.]+)?`)

var majorPattern = regexp.MustCompile(`\d+\b`)

var minorPatchPattern = regexp.MustCompile(`\b\d+\b`)

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

type Scheme struct {
	kind   schemeKind
	calver *version.CalVerScheme
}

type schemeKind uint8

const (
	schemeSemVer schemeKind = iota + 1
	schemeCalVer
)

func SemVerScheme() Scheme {
	return Scheme{kind: schemeSemVer}
}

func CalVerScheme(calver *version.CalVerScheme) Scheme {
	return Scheme{kind: schemeCalVer, calver: calver}
}

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
		values:  values,
		allowed: allowed,
		scheme:  scheme,
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

func (s Scheme) markerValues(nextVersion string) (map[markerScope]string, map[markerScope]bool, error) {
	switch s.kind {
	case schemeSemVer:
		return semverValues(nextVersion)
	case schemeCalVer:
		if s.calver == nil {
			return nil, nil, fmt.Errorf("%w: calver format is nil", ErrInvalidScheme)
		}

		return s.calverValues(nextVersion)
	default:
		return nil, nil, fmt.Errorf("%w: unknown scheme", ErrInvalidScheme)
	}
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
	tokens, err := s.calver.MarkerValues(nextVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidNextVersion, err)
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

	name := "x-yeet-" + string(scope)
	suggestions := p.markerSuggestions(scope)

	return &MarkerError{
		Name:        name,
		Suggestions: suggestions,
		Line:        lineNo,
		Err: fmt.Errorf(
			"%w: %q at line %d is not valid for %s (%s)",
			ErrMarkerSchemeMismatch,
			name,
			lineNo,
			p.schemeDescription(),
			p.describeSuggestions(scope, suggestions),
		),
	}
}

func (p *markerParser) markerSuggestions(scope markerScope) []string {
	if p.scheme.kind == schemeCalVer {
		switch scope {
		case markerScopeMajor:
			return []string{"x-yeet-year"}
		case markerScopeMinor:
			if p.scheme.calver.HasWeek() {
				return []string{"x-yeet-week"}
			}

			if p.scheme.calver.HasMonth() {
				return []string{"x-yeet-month"}
			}
		case markerScopePatch:
			return []string{"x-yeet-micro"}
		case markerScopeVersion, markerScopeYear, markerScopeMonth, markerScopeWeek, markerScopeDay, markerScopeMicro:
		}

		return nil
	}

	switch scope {
	case markerScopeYear:
		return []string{"x-yeet-major"}
	case markerScopeMonth, markerScopeWeek:
		return []string{"x-yeet-minor"}
	case markerScopeDay, markerScopeMicro:
		return []string{"x-yeet-patch"}
	case markerScopeVersion, markerScopeMajor, markerScopeMinor, markerScopePatch:
		return nil
	default:
		return nil
	}
}

func (p *markerParser) schemeDescription() string {
	if p.scheme.kind == schemeCalVer {
		return fmt.Sprintf("calver format %q", p.scheme.calver.Format())
	}

	return "semver"
}

func (p *markerParser) describeSuggestions(scope markerScope, suggestions []string) string {
	if p.scheme.kind == schemeCalVer {
		return calVerSuggestion(scope, suggestions)
	}

	return semVerSuggestion(suggestions)
}

func semVerSuggestion(suggestions []string) string {
	if len(suggestions) > 0 {
		return `use "` + suggestions[0] + `"`
	}

	return `valid scopes are "version", "major", "minor", "patch"`
}

func calVerSuggestion(scope markerScope, suggestions []string) string {
	if len(suggestions) > 0 {
		return `use "` + suggestions[0] + `"`
	}

	switch scope {
	case markerScopeMinor:
		return `the configured calver format has no addressable second segment`
	case markerScopeMonth:
		return `the configured calver format has no month token`
	case markerScopeWeek:
		return `the configured calver format has no week token`
	case markerScopeDay:
		return `the configured calver format has no day token`
	case markerScopeVersion, markerScopeYear, markerScopeMicro, markerScopeMajor, markerScopePatch:
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

func replaceForScope(line string, scope markerScope, values map[markerScope]string) (string, bool) {
	value, ok := values[scope]
	if !ok {
		return line, false
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
		return line, false
	}
}

func replaceFirst(pattern *regexp.Regexp, line, replacement string) (string, bool) {
	match := pattern.FindStringIndex(line)
	if match == nil {
		return line, false
	}

	return line[:match[0]] + replacement + line[match[1]:], true
}
