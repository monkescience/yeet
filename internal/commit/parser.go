package commit

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

type Commit struct {
	Hash        string
	Type        string
	Scope       string
	Description string
	Body        string
	Footers     []Footer
	Breaking    bool
}

type Footer struct {
	Key   string
	Value string
}

type BumpType string

const (
	BumpNone  BumpType = "none"
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"

	bumpRankNone  = 0
	bumpRankPatch = 1
	bumpRankMinor = 2
	bumpRankMajor = 3
)

const (
	FooterBreakingChange  = "BREAKING CHANGE"
	FooterBreakingChanged = "BREAKING-CHANGE"
)

// BumpMapping defines per-type bump levels.
// Types not present produce BumpNone. Breaking commits always produce BumpMajor regardless of mapping.
type BumpMapping map[string]BumpType

// Format: type(scope)!: description.
var conventionalCommitPattern = regexp.MustCompile(
	`^(?P<type>\p{L}[\p{L}\p{M}]*)` +
		`(?:\((?P<scope>[^()\r\n]+)\))?` +
		`(?P<breaking>!)?` +
		`: (?P<description>\S.*)$`,
)

// Parse returns a Commit with an empty Type when the message is not conventional.
func Parse(ctx context.Context, hash, rawMessage string) Commit {
	c := Commit{
		Hash: hash,
	}

	lines := strings.Split(rawMessage, "\n")
	header := strings.TrimRightFunc(lines[0], unicode.IsSpace)
	matches := conventionalCommitPattern.FindStringSubmatch(header)
	scopeIndex := conventionalCommitPattern.SubexpIndex("scope")

	if matches == nil ||
		(matches[scopeIndex] != "" && strings.TrimSpace(matches[scopeIndex]) == "") ||
		(len(lines) > 1 && strings.TrimSpace(lines[1]) != "") {
		c.Description = strings.TrimSpace(header)

		slog.DebugContext(ctx, "commit: invalid message structure, treating as no-bump",
			slog.String("hash", hash),
		)

		return c
	}

	c.Type = strings.ToLower(matches[conventionalCommitPattern.SubexpIndex("type")])
	c.Scope = matches[scopeIndex]
	c.Description = matches[conventionalCommitPattern.SubexpIndex("description")]
	c.Breaking = matches[conventionalCommitPattern.SubexpIndex("breaking")] == "!"

	parseBodyAndFooters(&c, lines[1:])
	logRejectedBreakingMarkers(ctx, &c, lines[1:])

	return c
}

func parseBodyAndFooters(c *Commit, lines []string) {
	footerStart := -1

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(lines[i-1]) != "" {
			continue
		}

		_, isFooter := parseFooter(strings.TrimLeftFunc(line, unicode.IsSpace))
		if !isFooter {
			continue
		}

		footerStart = i

		break
	}

	if footerStart == -1 {
		c.Body = strings.TrimSpace(strings.Join(lines, "\n"))

		return
	}

	c.Body = strings.TrimSpace(strings.Join(lines[:footerStart], "\n"))

	parseFooters(c, lines[footerStart:])
}

func parseFooters(c *Commit, lines []string) {
	var continuation strings.Builder

	for _, line := range lines {
		if footer, ok := parseFooter(strings.TrimLeftFunc(line, unicode.IsSpace)); ok {
			flushContinuation(c.Footers, &continuation)
			c.Footers = append(c.Footers, footer)

			continue
		}

		if len(c.Footers) == 0 {
			continue
		}

		appendContinuation(&continuation, line)
	}

	flushContinuation(c.Footers, &continuation)

	c.Footers = slices.DeleteFunc(c.Footers, func(footer Footer) bool {
		return isBreakingFooter(footer.Key) && strings.TrimSpace(footer.Value) == ""
	})

	for _, footer := range c.Footers {
		setBreaking(c, footer)
	}
}

func logRejectedBreakingMarkers(ctx context.Context, c *Commit, lines []string) {
	if c.Breaking {
		return
	}

	for _, line := range lines {
		marker, found := breakingMarkerToken(line)
		if !found {
			continue
		}

		footer, ok := parseFooter(strings.TrimLeftFunc(line, unicode.IsSpace))
		if ok && isBreakingFooter(footer.Key) && strings.TrimSpace(footer.Value) != "" {
			continue
		}

		slog.DebugContext(ctx, "commit: breaking marker rejected, treating as non-breaking",
			slog.String("hash", c.Hash),
			slog.String("marker", marker),
		)
	}
}

func breakingMarkerToken(line string) (string, bool) {
	marker := strings.TrimSpace(line)
	if before, _, found := strings.Cut(marker, ":"); found {
		marker = before
	} else if before, _, found := strings.Cut(marker, " #"); found {
		marker = before
	}

	if strings.EqualFold(marker, FooterBreakingChange) || strings.EqualFold(marker, FooterBreakingChanged) {
		return marker, true
	}

	return "", false
}

func appendContinuation(continuation *strings.Builder, line string) {
	continuation.WriteString("\n")
	continuation.WriteString(line)
}

func isBreakingFooter(key string) bool {
	return key == FooterBreakingChange || key == FooterBreakingChanged
}

func setBreaking(c *Commit, footer Footer) {
	if isBreakingFooter(footer.Key) {
		c.Breaking = true
	}
}

func flushContinuation(footers []Footer, continuation *strings.Builder) {
	if continuation.Len() == 0 {
		return
	}

	footers[len(footers)-1].Value += continuation.String()

	continuation.Reset()
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
	return unicode.IsLetter(ch) || unicode.IsMark(ch) || unicode.IsNumber(ch) || ch == '_'
}

func parseFooter(line string) (Footer, bool) {
	line = strings.TrimSuffix(line, "\r")

	if after, found := strings.CutPrefix(line, FooterBreakingChange+": "); found {
		return Footer{Key: FooterBreakingChange, Value: after}, true
	}

	if after, found := strings.CutPrefix(line, FooterBreakingChanged+": "); found {
		return Footer{Key: FooterBreakingChanged, Value: after}, true
	}

	if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 && isToken(parts[0]) { //nolint:mnd // footer format
		return Footer{Key: parts[0], Value: parts[1]}, true
	}

	if parts := strings.SplitN(line, " #", 2); len(parts) == 2 && isToken(parts[0]) { //nolint:mnd // footer format
		if isBreakingFooter(parts[0]) {
			return Footer{}, false
		}

		return Footer{Key: parts[0], Value: "#" + parts[1]}, true
	}

	return Footer{}, false
}

func DetermineBump(commits []Commit, mapping BumpMapping) BumpType {
	bump := BumpNone

	for _, c := range commits {
		b := commitBump(c, mapping)

		if CompareBump(b, bump) > 0 {
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

// CompareBump orders bump types by severity: none < patch < minor < major.
func CompareBump(a, b BumpType) int {
	return bumpOrder(a) - bumpOrder(b)
}

func bumpOrder(b BumpType) int {
	switch b {
	case BumpMajor:
		return bumpRankMajor
	case BumpMinor:
		return bumpRankMinor
	case BumpPatch:
		return bumpRankPatch
	case BumpNone:
		return bumpRankNone
	default:
		return bumpRankNone
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
