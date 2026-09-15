package release

import (
	"errors"
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

func TestClassifyIndependent(t *testing.T) {
	t.Parallel()

	// given: mixed-case independent units that fail in different lifecycle phases
	policyFailure := &forge.MergeBlockedError{Reason: forge.MergeBlockedReasonPolicy}
	draftFailure := &forge.MergeBlockedError{Reason: forge.MergeBlockedReasonDraft}
	joined := errors.Join(
		&unitError{unit: "group:BackendTeam", phase: "reconciliation", cause: policyFailure},
		&unitError{unit: "target:Web", phase: "finalization", cause: draftFailure},
	)

	// when: classifying the joined release failure
	failure := classifyFailure("release.yaml", joined)

	// then: aggregate telemetry retains the first classified merge-blocked result
	testastic.Equal(t, FailureMergeBlocked, failure.Kind())
	testastic.Equal(t, MergeReasonPolicy, failure.MergeReason())
	testastic.ErrorIs(t, failure, forge.ErrMergeBlocked)

	var aggregateBlocked *forge.MergeBlockedError

	testastic.True(t, errors.As(failure, &aggregateBlocked))
	testastic.Equal(t, forge.MergeBlockedReasonPolicy, aggregateBlocked.Reason)

	// then: each constituent retains its own kind, reason, name, and phase
	failures := failure.Failures()
	testastic.Len(t, failures, 2)
	testastic.Equal(t, "group:BackendTeam", failures[0].Unit())
	testastic.Equal(t, "reconciliation", failures[0].Phase())
	testastic.Equal(t, FailureMergeBlocked, failures[0].Kind())
	testastic.Equal(t, MergeReasonPolicy, failures[0].MergeReason())
	testastic.ErrorIs(t, failures[0], policyFailure)
	testastic.Equal(t, "target:Web", failures[1].Unit())
	testastic.Equal(t, "finalization", failures[1].Phase())
	testastic.Equal(t, FailureMergeBlocked, failures[1].Kind())
	testastic.Equal(t, MergeReasonDraft, failures[1].MergeReason())
	testastic.ErrorIs(t, failures[1], draftFailure)
}

func TestClassifyIndependentRetainsUnscopedFailure(t *testing.T) {
	t.Parallel()

	// given: a malformed merged pull request and a failed unit finalization
	manifestFailure := fmt.Errorf("%w: missing release unit", errInvalidReleaseManifest)
	finalizationFailure := &forge.MergeBlockedError{Reason: forge.MergeBlockedReasonPolicy}
	joined := fmt.Errorf("independent finalization: %w", errors.Join(
		manifestFailure,
		&unitError{unit: "target:Web", phase: "finalization", cause: finalizationFailure},
	))

	// when: classifying the independent aggregate
	failure := classifyFailure("release.yaml", joined)

	// then: aggregate classification and inspection preserve both causes
	testastic.Equal(t, FailureMergeBlocked, failure.Kind())
	testastic.Equal(t, MergeReasonPolicy, failure.MergeReason())
	testastic.ErrorIs(t, failure, manifestFailure)
	testastic.ErrorIs(t, failure, finalizationFailure)

	// then: the unscoped manifest failure and scoped finalization are both reportable
	failures := failure.Failures()
	testastic.Len(t, failures, 2)
	testastic.Equal(t, FailureUnexpected, failures[0].Kind())
	testastic.Equal(t, "", failures[0].Unit())
	testastic.Equal(t, "", failures[0].Phase())
	testastic.ErrorIs(t, failures[0], manifestFailure)
	testastic.Equal(t, FailureMergeBlocked, failures[1].Kind())
	testastic.Equal(t, "target:Web", failures[1].Unit())
	testastic.Equal(t, "finalization", failures[1].Phase())
	testastic.Equal(t, MergeReasonPolicy, failures[1].MergeReason())
	testastic.ErrorIs(t, failures[1], finalizationFailure)
}

func TestClassifyIndependentPreservesSemanticWrapper(t *testing.T) {
	t.Parallel()

	// given: a checkout diagnostic retaining multiple implementation causes
	checkout := &history.CheckoutError{
		Problem: "checkout is unavailable",
		Err:     errors.Join(errors.New("local state failed"), errors.New("remote state failed")),
	}

	// when: classifying the checkout failure
	failure := classifyFailure("release.yaml", checkout)

	// then: one checkout constituent retains the typed diagnostic wrapper
	failures := failure.Failures()
	testastic.Len(t, failures, 1)
	testastic.Equal(t, FailureCheckout, failures[0].Kind())
	testastic.ErrorIs(t, failures[0], history.ErrCheckoutUnusable)

	var classified *history.CheckoutError

	testastic.True(t, errors.As(failures[0], &classified))
	testastic.Equal(t, "checkout is unavailable", classified.Problem)
}
