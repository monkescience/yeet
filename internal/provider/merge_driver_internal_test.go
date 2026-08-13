package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

type fakeForgeMerge struct {
	states       []mergeState
	executeSHA   string
	stateCalls   int
	methodCalls  int
	executeCalls int
	executeEnded bool
}

func (f *fakeForgeMerge) state(context.Context) (mergeState, error) {
	current := f.states[min(f.stateCalls, len(f.states)-1)]
	f.stateCalls++

	return current, nil
}

func (f *fakeForgeMerge) resolveMethod(context.Context, forge.MergeMethod) (any, error) {
	f.methodCalls++

	return forge.MergeMethodSquash, nil
}

func (f *fakeForgeMerge) execute(context.Context, mergeState, any) (string, bool, error) {
	f.executeCalls++

	return f.executeSHA, !f.executeEnded, nil
}

func mergeableState() mergeState {
	return mergeState{
		Reference:      "pull request #42",
		SourceBranch:   "yeet/release-main",
		BaseBranch:     "main",
		IsOpen:         true,
		SameRepository: true,
	}
}

func newTestMergeDriver(forge forgeMerge) mergeDriver {
	return mergeDriver{
		forge:   forge,
		polling: newMergePolling(WithMergePolling(time.Millisecond, time.Millisecond, time.Second)),
	}
}

func TestMergeDriverRefusesBeforeMutating(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		current  func() mergeState
		bypass   bool
		expected forge.MergeBlockedReason
	}{
		{
			name: "draft",
			current: func() mergeState {
				current := mergeableState()
				current.IsDraft = true

				return current
			},
			expected: forge.MergeBlockedReasonDraft,
		},
		{
			name: "conflicted while bypassing merge checks",
			current: func() mergeState {
				current := mergeableState()
				current.HasConflicts = true
				current.ReadinessBlocked = true

				return current
			},
			bypass:   true,
			expected: forge.MergeBlockedReasonConflicts,
		},
		{
			name: "closed and unmerged",
			current: func() mergeState {
				current := mergeableState()
				current.IsOpen = false
				current.IsClosedUnmerged = true

				return current
			},
			expected: forge.MergeBlockedReasonClosed,
		},
		{
			name: "readiness blocked",
			current: func() mergeState {
				current := mergeableState()
				current.ReadinessBlocked = true
				current.RawReadiness = "mergeable_state=blocked"

				return current
			},
			expected: forge.MergeBlockedReasonPolicy,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a forge reporting a release PR the merge policy must refuse
			adapter := &fakeForgeMerge{states: []mergeState{testCase.current()}}

			// when: the merge driver runs
			mergeSHA, err := newTestMergeDriver(adapter).run(
				context.Background(),
				forge.MergeReleasePROptions{BypassMergeChecks: testCase.bypass},
			)

			// then: the reason is named and no merge is attempted
			var blocked *forge.MergeBlockedError
			testastic.True(t, errors.As(err, &blocked))
			testastic.Equal(t, string(testCase.expected), string(blocked.Reason))
			testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
			testastic.Equal(t, "", mergeSHA)
			testastic.Equal(t, 0, adapter.methodCalls)
			testastic.Equal(t, 0, adapter.executeCalls)
		})
	}
}

func TestMergeDriverRefusesAnUntrustedRequestBeforeMutating(t *testing.T) {
	t.Parallel()

	// given: a mergeable release PR that does not come from the configured repository
	current := mergeableState()
	current.SameRepository = false

	adapter := &fakeForgeMerge{states: []mergeState{current}}

	// when: the merge driver runs
	mergeSHA, err := newTestMergeDriver(adapter).run(context.Background(), forge.MergeReleasePROptions{})

	// then: the trust check refuses before the merge method is even resolved
	testastic.ErrorIs(t, err, forge.ErrUntrustedReleasePR)
	testastic.Equal(t, "", mergeSHA)
	testastic.Equal(t, 0, adapter.methodCalls)
	testastic.Equal(t, 0, adapter.executeCalls)
}

