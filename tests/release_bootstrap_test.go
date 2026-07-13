package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseSemverBootstrap(t *testing.T) {
	t.Parallel()

	t.Run("github bootstraps a semver project with no prior tag", func(t *testing.T) {
		t.Parallel()

		// given: a semver project that has never tagged a release
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: initial feature"},
				{SHA: "boundary-sha", Message: "chore: bootstrap"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans the initial semver release (0.0.0 -> 0.0.1) and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_semver_bootstrap/github_initial/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseCommitMix(t *testing.T) {
	t.Parallel()

	t.Run("github groups feat, fix, perf and revert into separate sections", func(t *testing.T) {
		t.Parallel()

		// given: commits across every default changelog section
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "feat-sha", Message: "feat: ship a thing"},
				{SHA: "fix-sha", Message: "fix: fix a thing"},
				{SHA: "perf-sha", Message: "perf: speed up a thing"},
				{SHA: "revert-sha", Message: "revert: roll back the prior feat"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: each commit type appears under its own changelog section
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_commit_mix/github_all_sections/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseAutoMergeForce(t *testing.T) {
	t.Parallel()

	t.Run("github --auto-merge-force does not bypass draft state", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server that flags the PR as draft (merge-blocked)
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:        "testorg",
			Repo:         "testrepo",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			MergeBlocked: true,
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release --auto-merge --auto-merge-force` against the blocked PR
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-force",
				"--config", configPath,
			},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: GitHub still rejects merging a draft PR and the binary surfaces the block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "draft")
	})
}
