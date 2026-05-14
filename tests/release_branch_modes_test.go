package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseBranchAutoChannel(t *testing.T) {
	t.Parallel()

	t.Run("github auto-selects a configured channel from the current branch", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel and the binary running on the beta branch with no --channel flag
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
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
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		// when: invoking `yeet release --dry-run` without --channel from the beta branch
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet auto-selects the beta channel and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_branch_modes/github_auto_beta/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github rejects unknown --channel value", func(t *testing.T) {
		t.Parallel()

		// given: a config with only a beta channel but caller passes --channel rc
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
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
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		// when: invoking `yeet release --channel rc` (rc is not configured)
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--channel", "rc", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet exits 1 with an unknown-channel error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "unknown release channel")
	})

	t.Run("github rejects explicit channel from the wrong branch", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel configured for the beta branch, but the current branch is main
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		// when: invoking `yeet release --channel beta` from main without --dry-run
		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet rejects the branch/channel mismatch before provider work starts
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "must run on branch")
	})

	t.Run("github rejects non-dry-run release from an unconfigured branch", func(t *testing.T) {
		t.Parallel()

		// given: a config without channels and the binary on a non-main branch
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
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
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		// when: invoking `yeet release` from a branch that matches no channel
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "feature/random")...),
		)

		// then: yeet exits 1 with an unconfigured-branch error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "branch is not configured for releases")
	})

	t.Run("github runs --dry-run from an unconfigured branch with no plans", func(t *testing.T) {
		t.Parallel()

		// given: a config without any channel matching `topic`, and --dry-run
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
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
			Channels: map[string]fixture.ChannelOptions{
				"beta": {Branch: "beta", Prerelease: "beta"},
			},
		})

		// when: invoking `yeet release --dry-run` from a topic branch
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "feature/random")...),
		)

		// then: yeet exits 0 (channel cleared) and falls through to the main branch
		testastic.Equal(t, 0, result.ExitCode)
	})
}
