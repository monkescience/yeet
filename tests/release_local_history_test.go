package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseLocalHistory(t *testing.T) {
	t.Parallel()

	t.Run("matching checkout serves commit ranges from local git", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout whose head matches the provider branch head,
		// while the provider's compare fixture would tell a different story
		repoDir, boundarySHA, headSHA := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "acme",
			Repo:        "repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: boundarySHA,
			Commits: []fakeprovider.GitHubCommit{
				{SHA: headSHA, Message: "fix: remote patch"},
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: invoking `yeet release --dry-run` inside the checkout
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the plan reflects the local feat commit, not the provider fixture
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/matching_checkout_serves_commit_ranges_from_local_git/"+
				"stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("stale checkout fails with an actionable error", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout whose head differs from the provider branch head
		repoDir, boundarySHA, _ := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   boundarySHA,
			BranchHeadSHA: "1111111111111111111111111111111111111111",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: invoking `yeet release --dry-run` inside the stale checkout
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the run fails telling the user to update the checkout
		testastic.NotEqual(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/stale_checkout_fails_with_an_actionable_error/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("stale checkout is rejected before remote mutation", func(t *testing.T) {
		t.Parallel()

		// given: a non-dry release with a merged PR ready to finalize but a stale checkout
		repoDir, boundarySHA, _ := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                "acme",
			Repo:                 "repo",
			LatestTag:            "v1.0.0",
			BoundarySHA:          boundarySHA,
			BranchHeadSHA:        "1111111111111111111111111111111111111111",
			MergedPendingRelease: true,
			FailOnMutation:       true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: invoking a release that would otherwise finalize the merged PR
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: checkout validation fails before any provider mutation
		testastic.NotEqual(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/stale_checkout_is_rejected_before_remote_mutation/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("remote tag target overrides stale local tag", func(t *testing.T) {
		t.Parallel()

		// given: HEAD matches the provider while the local release tag points to a different commit
		repoDir, _, headSHA := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   headSHA,
			BranchHeadSHA: headSHA,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: invoking release analysis with the mismatched local tag
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the provider target remains authoritative
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("missing repository fails with an actionable error", func(t *testing.T) {
		t.Parallel()

		// given: no git repository in the working directory
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "acme",
			Repo:        "repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "2222222222222222222222222222222222222222",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "acme",
			Repo:     "repo",
		})

		// when: invoking `yeet release --dry-run` outside any checkout
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(t.TempDir()),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the run fails asking for a full checkout
		testastic.NotEqual(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/missing_repository_fails_with_an_actionable_error/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})
}
