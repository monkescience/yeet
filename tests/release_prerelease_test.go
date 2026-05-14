package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleasePrereleaseCounter(t *testing.T) {
	t.Parallel()

	t.Run("github beta channel increments existing prerelease counter", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel whose latest tag is already a prerelease
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.1.0-beta.1",
			ExtraTags:   []string{"v1.0.0"},
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.1.0-beta.1"},
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

		// when: invoking `yeet release --dry-run --channel beta`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet bumps the counter (beta.1 -> beta.2) and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_prerelease_counter/github_increment/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github beta channel honours Release-As on top of prerelease", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel and a Release-As footer requesting a major bump
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: rework api\n\nRelease-As: 2.0.0"},
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

		// when: invoking `yeet release --dry-run --channel beta`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--channel", "beta", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: Release-As pins the stable base and the channel adds the first prerelease tag
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_prerelease_counter/github_release_as/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseAsFooterErrors(t *testing.T) {
	t.Parallel()

	t.Run("github rejects conflicting Release-As footers across commits", func(t *testing.T) {
		t.Parallel()

		// given: two feat commits whose Release-As footers disagree
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: first change\n\nRelease-As: 2.0.0"},
				{SHA: "second-sha", Message: "feat: second change\n\nRelease-As: 3.0.0"},
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

		// then: yeet exits 1 with a conflicting Release-As error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_as_footer_errors/conflicting_values/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects invalid Release-As value", func(t *testing.T) {
		t.Parallel()

		// given: a commit whose Release-As footer is not a valid semver
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: bad pin\n\nRelease-As: not-a-version"},
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

		// then: yeet exits 1 with an invalid Release-As error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_as_footer_errors/invalid_value/stderr.expected.txt",
			result.Stderr,
		)
	})
}
