package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

// mergedCandidateStub stands in for one forge's pull request record. A zero
// mergedAt means the listing did not say when the candidate merged.
type mergedCandidateStub struct {
	number   int
	mergedAt time.Time
}

// mergedCandidateHydration records what a forge would answer on a re-read.
type mergedCandidateHydration struct {
	mergedAt time.Time
	merged   bool
	err      error
}

func newMergedCandidateSpec(
	answers map[int]mergedCandidateHydration,
	calls *[]int,
) mergedCandidates[mergedCandidateStub] {
	return mergedCandidates[mergedCandidateStub]{
		mergedAt: func(candidate mergedCandidateStub) (time.Time, bool) {
			return candidate.mergedAt, !candidate.mergedAt.IsZero()
		},
		hydrate: func(_ context.Context, candidate mergedCandidateStub) (mergedCandidateStub, bool, error) {
			*calls = append(*calls, candidate.number)

			answer := answers[candidate.number]
			if answer.err != nil {
				return candidate, false, answer.err
			}

			return mergedCandidateStub{number: candidate.number, mergedAt: answer.mergedAt}, answer.merged, nil
		},
		reference: func(candidate mergedCandidateStub) string {
			return fmt.Sprintf("pull request #%d", candidate.number)
		},
	}
}

func TestResolveLatestMerged(t *testing.T) {
	t.Parallel()

	early := time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC)
	late := time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)

	t.Run("reports no PR when nothing was listed", func(t *testing.T) {
		t.Parallel()

		// given: a forge that listed no merged candidates
		var calls []int

		spec := newMergedCandidateSpec(nil, &calls)

		// when: the latest merged candidate is resolved
		_, err := resolveLatestMerged(context.Background(), nil, spec)

		// then: the caller is told no release PR exists
		testastic.ErrorIs(t, err, forge.ErrNoPR)
	})

	t.Run("returns a lone candidate without re-reading it", func(t *testing.T) {
		t.Parallel()

		// given: one candidate whose listing carried no merge time
		var calls []int

		spec := newMergedCandidateSpec(nil, &calls)
		candidates := []mergedCandidateStub{{number: 7}}

		// when: the latest merged candidate is resolved
		best, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: nothing competes with it, so the forge is not asked again
		testastic.NoError(t, err)
		testastic.Equal(t, 7, best.number)
		testastic.Equal(t, 0, len(calls))
	})

	t.Run("picks the candidate that merged last", func(t *testing.T) {
		t.Parallel()

		// given: two candidates the listing already dated
		var calls []int

		spec := newMergedCandidateSpec(nil, &calls)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: early}, {number: 9, mergedAt: late}}

		// when: the latest merged candidate is resolved
		best, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the later merge wins and no re-read was needed
		testastic.NoError(t, err)
		testastic.Equal(t, 9, best.number)
		testastic.Equal(t, 0, len(calls))
	})

	t.Run("keeps the first of two candidates that merged at the same time", func(t *testing.T) {
		t.Parallel()

		// given: two candidates the listing dated identically
		var calls []int

		spec := newMergedCandidateSpec(nil, &calls)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: late}, {number: 9, mergedAt: late}}

		// when: the latest merged candidate is resolved
		best, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: listing order breaks the tie
		testastic.NoError(t, err)
		testastic.Equal(t, 7, best.number)
	})

	t.Run("re-reads only the candidate the listing left undated", func(t *testing.T) {
		t.Parallel()

		// given: two competing candidates, one of which carries no merge time
		var calls []int

		spec := newMergedCandidateSpec(
			map[int]mergedCandidateHydration{9: {mergedAt: late, merged: true}},
			&calls,
		)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: early}, {number: 9}}

		// when: the latest merged candidate is resolved
		best, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the undated candidate is re-read and wins on its recovered time
		testastic.NoError(t, err)
		testastic.Equal(t, 9, best.number)
		testastic.DeepEqual(t, []int{9}, calls)
	})

	t.Run("drops a candidate the re-read proves never merged", func(t *testing.T) {
		t.Parallel()

		// given: an undated candidate that turns out to be closed rather than merged
		var calls []int

		spec := newMergedCandidateSpec(
			map[int]mergedCandidateHydration{9: {merged: false}},
			&calls,
		)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: early}, {number: 9}}

		// when: the latest merged candidate is resolved
		best, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the merged candidate stands alone and wins
		testastic.NoError(t, err)
		testastic.Equal(t, 7, best.number)
	})

	t.Run("reports no PR when every candidate is dropped", func(t *testing.T) {
		t.Parallel()

		// given: two undated candidates that both turn out to be unmerged
		var calls []int

		spec := newMergedCandidateSpec(
			map[int]mergedCandidateHydration{7: {merged: false}, 9: {merged: false}},
			&calls,
		)
		candidates := []mergedCandidateStub{{number: 7}, {number: 9}}

		// when: the latest merged candidate is resolved
		_, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the caller is told no release PR exists
		testastic.ErrorIs(t, err, forge.ErrNoPR)
	})

	t.Run("refuses to guess when a competing merge time stays unknown", func(t *testing.T) {
		t.Parallel()

		// given: a competing candidate the forge still cannot date after a re-read
		var calls []int

		spec := newMergedCandidateSpec(
			map[int]mergedCandidateHydration{9: {merged: true}},
			&calls,
		)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: early}, {number: 9}}

		// when: the latest merged candidate is resolved
		_, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the ambiguity is reported against the candidate that caused it
		testastic.ErrorIs(t, err, errMergeTimeMissing)
		testastic.ErrorContains(t, err, "pull request #9")
	})

	t.Run("propagates a failed re-read", func(t *testing.T) {
		t.Parallel()

		// given: a forge that fails while re-reading an undated candidate
		var calls []int

		errUnavailable := errors.New("forge unavailable")

		spec := newMergedCandidateSpec(
			map[int]mergedCandidateHydration{9: {err: errUnavailable}},
			&calls,
		)
		candidates := []mergedCandidateStub{{number: 7, mergedAt: early}, {number: 9}}

		// when: the latest merged candidate is resolved
		_, err := resolveLatestMerged(context.Background(), candidates, spec)

		// then: the forge failure reaches the caller unchanged
		testastic.ErrorIs(t, err, errUnavailable)
	})
}
