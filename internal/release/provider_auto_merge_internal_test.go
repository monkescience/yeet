package release

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

func TestProviderAutoMergePublicationBoundary(t *testing.T) {
	t.Parallel()

	for _, failScheduling := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "refused"}[failScheduling], func(t *testing.T) {
			t.Parallel()

			// given: provider scheduling and a publication preflight that would fail
			cfg := config.Default()
			cfg.Release.AutoMerge = true
			stub := newProviderStub()
			stub.preflightErr = forge.ErrReleasePRLabelMissing

			if failScheduling {
				stub.autoMergeErr = forge.ErrMergeBlocked
			}

			r := newTestReleaser(t, cfg, stub)
			pr := &forge.PullRequest{Number: 12, URL: "https://example.com/pr/12", Branch: "yeet/release-main"}

			// when: scheduling a reconciled release pull request
			releases, err := r.lifecycle.autoMerge(t.Context(), pr, nil, nil)

			// then: scheduling never touches publication or falls back to direct merge
			if failScheduling {
				testastic.ErrorIs(t, err, forge.ErrMergeBlocked)
			} else {
				testastic.NoError(t, err)
			}

			testastic.SliceEqual(t, []int{12}, stub.autoMergeNumbers)
			testastic.Equal(t, cfg.Branch, stub.autoMergeOptions[0].BaseBranch)
			testastic.Equal(t, pr.Branch, stub.autoMergeOptions[0].ReleaseBranch)
			testastic.Equal(t, forge.MergeMethodAuto, stub.autoMergeOptions[0].Method)
			testastic.Equal(t, 0, len(releases))
			testastic.Equal(t, 0, len(stub.preflightCalls))
			testastic.Equal(t, 0, stub.mergePRCalls)
			testastic.Equal(t, 0, stub.getReleaseByTagCalls)
			testastic.Equal(t, 0, stub.createReleaseCalls)
			testastic.Equal(t, 0, len(stub.markTaggedCalls))
		})
	}
}

func TestProviderAutoMergeContinuesIndependentUnits(t *testing.T) {
	t.Parallel()

	// given: two reconciled independent units with the first scheduling request refused
	r, stub, units := newPlannedIndependentLifecycle(t, true)
	r.core.run.autoMerge.mode = config.AutoMergeModeProvider
	stub.autoMergeErrByNumber = map[int]error{1: errTestUnitMerge}

	// when: reconciling and scheduling both units
	outcome, err := r.lifecycle.apply(t.Context(), units)

	// then: the later request is accepted and neither unit enters publication
	testastic.ErrorIs(t, err, errTestUnitMerge)
	testastic.SliceEqual(t, []int{1, 2}, stub.autoMergeNumbers)
	testastic.ErrorIs(t, outcome.units[0].err, errTestUnitMerge)
	testastic.NoError(t, outcome.units[1].err)
	testastic.Equal(t, 0, len(outcome.releases))
	testastic.Equal(t, 0, stub.mergePRCalls)
	testastic.Equal(t, 0, stub.createReleaseCalls)
}

func TestAutoMergeModeResolution(t *testing.T) {
	t.Parallel()

	t.Run("mode override is independent of method", func(t *testing.T) {
		t.Parallel()

		// given: provider mode enabled with a configured squash method
		cfg := config.Default()
		cfg.Release.AutoMerge = true
		cfg.Release.AutoMergeMethod = config.AutoMergeMethodSquash

		// when: selecting direct mode for one run
		run, err := resolveRun(cfg, "main", Options{AutoMergeMode: new("direct")})

		// then: the override changes only the run mode and preserves the configuration
		testastic.NoError(t, err)
		testastic.True(t, run.autoMerge.enabled)
		testastic.Equal(t, config.AutoMergeModeDirect, run.autoMerge.mode)
		testastic.Equal(t, config.AutoMergeMethodSquash, run.autoMerge.method)
		testastic.Equal(t, config.AutoMergeModeProvider, cfg.Release.AutoMergeMode)
	})

	t.Run("invalid override is rejected even when disabled", func(t *testing.T) {
		t.Parallel()

		// given: auto merge disabled by default
		cfg := config.Default()

		// when: selecting an unsupported mode
		_, err := resolveRun(cfg, "main", Options{AutoMergeMode: new("invalid")})

		// then: mode validation fails without changing the configured default
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(t, config.AutoMergeModeProvider, cfg.Release.AutoMergeMode)
	})
}
