//nolint:testpackage // This test exercises the unexported release-unit lifecycle seam.
package release

import (
	"encoding/json/v2"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type lifecycleTestError struct{}

func (e *lifecycleTestError) Error() string {
	return "test lifecycle failure"
}

func TestReleaseUnitLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("preview returns fresh rendered outcomes without mutation", func(t *testing.T) {
		t.Parallel()

		// given: two planned independent release units
		r, stub, units := newPlannedIndependentLifecycle(t, false)

		// when: previewing the collection twice
		first, firstErr := r.lifecycle.preview(t.Context(), units)
		testastic.NoError(t, firstErr)

		first.units[0].unit = "changed"
		second, secondErr := r.lifecycle.preview(t.Context(), units)

		// then: every unit is rendered into a fresh batch without provider mutation
		testastic.NoError(t, secondErr)
		testastic.Equal(t, 2, len(second.units))
		testastic.Equal(t, "target:api", second.units[0].unit)
		testastic.NotNil(t, second.units[0].text)
		testastic.NotNil(t, second.units[1].text)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 0, stub.mergePRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
		testastic.Equal(t, 0, len(stub.markPendingCalls))
		testastic.Equal(t, 0, len(stub.markTaggedCalls))
	})

	t.Run("apply validates every manifest before the first write", func(t *testing.T) {
		t.Parallel()

		// given: the second planned unit has an invalid persisted manifest
		r, stub, units := newPlannedIndependentLifecycle(t, false)
		stub.openPending = []*forge.PullRequest{{
			Number: 8,
			Branch: units[1].ReleaseBranch,
		}}

		// when: applying the complete planned collection
		outcome, err := r.lifecycle.apply(t.Context(), units)

		// then: every unit was rendered, but no planned-unit mutation began
		testastic.ErrorIs(t, err, errInvalidReleaseManifest)
		testastic.Equal(t, len(units), len(outcome.units))
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
		testastic.Equal(t, 0, stub.mergePRCalls)
		testastic.Equal(t, 0, stub.createReleaseCalls)
	})

	t.Run("apply reconciles the complete collection before auto merge", func(t *testing.T) {
		t.Parallel()

		// given: two planned independent units with auto merge enabled
		r, stub, units := newPlannedIndependentLifecycle(t, true)

		// when: applying the complete planned collection
		outcome, err := r.lifecycle.apply(t.Context(), units)

		// then: both reconciliation waves finish before the first sequential merge
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{
			"UpdateFiles",
			"CreateReleasePR",
			"UpdateFiles",
			"CreateReleasePR",
			"MergeReleasePR",
			"MergeReleasePR",
		}, stub.sequence.calls)
		testastic.SliceEqual(t, []int{1, 2}, stub.mergePRNumbers)
		testastic.Equal(t, 2, len(outcome.releases))
	})

	t.Run("apply skips failed reconciliation and continues later eligible units", func(t *testing.T) {
		t.Parallel()

		// given: the first unit fails pull request creation and auto merge is enabled
		r, stub, units := newPlannedIndependentLifecycle(t, true)
		stub.createPRErrByCall = map[int]error{1: errTestUnitCreate}

		// when: applying the complete planned collection
		outcome, err := r.lifecycle.apply(t.Context(), units)

		// then: the later unit reconciles and merges after the reconciliation wave
		testastic.ErrorIs(t, err, errTestUnitCreate)
		testastic.ErrorIs(t, outcome.units[0].err, errTestUnitCreate)
		testastic.Nil(t, outcome.units[0].pullRequest)
		testastic.NotNil(t, outcome.units[1].pullRequest)
		testastic.SliceEqual(t, []int{2}, stub.mergePRNumbers)
		testastic.Equal(t, 1, len(outcome.units[1].releases))
		testastic.Equal(t, 1, len(outcome.releases))
	})

	t.Run("finalize retains later publications and typed error identity", func(t *testing.T) {
		t.Parallel()

		// given: two merged units where publishing the first returns a typed error
		cfg := newIndependentLifecycleConfig(t)
		stub := newProviderStub()
		r := newTestReleaser(t, cfg, stub)
		units, unitErr := configuredReleaseUnits(r.core)
		testastic.NoError(t, unitErr)
		stub.mergedPRResponses = []*forge.PullRequest{
			independentMergedPullRequest(
				t,
				units[0],
				1,
				"api-merge-sha",
				"api",
				"api-v1.0.0",
				"api/CHANGELOG.md",
			),
			independentMergedPullRequest(
				t,
				units[1],
				2,
				"web-merge-sha",
				"web",
				"web-v2.0.0",
				"web/CHANGELOG.md",
			),
			nil,
		}
		stub.files[providerFileKey(cfg.Branch, "api/CHANGELOG.md")] = "## api-v1.0.0\n\napi"
		stub.files[providerFileKey(cfg.Branch, "web/CHANGELOG.md")] = "## web-v2.0.0\n\nweb"
		stub.createReleaseErrByCall = map[int]error{1: &lifecycleTestError{}}

		// when: finalizing the complete configured collection
		outcome, err := r.lifecycle.finalize(t.Context(), units)

		// then: the later publication remains visible and errors.As reaches the cause
		var typedErr *lifecycleTestError

		testastic.Error(t, err)
		testastic.True(t, errors.As(err, &typedErr))
		testastic.Equal(t, 1, stub.findMergedPRsCalls)
		testastic.Equal(t, len(units), len(outcome.units))
		testastic.Error(t, outcome.units[0].err)
		testastic.Equal(t, 1, len(outcome.units[1].releases))
		testastic.Equal(t, "web-merge-sha", outcome.units[1].releases[0].CommitSHA)
		testastic.Equal(t, 1, len(outcome.releases))
	})
}

func newIndependentLifecycleConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, _, err := config.LoadResolvedQuiet(t.Context(), "testdata/independent_workflow/config.input.yaml")
	testastic.NoError(t, err)

	return cfg
}

func newPlannedIndependentLifecycle(
	t *testing.T,
	autoMerge bool,
) (*releaser, *providerStub, []releaseUnit) {
	t.Helper()

	cfg := newIndependentLifecycleConfig(t)
	cfg.Release.AutoMerge = autoMerge
	stub := newProviderStub()
	err := json.Unmarshal(
		[]byte(readTestFile(t, "testdata/independent_workflow/commits.input.json")),
		&stub.commits,
	)
	testastic.NoError(t, err)
	r := newTestReleaser(t, cfg, stub)
	selection, err := selectTargets(r.core, nil)
	testastic.NoError(t, err)
	plans, err := analyze(t.Context(), r.core, r.source, selection, nil)
	testastic.NoError(t, err)
	units, err := planReleaseUnits(r.core, plans)
	testastic.NoError(t, err)

	return r, stub, units
}
