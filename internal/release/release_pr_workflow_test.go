//nolint:testpackage // This test validates unexported release PR workflow behavior.
package release

import (
	"context"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

func TestReleaseRefreshWritesBranchFilesBeforeTheManifest(t *testing.T) {
	t.Parallel()

	// given: an open pending release PR and a commit that moves the version on
	cfg := config.Default()

	stub := newProviderStub()
	stub.openPending = []*forge.PullRequest{{
		Number: 7,
		URL:    "https://example.com/pr/7",
		Branch: "yeet/release-main",
	}}
	stub.commits = []history.CommitEntry{{
		Hash:    "abcdef1234567890",
		Message: "feat: add a thing",
	}}

	r := newTestReleaser(t, cfg, stub)

	// when: the run refreshes that pull request
	_, err := r.Release(context.Background(), false)

	// then: the branch carries the new tags and changelog before the body
	// advertises them, so a merge inside the window publishes the older tag the
	// next run re-plans from instead of one whose commits were never written
	testastic.NoError(t, err)
	testastic.SliceEqual(t, []string{"UpdateFiles", "UpdateReleasePR"}, stub.sequence.calls)
}

func TestPreserveTargetChangelogEdits(t *testing.T) {
	t.Parallel()

	t.Run("drops a child target section absent from this release wave", func(t *testing.T) {
		t.Parallel()

		// given: a release branch entry written by a wave that released both children
		cfg := config.Default()
		cfg.Targets = map[string]config.Target{
			"api":  {Type: config.TargetTypePath, Path: "services/api", TagPrefix: "api-v"},
			"web":  {Type: config.TargetTypePath, Path: "apps/web", TagPrefix: "web-v"},
			"root": {Type: config.TargetTypeDerived, Includes: []string{"api", "web"}, TagPrefix: "v"},
		}

		branch := "yeet/release-main"
		stub := newProviderStub()
		stub.files[providerFileKey(branch, "CHANGELOG.md")] = strings.TrimSpace(`
# Changelog

## v3.1.0 (2026-03-01)

### api

### Features

- add token rotation (abcdef1)

### web

### Bug Fixes

- patch dashboard (1234567)
`)

		r := newTestReleaser(t, cfg, stub)
		workflow := newReleasePRWorkflow(r.core, r.source, r.prs, r.files, r.publisher)

		// and: a refreshed wave in which only the web child releases
		webPlan := TargetPlan{
			ID: "web",
			Entry: changelog.ParseEntry(
				"## web-v2.1.5 (2026-03-02)\n\n### Bug Fixes\n\n- patch dashboard again (89abcde)\n",
			),
		}

		plan := TargetPlan{
			ID:              "root",
			Type:            config.TargetTypeDerived,
			NextTag:         "v3.1.0",
			IncludedTargets: []string{"web"},
			Entry: derivedChangelogEntry(
				t.Context(),
				r.core.targets["root"],
				"v3.1.0",
				"",
				nil,
				[]TargetPlan{webPlan},
				"",
				derivedChangelogRelease,
				r.core.metadata,
			),
		}
		plan.PREntry = plan.Entry

		// when: refreshing the release PR against the previous wave's entry
		err := workflow.preserveTargetChangelogEdits(t.Context(), branch, "CHANGELOG.md", "v3.1.0", &plan)

		// then: the child that did not release again leaves no stale heading behind
		testastic.NoError(t, err)
		testastic.NotContains(t, changelog.Render(plan.Entry), "### api")
		testastic.NotContains(t, changelog.Render(plan.PREntry), "### api")
	})
}
