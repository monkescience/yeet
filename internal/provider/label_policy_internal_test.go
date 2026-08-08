package provider

import (
	"slices"
	"testing"

	"github.com/monkescience/testastic"
)

func policyTestLabels() ReleasePRLabels {
	return ReleasePRLabels{
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
	pending := managedLabelChange(labels, ReleasePRPhasePending)
	tagged := managedLabelChange(labels, ReleasePRPhaseTagged)

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

func TestManagedLabelChangeMovesAnEmptyPullRequestThroughBothPhases(t *testing.T) {
	t.Parallel()

	// given: a pull request carrying no labels at all
	labels := policyTestLabels()

	// when: the pending phase is applied
	pending := applyLabelChange(nil, managedLabelChange(labels, ReleasePRPhasePending), foldedLabelMatch)

	// then: it carries pending, the extras and the yeet marker, and not tagged
	testastic.SliceEqual(
		t,
		[]string{labels.Pending, "release", "automated", ReleaseLabelYeet},
		pending,
	)

	// when: the tagged phase is applied to that result
	tagged := applyLabelChange(pending, managedLabelChange(labels, ReleasePRPhaseTagged), foldedLabelMatch)

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
	pending := applyLabelChange(current, managedLabelChange(labels, ReleasePRPhasePending), foldedLabelMatch)
	tagged := applyLabelChange(pending, managedLabelChange(labels, ReleasePRPhaseTagged), foldedLabelMatch)

	// then: the maintainer's labels survive both transitions
	for _, phase := range [][]string{pending, tagged} {
		testastic.True(t, slices.Contains(phase, "priority/high"))
		testastic.True(t, slices.Contains(phase, "area/api"))
	}
}
