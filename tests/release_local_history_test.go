package integration_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseLocalHistory(t *testing.T) {
	t.Parallel()

	t.Run("matching checkout serves commit ranges from local git", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout whose head matches the provider branch head,
		// while the provider's compare fixture would tell a different story
		repoDir, headSHA := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "acme",
			Repo:        "repo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
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
		testastic.True(t, strings.Contains(result.Stdout, "1.1.0"))
		testastic.True(t, strings.Contains(result.Stdout, "add local feature"))
		testastic.False(t, strings.Contains(result.Stdout, "remote patch"))
	})

	t.Run("stale checkout fails with an actionable error", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout whose head differs from the provider branch head
		repoDir, _ := fixture.WriteRepoWithTaggedHistory(
			t,
			"https://github.com/acme/repo.git",
			"main",
			"v1.0.0",
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "acme",
			Repo:          "repo",
			LatestTag:     "v1.0.0",
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
		testastic.True(t, strings.Contains(result.Stderr, "does not match the remote head"))
	})

	t.Run("missing repository fails with an actionable error", func(t *testing.T) {
		t.Parallel()

		// given: no git repository in the working directory
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:     "acme",
			Repo:      "repo",
			LatestTag: "v1.0.0",
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
		testastic.True(t, strings.Contains(result.Stderr, "local checkout cannot serve release history"))
	})
}
