package release //nolint:testpackage // validates private classification against package-owned sentinels

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
)

func TestClassifyFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		cause  error
		kind   FailureKind
		reason MergeReason
	}{
		{name: "missing config", cause: os.ErrNotExist, kind: FailureConfigMissing},
		{name: "invalid config", cause: config.ErrInvalidConfig, kind: FailureConfigInvalid},
		{name: "authentication", cause: provider.ErrMissingToken, kind: FailureAuthentication},
		{name: "repository", cause: provider.ErrUnknownRemote, kind: FailureRepository},
		{name: "host trust", cause: provider.ErrUntrustedHost, kind: FailureHostTrust},
		{name: "checkout", cause: history.ErrCheckoutUnusable, kind: FailureCheckout},
		{name: "release branch", cause: errUnconfiguredReleaseBranch, kind: FailureReleaseBranch},
		{name: "release state", cause: ErrMultiplePendingReleasePRs, kind: FailureReleaseState},
		{
			name:   "merge blocked",
			cause:  &forge.MergeBlockedError{Reason: forge.MergeBlockedReasonPolicy},
			kind:   FailureMergeBlocked,
			reason: MergeReasonPolicy,
		},
		{name: "merge timeout", cause: forge.ErrMergeNotFinalized, kind: FailureMergeTimeout},
		{name: "reviewer", cause: forge.ErrReviewerNotFound, kind: FailureReviewer},
		{name: "labels", cause: forge.ErrReleasePRLabelMissing, kind: FailureLabels},
		{name: "unexpected", cause: errors.New("surprise"), kind: FailureUnexpected},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a wrapped stable cause from the release workflow
			wrapped := fmt.Errorf("release operation: %w", testCase.cause)

			// when: classifying the failure
			failure := classifyFailure("custom.yaml", wrapped)

			// then: the operational kind, config path, and original cause are preserved
			testastic.Equal(t, testCase.kind, failure.Kind())
			testastic.Equal(t, "custom.yaml", failure.ConfigPath())
			testastic.Equal(t, testCase.reason, failure.MergeReason())
			testastic.ErrorIs(t, failure, testCase.cause)
		})
	}
}

func TestClassifyMergeReason(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cause    error
		expected MergeReason
	}{
		{name: "conflicts", cause: blockedFailure(forge.MergeBlockedReasonConflicts), expected: MergeReasonConflicts},
		{name: "draft", cause: blockedFailure(forge.MergeBlockedReasonDraft), expected: MergeReasonDraft},
		{name: "closed", cause: blockedFailure(forge.MergeBlockedReasonClosed), expected: MergeReasonClosed},
		{name: "policy", cause: blockedFailure(forge.MergeBlockedReasonPolicy), expected: MergeReasonPolicy},
		{name: "method refusal", cause: blockedFailure(forge.MergeBlockedReasonMethod), expected: MergeReasonMethod},
		{name: "provider refusal", cause: blockedFailure(forge.MergeBlockedReasonFailure), expected: MergeReasonProvider},
		{name: "unknown refusal", cause: blockedFailure(forge.MergeBlockedReasonUnknown), expected: MergeReasonUnknown},
		{name: "unsupported method", cause: forge.ErrMergeMethodUnsupported, expected: MergeReasonMethod},
		{name: "untrusted request", cause: forge.ErrUntrustedReleasePR, expected: MergeReasonProvider},
		{name: "untyped refusal", cause: forge.ErrMergeBlocked, expected: MergeReasonUnknown},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a stable forge merge failure

			// when: classifying it for release reporting
			actual := classifyMergeReason(testCase.cause)

			// then: the forge reason maps to the release-owned reason
			testastic.Equal(t, testCase.expected, actual)
		})
	}
}

func TestRunAlwaysReturnsFailure(t *testing.T) {
	// given: an explicit config path that does not exist
	configPath := t.TempDir() + "/missing.yaml"

	// when: running a release
	_, err := Run(t.Context(), configPath, Options{})

	// then: the error has the one exported release failure interface
	var failure *Failure

	testastic.True(t, errors.As(err, &failure))

	if failure == nil {
		return
	}

	testastic.Equal(t, FailureConfigMissing, failure.Kind())
	testastic.Equal(t, configPath, failure.ConfigPath())
	testastic.ErrorIs(t, failure, os.ErrNotExist)
}

func blockedFailure(reason forge.MergeBlockedReason) error {
	return &forge.MergeBlockedError{Reason: reason}
}
