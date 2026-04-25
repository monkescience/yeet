package release

import (
	"errors"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
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

func commitOverrideMessages(body string) ([]string, bool, error) {
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

	messages := splitCommitOverrideMessages(block)
	if len(messages) == 0 {
		return nil, true, fmt.Errorf("%w: empty override block", ErrInvalidCommitOverride)
	}

	return messages, true, nil
}

func splitCommitOverrideMessages(block string) []string {
	lines := strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	messages := make([]string, 0)
	current := make([]string, 0)

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if len(current) > 0 && isConventionalCommitHeader(trimmedLine) && previousLineBlank(current) {
			messages = appendCommitOverrideMessage(messages, current)
			current = current[:0]
		}

		current = append(current, line)
	}

	messages = appendCommitOverrideMessage(messages, current)

	return messages
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

func isConventionalCommitHeader(line string) bool {
	return commit.Parse("", line).IsConventional()
}
