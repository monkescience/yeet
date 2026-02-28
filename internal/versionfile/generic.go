// Package versionfile updates version references in configured files.
package versionfile

import (
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

	versionPartCount = 3
)

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[\w.]+)?(?:\+[-\w.]+)?`)

var majorPattern = regexp.MustCompile(`\d+\b`)

var minorPatchPattern = regexp.MustCompile(`\b\d+\b`)

var inlineMarkerPattern = regexp.MustCompile(`x-yeet-(major|minor|patch|version|year|month|micro)\b`)

var blockStartPattern = regexp.MustCompile(`x-yeet-start-(major|minor|patch|version|year|month|micro)\b`)

var blockEndPattern = regexp.MustCompile(`x-yeet-end\b`)

// ApplyGenericMarkers applies yeet marker-based version replacements to file content.
func ApplyGenericMarkers(content, nextVersion string) (string, bool) {
	if content == "" {
		return content, false
	}

	major, minor, patch := splitVersion(nextVersion)
	lines := strings.Split(content, "\n")

	updated := make([]string, 0, len(lines))

	var blockScope markerScope

	for _, line := range lines {
		scope, isInline := markerScopeFromLine(line, inlineMarkerPattern)
		if isInline {
			updated = append(updated, replaceForScope(line, scope, nextVersion, major, minor, patch))

			continue
		}

		if blockScope != "" {
			updated = append(updated, replaceForScope(line, blockScope, nextVersion, major, minor, patch))

			if blockEndPattern.MatchString(line) {
				blockScope = ""
			}

			continue
		}

		scope, isBlockStart := markerScopeFromLine(line, blockStartPattern)
		if isBlockStart {
			blockScope = scope
		}

		updated = append(updated, line)
	}

	result := strings.Join(updated, "\n")

	return result, result != content
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
	parts := strings.SplitN(version, ".", versionPartCount)
	if len(parts) < versionPartCount {
		return "", "", ""
	}

	patch := parts[2]
	if idx := strings.IndexAny(patch, "-+"); idx >= 0 {
		patch = patch[:idx]
	}

	return parts[0], parts[1], patch
}

func replaceForScope(line string, scope markerScope, nextVersion, major, minor, patch string) string {
	switch scope {
	case markerScopeVersion:
		return replaceFirst(versionPattern, line, nextVersion)
	case markerScopeMajor:
		if major == "" {
			return line
		}

		return replaceFirst(majorPattern, line, major)
	case markerScopeMinor:
		if minor == "" {
			return line
		}

		return replaceFirst(minorPatchPattern, line, minor)
	case markerScopePatch:
		if patch == "" {
			return line
		}

		return replaceFirst(minorPatchPattern, line, patch)
	case markerScopeYear:
		if major == "" {
			return line
		}

		return replaceFirst(majorPattern, line, major)
	case markerScopeMonth:
		if minor == "" {
			return line
		}

		return replaceFirst(minorPatchPattern, line, minor)
	case markerScopeMicro:
		if patch == "" {
			return line
		}

		return replaceFirst(minorPatchPattern, line, patch)
	default:
		return line
	}
}

func replaceFirst(pattern *regexp.Regexp, line, replacement string) string {
	match := pattern.FindStringIndex(line)
	if match == nil {
		return line
	}

	return line[:match[0]] + replacement + line[match[1]:]
}
