package history

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

type RemoteTagMetadataError struct {
	Tag     string
	Problem string
	Err     error
}

func (e *RemoteTagMetadataError) Error() string { return e.Err.Error() }

func (e *RemoteTagMetadataError) Unwrap() error { return e.Err }

func remoteTagMetadataError(tag, problem, detail string) error {
	return &RemoteTagMetadataError{
		Tag:     tag,
		Problem: problem,
		Err:     fmt.Errorf("%w: tag %q %s", errRemoteTagMetadata, tag, detail),
	}
}

var gitConfigParseFailure = regexp.MustCompile(
	`(?:^|: )read (?:worktree )?config: ([0-9]+):([0-9]+): (` +
		`expected section name|expected right bracket|expected EOL, EOF, or comment|` +
		`expected section header|expected '='|expected value|expected section header or variable declaration)$`,
)

const gitConfigParseMatches = 4

func openCheckoutError(branch string, err error) error {
	checkout := &CheckoutError{Problem: CheckoutProblemNoRepository, Branch: branch, Err: err}

	if errors.Is(err, git.ErrRepositoryNotExists) {
		checkout.Cause = CheckoutCauseNoRepository

		return checkout
	}

	matches := gitConfigParseFailure.FindStringSubmatch(err.Error())
	if len(matches) != gitConfigParseMatches {
		return checkout
	}

	line, lineErr := strconv.Atoi(matches[1])

	column, columnErr := strconv.Atoi(matches[2])
	if lineErr != nil || columnErr != nil {
		return checkout
	}

	checkout.Cause = CheckoutCauseInvalidConfig
	checkout.Reason = matches[3]
	checkout.Line = line
	checkout.Column = column

	return checkout
}

func headCheckoutError(err error) error {
	checkout := &CheckoutError{Problem: CheckoutProblemNoHead, Err: err}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		checkout.Cause = CheckoutCauseNoReference
	}

	return checkout
}
