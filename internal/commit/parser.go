// Package commit provides conventional commit message parsing.
package commit

import (
	"regexp"
	"slices"
	"strings"
)

type Commit struct {
	Hash        string
	Type        string
	Scope       string
	Description string
	Body        string
	Footers     []Footer
	Breaking    bool
	Raw         string
}

// Footer represents a conventional commit footer (e.g., "BREAKING CHANGE: description").
type Footer struct {
	Key   string
	Value string
}

type BumpType = string

const (
	BumpNone  BumpType = "none"
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
)

// BumpMapping maps conventional commit types to their bump levels.
// Types not present in the map produce BumpNone.
// Breaking commits always produce BumpMajor regardless of mapping.
type BumpMapping map[string]BumpType

// DefaultBumpMapping returns the default mapping: feat→minor, fix/perf→patch.
func DefaultBumpMapping() BumpMapping {
	return BumpMapping{
		"feat": BumpMinor,
		"fix":  BumpPatch,
		"perf": BumpPatch,
	}
}

// conventionalCommitPattern matches a conventional commit header.
// Format: type(scope)!: description.
var conventionalCommitPattern = regexp.MustCompile(
	`^(?P<type>[a-zA-Z]+)` +
		`(?:\((?P<scope>[^)]*)\))?` +
		`(?P<breaking>!)?` +
		`:\s*(?P<description>.+)$`,
)

// Parse parses a raw commit message into a Commit.
// If the message does not follow the conventional commit format,
// it returns a Commit with an empty Type.
func Parse(hash, rawMessage string) Commit {
	c := Commit{
		Hash: hash,
		Raw:  rawMessage,
	}

	lines := strings.Split(rawMessage, "\n")
	if len(lines) == 0 {
		return c
	}

	header := strings.TrimSpace(lines[0])
	matches := conventionalCommitPattern.FindStringSubmatch(header)

	if matches == nil {
		c.Description = header

		return c
	}

	c.Type = strings.ToLower(matches[conventionalCommitPattern.SubexpIndex("type")])
	c.Scope = matches[conventionalCommitPattern.SubexpIndex("scope")]
	c.Description = matches[conventionalCommitPattern.SubexpIndex("description")]
	c.Breaking = matches[conventionalCommitPattern.SubexpIndex("breaking")] == "!"

	parseBody(&c, lines[1:])

	return c
}

func parseBody(c *Commit, lines []string) {
	footerStart := slices.IndexFunc(lines, func(line string) bool {
		return isFooter(strings.TrimSpace(line))
	})

	if footerStart == -1 {
		c.Body = strings.TrimSpace(strings.Join(lines, "\n"))

		return
	}

	c.Body = strings.TrimSpace(strings.Join(lines[:footerStart], "\n"))

	for _, line := range lines[footerStart:] {
		trimmed := strings.TrimSpace(line)

		if isFooter(trimmed) {
			footer := parseFooter(trimmed)
			c.Footers = append(c.Footers, footer)

			if footer.Key == "BREAKING CHANGE" || footer.Key == "BREAKING-CHANGE" {
				c.Breaking = true
			}

			continue
		}

		last := &c.Footers[len(c.Footers)-1]
		last.Value += "\n" + line
	}
}

func isFooter(line string) bool {
	if strings.HasPrefix(line, "BREAKING CHANGE:") || strings.HasPrefix(line, "BREAKING-CHANGE:") {
		return true
	}

	// Token: value format (token must be word characters or hyphens).
	parts := strings.SplitN(line, ": ", 2) //nolint:mnd // footer format is "key: value"
	if len(parts) == 2 && isToken(parts[0]) {
		return true
	}

	// Token #value format.
	parts = strings.SplitN(line, " #", 2) //nolint:mnd // footer format is "token #value"

	return len(parts) == 2 && isToken(parts[0])
}

func isToken(s string) bool {
	if s == "" {
		return false
	}

	for _, ch := range s {
		if ch != '-' && !isWordChar(ch) {
			return false
		}
	}

	return true
}

func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}

func parseFooter(line string) Footer {
	if after, found := strings.CutPrefix(line, "BREAKING CHANGE: "); found {
		return Footer{Key: "BREAKING CHANGE", Value: after}
	}

	if after, found := strings.CutPrefix(line, "BREAKING-CHANGE: "); found {
		return Footer{Key: "BREAKING-CHANGE", Value: after}
	}

	if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 && isToken(parts[0]) { //nolint:mnd // footer format
		return Footer{Key: parts[0], Value: parts[1]}
	}

	if parts := strings.SplitN(line, " #", 2); len(parts) == 2 && isToken(parts[0]) { //nolint:mnd // footer format
		return Footer{Key: parts[0], Value: "#" + parts[1]}
	}

	return Footer{Key: line}
}

func DetermineBump(commits []Commit, mapping BumpMapping) BumpType {
	bump := BumpNone

	for _, c := range commits {
		b := commitBump(c, mapping)

		if compareBump(b, bump) > 0 {
			bump = b
		}

		if bump == BumpMajor {
			return bump
		}
	}

	return bump
}

func commitBump(c Commit, mapping BumpMapping) BumpType {
	if c.Breaking {
		return BumpMajor
	}

	if bump, ok := mapping[c.Type]; ok {
		return bump
	}

	return BumpNone
}

func compareBump(a, b BumpType) int {
	return bumpOrder(a) - bumpOrder(b)
}

func bumpOrder(b BumpType) int {
	switch b {
	case BumpMajor:
		return 3 //nolint:mnd // ordering
	case BumpMinor:
		return 2 //nolint:mnd // ordering
	case BumpPatch:
		return 1
	default:
		return 0
	}
}

func (c Commit) IsConventional() bool {
	return c.Type != ""
}

func FilterByTypes(commits []Commit, types []string) []Commit {
	typeSet := make(map[string]struct{}, len(types))
	for _, t := range types {
		typeSet[t] = struct{}{}
	}

	var filtered []Commit

	for _, c := range commits {
		_, typeMatches := typeSet[c.Type]
		if typeMatches || c.Breaking {
			filtered = append(filtered, c)
		}
	}

	return filtered
}
