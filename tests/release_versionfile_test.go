package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

// versionFileHistory writes the shared checkout used by version-file tests:
// one tagged v1.0.0 boundary and one releasable feat commit.
func versionFileHistory(t *testing.T) (string, []string) {
	t.Helper()

	return fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: add a thing"},
		})
}

func TestReleaseVersionFileBlocks(t *testing.T) {
	t.Parallel()

	t.Run("github replaces semver inside an x-yeet-start/end block", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that wraps prose in an x-yeet-start-version block
		fileContent := "# x-yeet-start-version\n" +
			"This release is 1.0.0 and supersedes the previous one.\n" +
			"# x-yeet-end\n"

		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         map[string]string{"VERSION.txt": fileContent},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet substitutes the version inside the block and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github rejects an unclosed x-yeet-start block", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt whose x-yeet-start block has no matching x-yeet-end
		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files: map[string]string{
				"VERSION.txt": "# x-yeet-start-version\nThis is 1.0.0 and the block is never closed.\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr names the unclosed block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "unclosed x-yeet-start block")
	})

	t.Run("github rejects nested x-yeet-start markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that opens a second block inside an already-open one
		fileContent := "# x-yeet-start-version\n" +
			"version 1.0.0\n" +
			"# x-yeet-start-major\n" +
			"# x-yeet-end\n"

		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         map[string]string{"VERSION.txt": fileContent},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr names the nested block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "nested x-yeet-start")
	})

	t.Run("github rejects a file that has no yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt with no markers at all
		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         map[string]string{"VERSION.txt": "1.0.0\n"},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr names the missing markers
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "no yeet markers")
	})
}

func TestReleaseVersionFileSemverScopes(t *testing.T) {
	t.Parallel()

	t.Run("github updates major, minor and patch inline markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that breaks the version across major/minor/patch markers
		fileContent := "major: 1  # x-yeet-major\n" +
			"minor: 0  # x-yeet-minor\n" +
			"patch: 0  # x-yeet-patch\n"

		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         map[string]string{"VERSION.txt": fileContent},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet substitutes each scope and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github surfaces semver suggestion for a calver-only marker", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that uses `x-yeet-year` under a semver scheme
		repoDir, shas := versionFileHistory(t)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files: map[string]string{
				"VERSION.txt": "year: 2026  # x-yeet-year\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr suggests x-yeet-major
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, `use "x-yeet-major"`)
	})
}

func TestReleaseCalVerMarkerSuggestions(t *testing.T) {
	t.Parallel()

	t.Run("week format suggests x-yeet-week for x-yeet-minor", func(t *testing.T) {
		t.Parallel()

		// given: a calver config with a week-based format and an x-yeet-minor marker
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release", Tag: "v2026.05.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2026.05.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files: map[string]string{
				"VERSION.txt": "minor: 5  # x-yeet-minor\n",
			},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.WW.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet rejects the marker and suggests x-yeet-week
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, `use "x-yeet-week"`)
	})

	t.Run("day-aware format substitutes the x-yeet-day marker", func(t *testing.T) {
		t.Parallel()

		// given: a calver format that exposes year/month/day/micro markers
		fileContent := "year: 2026  # x-yeet-year\n" +
			"month: 05  # x-yeet-month\n" +
			"day: 14    # x-yeet-day\n" +
			"micro: 0   # x-yeet-micro\n"

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: bootstrap"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			BranchHeadSHA: shas[1],
			Files:         map[string]string{"VERSION.txt": fileContent},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.MM.DD.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves day-aware values and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