func TestMergeDriverAnswersAnAlreadyMergedRequestWithoutMerging(t *testing.T) {
	t.Parallel()

	// given: a release PR the forge already merged
	current := mergeableState()
	current.IsOpen = false
	current.IsMerged = true
	current.MergeCommitSHA = "6d65726765736861000000000000000000000000"

	adapter := &fakeForgeMerge{states: []mergeState{current}}

	// when: the merge driver runs
	mergeSHA, err := newTestMergeDriver(adapter).run(context.Background(), forge.MergeReleasePROptions{})

	// then: the existing merge commit is returned without a second merge
	testastic.NoError(t, err)
	testastic.Equal(t, "6d65726765736861000000000000000000000000", mergeSHA)
	testastic.Equal(t, 1, adapter.stateCalls)
	testastic.Equal(t, 0, adapter.executeCalls)
}

func TestMergeDriverPollsOnlyWhenTheMergeIsStillPending(t *testing.T) {
	t.Parallel()

	t.Run("a finalized merge is not polled", func(t *testing.T) {
		t.Parallel()

		// given: a forge that applies the merge on the completion response
		adapter := &fakeForgeMerge{
			states:       []mergeState{mergeableState()},
			executeSHA:   "6d65726765736861000000000000000000000000",
			executeEnded: true,
		}

		// when: the merge driver runs
		mergeSHA, err := newTestMergeDriver(adapter).run(context.Background(), forge.MergeReleasePROptions{})

		// then: the reported commit is returned and the forge is not read again
		testastic.NoError(t, err)
		testastic.Equal(t, "6d65726765736861000000000000000000000000", mergeSHA)
		testastic.Equal(t, 1, adapter.stateCalls)
	})

	t.Run("a pending merge is polled until it lands", func(t *testing.T) {
		t.Parallel()

		// given: a forge that accepts the merge and applies it afterwards
		merged := mergeableState()
		merged.IsOpen = false
		merged.IsMerged = true
		merged.MergeCommitSHA = "6d65726765736861000000000000000000000000"

		adapter := &fakeForgeMerge{states: []mergeState{mergeableState(), merged}}

		// when: the merge driver runs
		mergeSHA, err := newTestMergeDriver(adapter).run(context.Background(), forge.MergeReleasePROptions{})

		// then: the commit comes from the poll rather than the completion response
		testastic.NoError(t, err)
		testastic.Equal(t, "6d65726765736861000000000000000000000000", mergeSHA)
		testastic.Equal(t, 2, adapter.stateCalls)
		testastic.Equal(t, 1, adapter.executeCalls)
	})

	t.Run("a terminal refusal stops polling", func(t *testing.T) {
		t.Parallel()

		// given: a forge that accepts the merge and later reports a terminal failure
		refused := mergeableState()
		refused.Refusal = &mergeRefusal{
			reason: forge.MergeBlockedReasonFailure,
			detail: "was refused: provider failure",
		}

		adapter := &fakeForgeMerge{states: []mergeState{mergeableState(), refused}}

		// when: the merge driver polls the accepted merge
		mergeSHA, err := newTestMergeDriver(adapter).run(context.Background(), forge.MergeReleasePROptions{})

		// then: the normalized refusal is returned after one read
		var blocked *forge.MergeBlockedError
		if !errors.As(err, &blocked) {
			t.Fatalf("expected forge.MergeBlockedError, got %v", err)
		}

		testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
		testastic.Equal(t, string(forge.MergeBlockedReasonFailure), string(blocked.Reason))
		testastic.Equal(t, "was refused: provider failure", blocked.Detail)
		testastic.Equal(t, "", mergeSHA)
		testastic.Equal(t, 2, adapter.stateCalls)
		testastic.Equal(t, 1, adapter.executeCalls)
	})
}
