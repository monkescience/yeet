package history_test

import (
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/history"
)

func TestCheckoutErrorPreservesClassificationAndFacts(t *testing.T) {
	t.Parallel()

	// given: a stale checkout with a retained Git failure cause
	cause := errors.New("git state failed")
	err := &history.CheckoutError{
		Problem:       "checkout does not match remote branch",
		Branch:        "main",
		CurrentBranch: "feature",
		Ref:           "v1.2.3",
		LocalHead:     "local-sha",
		RemoteHead:    "remote-sha",
		Err:           cause,
	}

	// when: the checkout failure is presented or inspected
	var checkout *history.CheckoutError

	found := errors.As(err, &checkout)

	// then: its sentinel, cause, and diagnostic facts remain available
	testastic.Equal(t, "local checkout cannot serve release history: checkout does not match remote branch", err.Error())
	testastic.ErrorIs(t, err, history.ErrCheckoutUnusable)
	testastic.ErrorIs(t, err, cause)
	testastic.True(t, found)
	testastic.Equal(t, "main", checkout.Branch)
	testastic.Equal(t, "feature", checkout.CurrentBranch)
	testastic.Equal(t, "v1.2.3", checkout.Ref)
	testastic.Equal(t, "local-sha", checkout.LocalHead)
	testastic.Equal(t, "remote-sha", checkout.RemoteHead)
}
