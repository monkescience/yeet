//nolint:testpackage // This test exercises the unit-scoped release orchestration seam.
package release

import (
	"encoding/json/v2"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	changelogpkg "github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

var errTestUnitCreate = errors.New("test unit create failure")

var errTestUnitMerge = errors.New("test unit merge failure")

var errTestUnitPublish = errors.New("test unit publication failure")

var errTestMergedDiscovery = errors.New("test merged discovery failure")

func TestIndependentReleaseWorkflow(t *testing.T) {
	newConfig := func(t *testing.T) *config.Config {
		t.Helper()

		cfg, _, err := config.LoadResolvedQuiet(t.Context(), "testdata/independent_workflow/config.input.yaml")
		testastic.NoError(t, err)

		return cfg
	}

	newStub := func(t *testing.T) *providerStub {
		t.Helper()

		stub := newProviderStub()
		err := json.Unmarshal(
			[]byte(readTestFile(t, "testdata/independent_workflow/commits.input.json")),
			&stub.commits,
		)
		testastic.NoError(t, err)

		return stub
	}

	t.Run("creates every eligible unit in one run", func(t *testing.T) {
		// given: two independently releasable targets with changes
		cfg := newConfig(t)
		stub := newStub(t)
		r := newTestReleaser(t, cfg, stub)

		// when: performing one release run
		result, err := r.Release(t.Context(), false)

		// then: both units receive distinct branches and pull requests
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(result.Units))
		testastic.Equal(t, 2, stub.createPRCalls)
		testastic.False(t, stub.createPROptions[0].ReleaseBranch == stub.createPROptions[1].ReleaseBranch)
		testastic.Equal(t, "target:api", result.Units[0].Unit)
		testastic.Equal(t, "target:web", result.Units[1].Unit)

		manifest, found, manifestErr := releaseManifestFromBody(result.Units[0].PullRequest.Body)
		testastic.NoError(t, manifestErr)
		testastic.True(t, found)
		testastic.Equal(t, "target:api", manifest.Unit)
		testastic.SliceEqual(t, []releaseManifestTarget{{ID: "api", Type: "path"}}, manifest.ConfiguredTargets)
		testastic.AssertJSON(
			t,
			"testdata/independent_workflow/creates_every_eligible_unit/manifest.expected.json",
			manifest,
		)
	})

	t.Run("preserves manual changelog edits on only the matching unit branch", func(t *testing.T) {
		// given: both unit branches contain different manual release notes
		cfg := newConfig(t)
		stub := newStub(t)
		r := newTestReleaser(t, cfg, stub)
		preview, previewErr := r.Release(t.Context(), true)
		testastic.NoError(t, previewErr)

		stub.openPending = make([]*forge.PullRequest, 0, len(preview.Units))
		for index, outcome := range preview.Units {
			plan := outcome.Plans[0]
			branch := outcome.Text.PROptions.ReleaseBranch
			note := "manual " + plan.ID + " note"
			stub.files[providerFileKey(branch, plan.ChangelogFile)] = changelogpkg.Render(plan.Entry) +
				"\n### Manual\n\n" + note + "\n"
			stub.openPending = append(stub.openPending, &forge.PullRequest{
				Number: index + 1,
				URL:    "https://example.com/pr/" + plan.ID,
				Branch: branch,
				Body:   outcome.Text.PROptions.Body,
			})
		}

		// when: refreshing every independent unit
		_, err := r.Release(t.Context(), false)

		// then: each branch keeps its own note without copying the other unit's edit
		testastic.NoError(t, err)

		for _, outcome := range preview.Units {
			plan := outcome.Plans[0]
			branch := outcome.Text.PROptions.ReleaseBranch
			content := stub.files[providerFileKey(branch, plan.ChangelogFile)]
			testastic.AssertFile(
				t,
				"testdata/independent_workflow/preserves_manual_changelog_edits/"+
					plan.ID+".expected.md",
				content,
			)
		}
	})

	t.Run("continues after one unit fails and returns partial outcomes", func(t *testing.T) {
		// given: the provider rejects the first unit pull request only
		cfg := newConfig(t)
		stub := newStub(t)
		stub.createPRErrByCall = map[int]error{1: errTestUnitCreate}
		r := newTestReleaser(t, cfg, stub)

		// when: performing one release run
		result, err := r.Release(t.Context(), false)

		// then: the second unit is still reconciled and both outcomes remain observable
		testastic.ErrorIs(t, err, errTestUnitCreate)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/continues_after_create_failure/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 2, stub.createPRCalls)
		testastic.Equal(t, 2, len(result.Units))
		testastic.ErrorIs(t, result.Units[0].Error, errTestUnitCreate)
		testastic.NotNil(t, result.Units[1].PullRequest)
	})

	t.Run("duplicate pending requests fail only their matching unit", func(t *testing.T) {
		// given: two pending pull requests exist for the api unit only
		cfg := newConfig(t)
		stub := newStub(t)
		r := newTestReleaser(t, cfg, stub)
		units, planErr := planReleaseUnits(r.core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath},
			{ID: "web", Type: config.TargetTypePath},
		})
		testastic.NoError(t, planErr)

		stub.openPending = []*forge.PullRequest{
			{Number: 10, URL: "https://example.com/pr/10", Branch: units[0].ReleaseBranch},
			{Number: 11, URL: "https://example.com/pr/11", Branch: units[0].ReleaseBranch},
		}

		// when: reconciling all eligible units
		result, err := r.Release(t.Context(), false)

		// then: api reports the ambiguity while web still creates its pull request
		testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/duplicate_pending_requests/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 1, stub.createPRCalls)
		testastic.ErrorIs(t, result.Units[0].Error, ErrMultiplePendingReleasePRs)
		testastic.NotNil(t, result.Units[1].PullRequest)
	})

	t.Run("attempts later auto-merges after an earlier unit fails", func(t *testing.T) {
		// given: two reconciled units where the first merge fails
		cfg := newConfig(t)
		cfg.Release.AutoMerge = true
		stub := newStub(t)
		stub.mergePRErrByNumber = map[int]error{1: errTestUnitMerge}
		r := newTestReleaser(t, cfg, stub)

		// when: performing one release run with automatic merging
		result, err := r.Release(t.Context(), false)

		// then: the second unit is merged and published after the first failure
		testastic.ErrorIs(t, err, errTestUnitMerge)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/continues_after_merge_failure/error.expected.txt",
			err.Error(),
		)
		testastic.SliceEqual(t, []int{1, 2}, stub.mergePRNumbers)
		testastic.ErrorIs(t, result.Units[0].Error, errTestUnitMerge)
		testastic.Equal(t, 1, len(result.Units[1].Releases))
		testastic.Equal(t, stub.mergePRSHA, result.Units[1].Releases[0].CommitSHA)
	})

	t.Run("fails closed when a legacy combined pull request remains open", func(t *testing.T) {
		// given: independent mode with an open combined-mode release pull request
		cfg := newConfig(t)
		stub := newProviderStub()
		stub.openPending = []*forge.PullRequest{{
			Number: 7,
			URL:    "https://example.com/pr/7",
			Branch: "yeet/release-main",
		}}
		r := newTestReleaser(t, cfg, stub)

		// when: performing one release run
		_, err := r.Release(t.Context(), false)

		// then: no unit branch or pull request is mutated under the new layout
		testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/legacy_combined_request/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})

	t.Run("fails closed when a renamed group leaves its old pull request open", func(t *testing.T) {
		// given: the configured group was renamed while its former pull request remained pending
		cfg := newConfig(t)
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"apps": {Targets: []string{"api", "web"}},
		}
		stub := newStub(t)
		r := newTestReleaser(t, cfg, stub)

		tmpl, templateErr := newReleaseBranchTemplate(effectiveReleaseBranchTemplateSource(cfg))
		testastic.NoError(t, templateErr)

		oldBranch, branchErr := renderReleaseBranch(
			tmpl,
			cfg.Branch,
			"",
			releaseUnitBranchValue("group", "backend"),
		)
		testastic.NoError(t, branchErr)

		marker, markerErr := releaseManifestMarker(releaseManifest{
			Unit:       "group:backend",
			BaseBranch: cfg.Branch,
			ConfiguredTargets: []releaseManifestTarget{
				{ID: "api", Type: string(config.TargetTypePath)},
				{ID: "web", Type: string(config.TargetTypePath)},
			},
			Targets: []releaseManifestEntry{
				{ID: "api", Type: string(config.TargetTypePath), Tag: "api-v1.0.0", ChangelogFile: "api/CHANGELOG.md"},
			},
		})
		testastic.NoError(t, markerErr)

		stub.openPending = []*forge.PullRequest{
			{Number: 9, URL: "https://example.com/pr/9", Branch: oldBranch, Body: marker},
		}

		// when: reconciling under the renamed group layout
		_, err := r.Release(t.Context(), false)

		// then: the stale request is reported and no current unit is mutated
		testastic.ErrorIs(t, err, ErrMultiplePendingReleasePRs)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/renamed_group_request/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})

	t.Run("validates every existing unit manifest before branch writes", func(t *testing.T) {
		// given: the second unit branch carries a manifest for the first unit
		cfg := newConfig(t)
		stub := newStub(t)
		r := newTestReleaser(t, cfg, stub)
		units, planErr := planReleaseUnits(r.core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath},
			{ID: "web", Type: config.TargetTypePath},
		})
		testastic.NoError(t, planErr)

		marker, markerErr := releaseManifestMarker(releaseManifest{
			Unit:       "target:api",
			BaseBranch: cfg.Branch,
			ConfiguredTargets: []releaseManifestTarget{
				{ID: "api", Type: string(config.TargetTypePath)},
			},
			Targets: []releaseManifestEntry{
				{ID: "api", Type: string(config.TargetTypePath), Tag: "api-v1.0.0", ChangelogFile: "api/CHANGELOG.md"},
			},
		})
		testastic.NoError(t, markerErr)

		stub.openPending = []*forge.PullRequest{{Number: 8, Branch: units[1].ReleaseBranch, Body: marker}}

		// when: reconciling the independent layout
		_, err := r.Release(t.Context(), false)

		// then: the mismatched manifest blocks every unit before a branch is updated
		testastic.ErrorIs(t, err, errInvalidReleaseManifest)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/mismatched_manifest/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})

	t.Run("finalization discovery failure prevents release analysis", func(t *testing.T) {
		// given: merged release discovery fails before target history is queried
		cfg := newConfig(t)
		stub := newStub(t)
		stub.findMergedPRsErr = errTestMergedDiscovery
		r := newTestReleaser(t, cfg, stub)

		// when: running the release
		result, err := r.Release(t.Context(), false)

		// then: no target analysis or reconciliation mutation begins
		testastic.ErrorIs(t, err, errTestMergedDiscovery)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/finalization_discovery_failure/error.expected.txt",
			err.Error(),
		)
		testastic.NotNil(t, result)
		testastic.Equal(t, 0, stub.getCommitsSinceRefsCalls)
		testastic.Equal(t, 0, stub.updateFilesCalls)
		testastic.Equal(t, 0, stub.createPRCalls)
	})

	t.Run("finalizes discovered units after another branch discovery fails", func(t *testing.T) {
		// given: api discovery fails while a merged web unit remains discoverable
		cfg := newConfig(t)
		stub := newProviderStub()
		r := newTestReleaser(t, cfg, stub)
		units, unitErr := configuredReleaseUnits(r.core)
		testastic.NoError(t, unitErr)

		stub.mergedPRErrByCall = map[int]error{1: errTestMergedDiscovery}
		stub.mergedPRResponses = []*forge.PullRequest{
			nil,
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
		stub.files[providerFileKey(cfg.Branch, "web/CHANGELOG.md")] = "## web-v2.0.0 (2026-08-29)\n\nweb"

		// when: running the finalization prerequisite
		result, err := r.Release(t.Context(), false)

		// then: web finalizes, the discovery error remains, and new analysis stays blocked
		testastic.ErrorIs(t, err, errTestMergedDiscovery)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/partial_finalization_discovery/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 1, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.getCommitsSinceRefsCalls)
		testastic.Equal(t, 1, len(result.Units[1].Releases))
		testastic.Equal(t, "web-merge-sha", result.Units[1].Releases[0].CommitSHA)
	})

	t.Run("discovers and finalizes all merged unit branches together", func(t *testing.T) {
		// given: two merged unit pull requests with different merge commits
		cfg := newConfig(t)
		stub := newProviderStub()
		r := newTestReleaser(t, cfg, stub)
		units, unitErr := configuredReleaseUnits(r.core)
		testastic.NoError(t, unitErr)

		apiPR := independentMergedPullRequest(
			t,
			units[0],
			1,
			"api-merge-sha",
			"api",
			"api-v1.0.0",
			"api/CHANGELOG.md",
		)
		webPR := independentMergedPullRequest(
			t,
			units[1],
			2,
			"web-merge-sha",
			"web",
			"web-v2.0.0",
			"web/CHANGELOG.md",
		)
		stub.mergedPRResponses = []*forge.PullRequest{apiPR, webPR, nil}
		stub.files[providerFileKey(cfg.Branch, "api/CHANGELOG.md")] = "## api-v1.0.0 (2026-08-29)\n\napi"
		stub.files[providerFileKey(cfg.Branch, "web/CHANGELOG.md")] = "## web-v2.0.0 (2026-08-29)\n\nweb"

		// when: running the finalization prerequisite
		result, err := r.Release(t.Context(), false)

		// then: one multi-branch discovery finalizes both units at their own merge commits
		testastic.NoError(t, err)
		testastic.Equal(t, 1, stub.findMergedPRsCalls)
		testastic.Equal(t, 2, len(result.Releases))
		testastic.Equal(t, "api-merge-sha", result.Releases[0].CommitSHA)
		testastic.Equal(t, "web-merge-sha", result.Releases[1].CommitSHA)
		testastic.Equal(t, 1, stub.getCommitsSinceRefsCalls)
		testastic.SliceEqual(t, []string{"api-v1.0.0", "web-v2.0.0"}, stub.getCommitsSinceRefsOf[0])
	})

	t.Run("finalizes a merged legacy combined request after independent mode is enabled", func(t *testing.T) {
		// given: a legacy combined request merged before the layout changed
		cfg := newConfig(t)
		stub := newProviderStub()
		r := newTestReleaser(t, cfg, stub)
		marker, markerErr := releaseManifestMarker(releaseManifest{
			BaseBranch: cfg.Branch,
			Targets: []releaseManifestEntry{
				{ID: "api", Type: string(config.TargetTypePath), Tag: "api-v1.0.0", ChangelogFile: "api/CHANGELOG.md"},
			},
		})
		testastic.NoError(t, markerErr)

		stub.mergedPRResponses = []*forge.PullRequest{
			nil,
			nil,
			{
				Number:         3,
				URL:            "https://example.com/pr/3",
				Body:           marker,
				Branch:         r.core.run.releaseBranch,
				MergeCommitSHA: "legacy-merge-sha",
			},
		}
		stub.files[providerFileKey(cfg.Branch, "api/CHANGELOG.md")] = "## api-v1.0.0 (2026-08-29)\n\napi"

		// when: running the independent release workflow
		result, err := r.Release(t.Context(), false)

		// then: the missing unit identity is treated as combined only for finalization
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(result.Releases))
		testastic.Equal(t, "legacy-merge-sha", result.Releases[0].CommitSHA)
		testastic.Equal(t, 3, stub.markTaggedCalls[0])
	})

	t.Run("continues finalization after an earlier unit publication fails", func(t *testing.T) {
		// given: two merged units where publishing the first unit fails
		cfg := newConfig(t)
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
		stub.files[providerFileKey(cfg.Branch, "api/CHANGELOG.md")] = "## api-v1.0.0 (2026-08-29)\n\napi"
		stub.files[providerFileKey(cfg.Branch, "web/CHANGELOG.md")] = "## web-v2.0.0 (2026-08-29)\n\nweb"
		stub.createReleaseErrByCall = map[int]error{1: errTestUnitPublish}

		// when: running the finalization prerequisite
		result, err := r.Release(t.Context(), false)

		// then: web still publishes, but the finalization failure blocks new analysis
		testastic.ErrorIs(t, err, errTestUnitPublish)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/continues_after_publication_failure/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 2, stub.createReleaseCalls)
		testastic.Equal(t, 0, stub.getCommitsSinceRefsCalls)
		testastic.ErrorIs(t, result.Units[0].Error, errTestUnitPublish)
		testastic.Equal(t, 1, len(result.Units[1].Releases))
		testastic.Equal(t, "web-merge-sha", result.Units[1].Releases[0].CommitSHA)
	})

	t.Run("retains releases completed before a grouped publication failure", func(t *testing.T) {
		// given: an atomic group whose second provider release fails
		cfg := newConfig(t)
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"apps": {Targets: []string{"api", "web"}},
		}
		cfg.Release.AutoMerge = true
		stub := newStub(t)
		stub.createReleaseErrByCall = map[int]error{2: errTestUnitPublish}
		r := newTestReleaser(t, cfg, stub)

		// when: auto-merging the grouped unit
		result, err := r.Release(t.Context(), false)

		// then: the completed first release remains visible in the partial outcome
		testastic.ErrorIs(t, err, errTestUnitPublish)
		testastic.AssertFile(
			t,
			"testdata/independent_workflow/grouped_publication_failure/error.expected.txt",
			err.Error(),
		)
		testastic.Equal(t, 1, len(result.Units))
		testastic.Equal(t, 1, len(result.Units[0].Releases))
		testastic.Equal(t, 1, len(result.Releases))
		testastic.ErrorIs(t, result.Units[0].Error, errTestUnitPublish)
	})
}

func independentMergedPullRequest(
	t *testing.T,
	unit releaseUnit,
	number int,
	mergeCommitSHA, targetID, tag, changelogFile string,
) *forge.PullRequest {
	t.Helper()

	marker, err := releaseManifestMarker(releaseManifest{
		Unit:       unit.ID,
		BaseBranch: "main",
		ConfiguredTargets: []releaseManifestTarget{
			{ID: targetID, Type: string(config.TargetTypePath)},
		},
		Targets: []releaseManifestEntry{
			{ID: targetID, Type: string(config.TargetTypePath), Tag: tag, ChangelogFile: changelogFile},
		},
	})
	testastic.NoError(t, err)

	return &forge.PullRequest{
		Number:         number,
		URL:            "https://example.com/pr/" + tag,
		Body:           marker,
		Branch:         unit.ReleaseBranch,
		MergeCommitSHA: mergeCommitSHA,
	}
}
