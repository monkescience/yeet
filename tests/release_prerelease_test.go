package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleasePrereleaseCounter(t *testing.T) {
	t.Parallel()

	t.Run("github beta channel increments existing prerelease counter", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel whose latest tag is already a prerelease
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "beta",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v1.1.0-beta.1", Tag: "v1.1.0-beta.1"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.1.0-beta.1",
			ExtraTags:     []string{"v1.0.0"},
			BoundarySHA:   shas[1],
			TagSHAs:       map[string]string{"v1.1.0-beta.1": shas[1], "v1.0.0": shas[0]},
			BranchHeadSHA: shas[2],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet bumps the counter (beta.1 -> beta.2) and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_increment/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github beta channel opens a major line for a breaking change", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel on a minor prerelease line and a new breaking change
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "beta",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v1.1.0-beta.1", Tag: "v1.1.0-beta.1"},
				{Message: "feat!: drop the old api"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.1.0-beta.1",
			ExtraTags:     []string{"v1.0.0"},
			BoundarySHA:   shas[1],
			TagSHAs:       map[string]string{"v1.1.0-beta.1": shas[1], "v1.0.0": shas[0]},
			BranchHeadSHA: shas[2],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet starts the 2.0.0 prerelease line instead of advancing the 1.1.0 counter
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stdout, "1.1.0-beta.1 → 2.0.0-beta.1")
		testastic.Contains(t, result.Stdout, "Tag         v2.0.0-beta.1")
	})

	t.Run("github beta channel honours Release-As on top of prerelease", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel and a Release-As footer requesting a major bump
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "beta",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: rework api\n\nRelease-As: 2.0.0"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: Release-As pins the stable base and the channel adds the first prerelease tag
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_release_as/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseAsFooterErrors(t *testing.T) {
	t.Parallel()

	t.Run("github rejects conflicting Release-As footers across commits", func(t *testing.T) {
		t.Parallel()

		// given: two feat commits whose Release-As footers disagree
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: second change\n\nRelease-As: 3.0.0"},
				{Message: "feat: first change\n\nRelease-As: 2.0.0"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[2],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with a conflicting Release-As error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/conflicting_values/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects invalid Release-As value", func(t *testing.T) {
		t.Parallel()

		// given: a commit whose Release-As footer is not a valid semver
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: bad pin\n\nRelease-As: not-a-version"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
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
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 with an invalid Release-As error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/invalid_value/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects calver Release-As value in another format", func(t *testing.T) {
		t.Parallel()

		// given: a calver project and a commit whose Release-As footer is a semver version
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.05.3", Tag: "v2025.05.3"},
				{Message: "feat: bad pin\n\nRelease-As: 2.0.0"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.05.3",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.0M.MICRO",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and names the calver format
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr,
			"invalid release version override: requested version does not match the calver format YYYY.0M.MICRO")
		testastic.Contains(t, result.Stderr, "requested=2.0.0 current=2025.05.3")
	})

	t.Run("github reports malformed remote tag metadata", func(t *testing.T) {
		t.Parallel()

		// given: a provider whose tag metadata does not resolve to a commit
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: next release"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			TagSHAs:       map[string]string{"v1.0.0": "not-a-commit"},
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: planning the release
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the malformed metadata is reported instead of being treated as a boundary
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/malformed_remote_tag_metadata/stderr.expected.txt",
			result.Stderr,
		)
	})
}
