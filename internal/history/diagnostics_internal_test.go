package history

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/monkescience/testastic"
)

func TestOpenCheckoutErrorClassifiesGitFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		err     error
		problem string
		cause   string
		reason  string
		line    int
		column  int
	}{
		{
			name:    "missing repository",
			err:     git.ErrRepositoryNotExists,
			problem: CheckoutProblemNoRepository,
			cause:   CheckoutCauseNoRepository,
		},
		{
			name:    "malformed config",
			err:     errors.New("read config: 1:6: expected right bracket"),
			problem: CheckoutProblemNoRepository,
			cause:   CheckoutCauseInvalidConfig,
			reason:  "expected right bracket",
			line:    1,
			column:  6,
		},
		{
			name:    "malformed worktree config",
			err:     fmt.Errorf("open: %w", errors.New("read worktree config: 12:3: expected value")),
			problem: CheckoutProblemNoRepository,
			cause:   CheckoutCauseInvalidConfig,
			reason:  "expected value",
			line:    12,
			column:  3,
		},
		{
			name:    "unrecognized failure",
			err:     errors.New("permission denied"),
			problem: CheckoutProblemNoRepository,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a failure returned while opening the local checkout
			// when: classifying it
			err := openCheckoutError("main", test.err)

			// then: the problem, cause and parse position are recorded without the raw go-git text
			var checkout *CheckoutError

			testastic.ErrorAs(t, err, &checkout)
			testastic.ErrorIs(t, err, ErrCheckoutUnusable)
			testastic.Equal(t, test.problem, checkout.Problem)
			testastic.Equal(t, test.cause, checkout.Cause)
			testastic.Equal(t, test.reason, checkout.Reason)
			testastic.Equal(t, test.line, checkout.Line)
			testastic.Equal(t, test.column, checkout.Column)
			testastic.Equal(t, "main", checkout.Branch)
		})
	}
}

func TestHeadCheckoutErrorClassifiesMissingReference(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		err   error
		cause string
	}{
		{name: "missing reference", err: plumbing.ErrReferenceNotFound, cause: CheckoutCauseNoReference},
		{name: "other failure", err: errors.New("storer closed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// given: a failure returned while reading the checkout head
			// when: classifying it
			err := headCheckoutError(test.err)

			// then: only a genuinely missing reference is named as such
			var checkout *CheckoutError

			testastic.ErrorAs(t, err, &checkout)
			testastic.Equal(t, CheckoutProblemNoHead, checkout.Problem)
			testastic.Equal(t, test.cause, checkout.Cause)
		})
	}
}
