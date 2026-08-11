package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/forge"
)

func policyTestLabels() forge.ReleasePRLabels {
	return forge.ReleasePRLabels{
		Pending: "autorelease: pending",
		Tagged:  "autorelease: tagged",
		Yeet:    true,
		Extra:   []string{"release", "automated"},
	}
}

// applyLabelChange models what a forge does with a change, so a test can assert
// the label set a phase leaves behind rather than the calls it makes.
func applyLabelChange(current []string, change labelChange, match labelMatch) []string {
	result := make([]string, 0, len(current)+len(change.add)+1)

	for _, label := range current {
		if slices.ContainsFunc(change.remove, func(removed string) bool { return match(label, removed) }) {
			continue
		}

		result = append(result, label)
	}

	for _, label := range labelsAnchoredFirst(change.anchor, change.add) {
		if !slices.ContainsFunc(result, func(existing string) bool { return match(existing, label) }) {
			result = append(result, label)
		}
	}

	return result
}

func TestClassifyReleasePRLabels(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		found    []string
		match    labelMatch
		expected releasePRLabelState
	}{
		{
			name:     "pending label present",
			found:    []string{"autorelease: pending", "area/api"},
			match:    foldedLabelMatch,
			expected: releasePRLabelsPending,
		},
		{
			name:     "other labels present",
			found:    []string{"autorelease: tagged"},
			match:    foldedLabelMatch,
			expected: releasePRLabelsMismatched,
		},
		{
			name:     "no labels present",
			found:    nil,
			match:    foldedLabelMatch,
			expected: releasePRLabelsAdoptable,
		},
		{
			name:     "case variant folded",
			found:    []string{"Autorelease: Pending"},
			match:    foldedLabelMatch,
			expected: releasePRLabelsPending,
		},
		{
			name:     "case variant not folded",
			found:    []string{"Autorelease: Pending"},
			match:    exactLabelMatch,
			expected: releasePRLabelsMismatched,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: the labels a trusted release pull request carries
			// when: they are classified against the configured pending label
			state := classifyReleasePRLabels(testCase.found, "autorelease: pending", testCase.match)

			// then: the pull request lands in the expected lifecycle bucket
			testastic.Equal(t, testCase.expected, state)
		})
	}
}

func TestManagedLabelChangeAnchorsTheDiscoverableLabel(t *testing.T) {
	t.Parallel()

	// given: the configured managed label set
	labels := policyTestLabels()

	// when: the change for each phase is computed
	pending := managedLabelChange(labels, forge.ReleasePRPhasePending)
	tagged := managedLabelChange(labels, forge.ReleasePRPhaseTagged)

	// then: each phase anchors on the label that keeps the pull request findable
	testastic.Equal(t, labels.Pending, pending.anchor)
	testastic.SliceEqual(t, []string{"release", "automated", ReleaseLabelYeet}, pending.add)
	testastic.SliceEqual(t, []string{labels.Tagged}, pending.remove)

	testastic.Equal(t, labels.Tagged, tagged.anchor)
	testastic.Equal(t, 0, len(tagged.add))
	testastic.SliceEqual(t, []string{labels.Pending}, tagged.remove)

	// then: a forge sending one request still attaches the anchor first
	testastic.SliceEqual(
		t,
		[]string{labels.Pending, "release", "automated", ReleaseLabelYeet},
		labelsAnchoredFirst(pending.anchor, pending.add),
	)
	testastic.SliceEqual(t, []string{labels.Tagged}, labelsAnchoredFirst(tagged.anchor, tagged.add))
}

