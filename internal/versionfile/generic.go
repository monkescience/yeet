// Package versionfile updates version references in configured files.
package versionfile

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type markerScope string

const (
	markerScopeVersion markerScope = "version"
	markerScopeMajor   markerScope = "major"
	markerScopeMinor   markerScope = "minor"
	markerScopePatch   markerScope = "patch"
	markerScopeYear    markerScope = "year"
	markerScopeMonth   markerScope = "month"
	markerScopeMicro   markerScope = "micro"

	minimumVersionPartCount = 3
)

var (
	// ErrUnclosedBlockMarker is returned when an x-yeet-start-* block has no matching x-yeet-end.
	ErrUnclosedBlockMarker = errors.New("unclosed x-yeet-start block")
	// ErrNestedBlockMarker is returned when an x-yeet-start-* appears inside an already-open block.
	ErrNestedBlockMarker = errors.New("nested x-yeet-start inside open block")
	// ErrMarkerNoMatch is returned when an inline marker's line has no value matching the expected pattern.
	ErrMarkerNoMatch = errors.New("yeet marker on line without matching version pattern")
	// ErrNoMarkersFound is returned when a configured version file has no yeet markers at all.
	ErrNoMarkersFound = errors.New("file has no yeet markers")
)

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+){2,}(?:-[\w.]+)?(?:\+[-\w.]+)?`)

var majorPattern = regexp.MustCompile(`\d+\b`)

var minorPatchPattern = regexp.MustCompile(`\b\d+\b`)

// commentPrefix requires a real comment opener (not arbitrary text like
// backticks or list markers) before a yeet marker. This keeps prose mentions
// of marker names in READMEs from being interpreted as live markers.
const commentPrefix = `(?:#+|//+|/\*+|--+|;+|<!--)[ \t]*`

const scopeAlternation = `(major|minor|patch|version|year|month|micro)`

var inlineMarkerPattern = regexp.MustCompile(commentPrefix + `x-yeet-` + scopeAlternation + `\b`)

var blockStartPattern = regexp.MustCompile(commentPrefix + `x-yeet-start-` + scopeAlternation + `\b`)

var blockEndPattern = regexp.MustCompile(commentPrefix + `x-yeet-end\b`)

// ApplyGenericMarkers applies yeet marker-based version replacements to file content.
// It returns the updated content, whether anything changed, and an error describing any
// structural problem (unclosed/nested blocks, inline markers without a matching pattern,
// or a file with no markers at all).
func ApplyGenericMarkers(content, nextVersion string) (string, bool, error) {
	if content == "" {
		return content, false, nil
	}

	major, minor, patch := splitVersion(nextVersion)
	lines := strings.Split(content, "\n")
	updated := make([]string, 0, len(lines))

	parser := &markerParser{
		nextVersion: nextVersion,
		major:       major,
		minor:       minor,
		patch:       patch,
	}

	for i, line := range lines {
		newLine, err := parser.processLine(line, i+1)
		if err != nil {
			return content, false, err
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

type markerParser struct {
	nextVersion         string
	major, minor, patch string
	blockScope          markerScope
	blockStartLine      int
	markerCount         int
}

func (p *markerParser) processLine(line string, lineNo int) (string, error) {
	if p.blockScope != "" {
		return p.processBlockLine(line, lineNo)
	}

	if scope, isInline := markerScopeFromLine(line, inlineMarkerPattern); isInline {
		p.markerCount++

		newLine, matched := replaceForScope(line, scope, p.nextVersion, p.major, p.minor, p.patch)
		if !matched {
			return line, fmt.Errorf("%w: line %d (%s)", ErrMarkerNoMatch, lineNo, scope)
		}

		return newLine, nil
	}

	if scope, isBlockStart := markerScopeFromLine(line, blockStartPattern); isBlockStart {
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

	newLine, _ := replaceForScope(line, p.blockScope, p.nextVersion, p.major, p.minor, p.patch)
	if blockEndPattern.MatchString(line) {
		p.blockScope = ""
	}

	return newLine, nil
}

func markerScopeFromLine(line string, pattern *regexp.Regexp) (markerScope, bool) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) < 2 { //nolint:mnd // marker regex has one capture group for scope
		return "", false
	}

	return normalizeScope(markerScope(matches[1])), true
}

func normalizeScope(scope markerScope) markerScope {
	switch scope {
	case markerScopeYear:
		return markerScopeMajor
	case markerScopeMonth:
		return markerScopeMinor
	case markerScopeMicro:
		return markerScopePatch
	case markerScopeVersion, markerScopeMajor, markerScopeMinor, markerScopePatch:
		return scope
	default:
		return scope
	}
}

func splitVersion(version string) (string, string, string) {
	if idx := strings.IndexAny(version, "-+"); idx >= 0 {
		version = version[:idx]
	}

	parts := strings.Split(version, ".")
	if len(parts) < minimumVersionPartCount {
		return "", "", ""
	}

	return parts[0], parts[1], parts[len(parts)-1]
}

// replaceForScope returns the possibly-updated line along with whether the
// numeric pattern for this scope found a replacement target on the line.
// The bool lets callers distinguish "marker present but value missing" from
// "no numeric target on the line" — only the latter indicates user error.
func replaceForScope(line string, scope markerScope, nextVersion, major, minor, patch string) (string, bool) {
	switch scope {
	case markerScopeVersion:
		return replaceFirst(versionPattern, line, nextVersion)
	case markerScopeMajor, markerScopeYear:
		if major == "" {
			return line, true
		}

		return replaceFirst(majorPattern, line, major)
	case markerScopeMinor, markerScopeMonth:
		if minor == "" {
			return line, true
		}

		return replaceFirst(minorPatchPattern, line, minor)
	case markerScopePatch, markerScopeMicro:
		if patch == "" {
			return line, true
		}

		return replaceFirst(minorPatchPattern, line, patch)
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
