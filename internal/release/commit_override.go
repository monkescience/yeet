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

var ErrInvalidCommitOverride = errors.New("invalid commit override")

type commitOverrideResult struct {
	commits []commit.Commit
	found   bool
}

func commitOverrideMessages(
	ctx context.Context,
	body string,
	knownTypes map[string]struct{},
) ([]string, bool, error) {
	start := strings.Index(body, commitOverrideStartMarker)
	if start == -1 {
		return nil, false, nil
	}

	start += len(commitOverrideStartMarker)

	end := strings.Index(body[start:], commitOverrideEndMarker)
	if end == -1 {
		return nil, true, fmt.Errorf("%w: missing %s marker", ErrInvalidCommitOverride, commitOverrideEndMarker)
	}

	block := strings.TrimSpace(body[start : start+end])
	if block == "" {
		return nil, true, fmt.Errorf("%w: empty override block", ErrInvalidCommitOverride)
	}

	messages := splitCommitOverrideMessages(ctx, block, knownTypes)
	if len(messages) == 0 {
		return nil, true, fmt.Errorf("%w: empty override block", ErrInvalidCommitOverride)
	}

	return messages, true, nil
}

func splitCommitOverrideMessages(ctx context.Context, block string, knownTypes map[string]struct{}) []string {
	lines := strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	messages := make([]string, 0)
	current := make([]string, 0)

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if len(current) > 0 && isConventionalCommitHeader(ctx, trimmedLine, knownTypes) && previousLineBlank(current) {
			messages = appendCommitOverrideMessage(messages, current)
			current = current[:0]
		}

		current = append(current, line)
	}

	messages = appendCommitOverrideMessage(messages, current)

	return messages
}

// knownCommitTypes is the vocabulary the override splitter uses to tell a real
// conventional header from a footer trailer like "Closes: #45" (which would
// otherwise be split into a spurious commit).
func knownCommitTypes(cfg *config.Config) map[string]struct{} {
	types := make(map[string]struct{})

	for bumpType := range cfg.BumpTypes.ToBumpMapping() {
		types[bumpType] = struct{}{}
	}

	for sectionType := range cfg.Changelog.Sections {
		types[sectionType] = struct{}{}
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