func TestLabelDefinitionCache(t *testing.T) {
	t.Parallel()

	t.Run("caches successful folded lookups", func(t *testing.T) {
		t.Parallel()

		// given: GitHub-style case-folded label definitions
		lookups := 0
		cache := &labelDefinitionCache{}
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return nil
			},
			cache:     cache,
			normalize: strings.ToLower,
		}

		// when: the same label is validated with different casing
		firstErr := definitions.validateExisting(context.Background(), "Release", "extra")
		secondErr := definitions.validateExisting(context.Background(), "release", "extra")

		// then: one successful lookup satisfies both requests
		testastic.NoError(t, firstErr)
		testastic.NoError(t, secondErr)
		testastic.Equal(t, 1, lookups)
	})

	t.Run("preserves exact-case lookup keys", func(t *testing.T) {
		t.Parallel()

		// given: GitLab-style exact-case label definitions
		lookups := 0
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return nil
			},
			cache: &labelDefinitionCache{},
		}

		// when: labels differing only by case are validated
		firstErr := definitions.validateExisting(context.Background(), "Release", "extra")
		secondErr := definitions.validateExisting(context.Background(), "release", "extra")

		// then: both exact definitions are looked up
		testastic.NoError(t, firstErr)
		testastic.NoError(t, secondErr)
		testastic.Equal(t, 2, lookups)
	})

	t.Run("does not cache failed or missing lookups", func(t *testing.T) {
		t.Parallel()

		// given: a label lookup that remains unavailable
		lookupErr := errors.New("lookup unavailable")
		lookups := 0
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return lookupErr
			},
			isNotFound: func(error) bool { return false },
			cache:      &labelDefinitionCache{},
		}

		// when: the failed lookup is attempted twice
		firstErr := definitions.validateExisting(context.Background(), "release", "extra")
		secondErr := definitions.validateExisting(context.Background(), "release", "extra")

		// then: both attempts reach the provider and preserve the failure
		testastic.ErrorIs(t, firstErr, lookupErr)
		testastic.ErrorIs(t, secondErr, lookupErr)
		testastic.Equal(t, 2, lookups)
	})

	t.Run("does not cache not-found validation", func(t *testing.T) {
		t.Parallel()

		// given: a label definition that remains absent
		notFoundErr := errors.New("not found")
		lookups := 0
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return notFoundErr
			},
			isNotFound: func(err error) bool { return errors.Is(err, notFoundErr) },
			cache:      &labelDefinitionCache{},
		}

		// when: the missing definition is validated twice
		firstErr := definitions.validateExisting(context.Background(), "release", "extra")
		secondErr := definitions.validateExisting(context.Background(), "release", "extra")

		// then: neither not-found response suppresses the next lookup
		testastic.ErrorIs(t, firstErr, forge.ErrReleasePRLabelMissing)
		testastic.ErrorIs(t, secondErr, forge.ErrReleasePRLabelMissing)
		testastic.Equal(t, 2, lookups)
	})

	t.Run("caches successful creation", func(t *testing.T) {
		t.Parallel()

		// given: a missing definition that can be created
		notFoundErr := errors.New("not found")
		lookups := 0
		creations := 0
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return notFoundErr
			},
			create: func(context.Context, string, string, string) error {
				creations++

				return nil
			},
			isNotFound: func(err error) bool { return errors.Is(err, notFoundErr) },
			cache:      &labelDefinitionCache{},
		}

		// when: the definition is ensured twice
		firstErr := definitions.ensure(context.Background(), "release", "ffffff", "release")
		secondErr := definitions.ensure(context.Background(), "release", "ffffff", "release")

		// then: one successful creation satisfies the second request
		testastic.NoError(t, firstErr)
		testastic.NoError(t, secondErr)
		testastic.Equal(t, 1, lookups)
		testastic.Equal(t, 1, creations)
	})

	t.Run("does not cache failed creation", func(t *testing.T) {
		t.Parallel()

		// given: a missing definition whose creation remains unavailable
		notFoundErr := errors.New("not found")
		createErr := errors.New("creation unavailable")
		lookups := 0
		creations := 0
		definitions := labelDefinitions{
			get: func(context.Context, string) error {
				lookups++

				return notFoundErr
			},
			create: func(context.Context, string, string, string) error {
				creations++

				return createErr
			},
			isNotFound: func(err error) bool { return errors.Is(err, notFoundErr) },
			cache:      &labelDefinitionCache{},
		}

		// when: ensuring the definition is attempted twice
		firstErr := definitions.ensure(context.Background(), "release", "ffffff", "release")
		secondErr := definitions.ensure(context.Background(), "release", "ffffff", "release")

		// then: both attempts reach the provider and preserve the creation failure
		testastic.ErrorIs(t, firstErr, createErr)
		testastic.ErrorIs(t, secondErr, createErr)
		testastic.Equal(t, 2, lookups)
		testastic.Equal(t, 2, creations)
	})
}

func TestManagedLabelChangeMovesAnEmptyPullRequestThroughBothPhases(t *testing.T) {
	t.Parallel()

	// given: a pull request carrying no labels at all
	labels := policyTestLabels()

	// when: the pending phase is applied
	pending := applyLabelChange(nil, managedLabelChange(labels, forge.ReleasePRPhasePending), foldedLabelMatch)

	// then: it carries pending, the extras and the yeet marker, and not tagged
	testastic.SliceEqual(
		t,
		[]string{labels.Pending, "release", "automated", ReleaseLabelYeet},
		pending,
	)

	// when: the tagged phase is applied to that result
	tagged := applyLabelChange(pending, managedLabelChange(labels, forge.ReleasePRPhaseTagged), foldedLabelMatch)

	// then: the lifecycle label flips and nothing else moves
	testastic.SliceEqual(
		t,
		[]string{"release", "automated", ReleaseLabelYeet, labels.Tagged},
		tagged,
	)
}

func TestManagedLabelChangeLeavesMaintainerLabelsAlone(t *testing.T) {
	t.Parallel()

	// given: a pending release pull request a maintainer has also labelled
	labels := policyTestLabels()
	current := []string{labels.Pending, "priority/high", "area/api"}

	// when: both phases are applied in turn
	pending := applyLabelChange(current, managedLabelChange(labels, forge.ReleasePRPhasePending), foldedLabelMatch)
	tagged := applyLabelChange(pending, managedLabelChange(labels, forge.ReleasePRPhaseTagged), foldedLabelMatch)

	// then: the maintainer's labels survive both transitions
	for _, phase := range [][]string{pending, tagged} {
		testastic.True(t, slices.Contains(phase, "priority/high"))
		testastic.True(t, slices.Contains(phase, "area/api"))
	}
}
