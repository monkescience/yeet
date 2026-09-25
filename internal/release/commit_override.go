package release

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
)

const (
	commitOverrideStartMarker = "BEGIN_COMMIT_OVERRIDE"
	commitOverrideEndMarker   = "END_COMMIT_OVERRIDE"
)

var errInvalidCommitOverride = errors.New("invalid commit override")

func commitOverrideMessages(
	ctx context.Context,
	hash, body string,
	knownTypes map[string]struct{},
) ([]string, bool, error) {
	noteLines := commit.ReleaseNoteLines(body)

	start := markerIndex(body, commitOverrideStartMarker, 0, noteLines)
	if start == -1 {
		return nil, false, nil
	}

	start += len(commitOverrideStartMarker)

	end := markerIndex(body, commitOverrideEndMarker, start, commit.ClosedReleaseNoteLines(body))
	if end == -1 {
		return nil, true, &CommitOverrideError{
			Commit:        hash,
			Problem:       "override block has no end marker",
			MissingMarker: commitOverrideEndMarker,
			cause:         fmt.Errorf("%w: missing %s marker", errInvalidCommitOverride, commitOverrideEndMarker),
		}
	}

	block := strings.TrimSpace(body[start:end])
	if block == "" {
		return nil, true, &CommitOverrideError{
			Commit:  hash,
			Problem: "override block is empty",
			cause:   fmt.Errorf("%w: empty override block", errInvalidCommitOverride),
		}
	}

	messages := splitCommitOverrideMessages(ctx, block, knownTypes)
	if len(messages) == 0 {
		return nil, true, &CommitOverrideError{
			Commit:  hash,
			Problem: "override block is empty",
			cause:   fmt.Errorf("%w: empty override block", errInvalidCommitOverride),
		}
	}

	return messages, true, nil
}

func markerIndex(body, marker string, from int, noteLines []bool) int {
	for from <= len(body) {
		offset := strings.Index(body[from:], marker)
		if offset == -1 {
			return -1
		}

		position := from + offset

		line := strings.Count(body[:position], "\n")
		if line >= len(noteLines) || !noteLines[line] {
			return position
		}

		from = position + len(marker)
	}

	return -1
}

func splitCommitOverrideMessages(ctx context.Context, block string, knownTypes map[string]struct{}) []string {
	normalized := strings.ReplaceAll(block, "\r\n", "\n")
	fenced := commit.ReleaseNoteLines(normalized)
	messages := make([]string, 0)
	current := make([]string, 0)

	for idx, line := range strings.Split(normalized, "\n") {
		trimmedLine := strings.TrimSpace(line)
		insideNote := len(fenced) > 0 && fenced[idx]

		if len(current) > 0 && !insideNote && previousLineBlank(current) &&
			isConventionalCommitHeader(ctx, trimmedLine, knownTypes) {
			messages = appendCommitOverrideMessage(messages, current)
			current = current[:0]
		}

		current = append(current, line)
	}

	messages = appendCommitOverrideMessage(messages, current)

	return messages
}

func knownCommitTypes(cfg *config.Config) map[string]struct{} {
	types := make(map[string]struct{})

	for bumpType := range cfg.BumpTypes.ToBumpMapping() {
		types[bumpType] = struct{}{}
	}

	for sectionType := range cfg.Changelog.Sections {
		if sectionType != config.BreakingSectionKey {
			types[sectionType] = struct{}{}
		}
	}

	return types
}

func appendCommitOverrideMessage(messages []string, lines []string) []string {
	message := strings.TrimSpace(strings.Join(lines, "\n"))
	if message == "" {
		return messages
	}

	return append(messages, message)
}

func previousLineBlank(lines []string) bool {
	return strings.TrimSpace(lines[len(lines)-1]) == ""
}

func isConventionalCommitHeader(ctx context.Context, line string, knownTypes map[string]struct{}) bool {
	parsed := commit.Parse(ctx, "", line)
	if !parsed.IsConventional() {
		return false
	}

	_, known := knownTypes[parsed.Type]

	return known
}
