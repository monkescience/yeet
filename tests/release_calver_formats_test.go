package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseCalVerFormats(t *testing.T) {
	t.Parallel()

	formats := []struct {
		name   string
		format string
		tag    string
	}{
		{name: "YYYY.0M.MICRO", format: "YYYY.0M.MICRO", tag: "v2025.05.0"},
		{name: "YY.MM.MICRO", format: "YY.MM.MICRO", tag: "v25.5.0"},
		{name: "YYYY.WW.MICRO", format: "YYYY.WW.MICRO", tag: "v2025.20.0"},
		{name: "YYYY.0M.0D.MICRO", format: "YYYY.0M.0D.MICRO", tag: "v2025.05.14.0"},
	}

	for _, tc := range formats {
		t.Run("github calver "+tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver project with the given format
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release", Tag: tc.tag},
					{Message: "feat: add a thing"},
				})

			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner:         "testorg",
				Repo:          "testrepo",
				LatestTag:     tc.tag,
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
				CalVerFormat: tc.format,
			})

			// when: invoking `yeet release --dry-run`
			result := binary.RunWithOptions(t,
				[]string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
			)

			// then: yeet plans the next calver and exits 0
			testastic.Equal(t, 0, result.ExitCode)
		})
	}
}

func TestReleaseCalVerInvalidFormats(t *testing.T) {
	t.Parallel()

	invalidFormats := []struct {
		name         string
		format       string
		expectedFile string
	}{
		{
			name:         "missing year",
			format:       "MM.DD",
			expectedFile: "testdata/release/rejects_missing_year/stderr.expected.txt",
		},
		{
			name:         "duplicate year tokens",
			format:       "YYYY.YYYY.MICRO",
			expectedFile: "testdata/release/rejects_duplicate_year_tokens/stderr.expected.txt",
		},
		{
			name:         "unknown token",
			format:       "YYYY.MM.FOO",
			expectedFile: "testdata/release/rejects_unknown_token/stderr.expected.txt",
		},
	}

	for _, tc := range invalidFormats {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a calver config with an invalid format
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider:     "github",
				Branch:       "main",
				Host:         "github.com",
				Owner:        "testorg",
				Repo:         "testrepo",
				Versioning:   "calver",
				CalVerFormat: tc.format,
			})

			// when: invoking `yeet release --dry-run`
			result := binary.RunWithOptions(t,
				[]string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunEnv(
					"GITHUB_REF_NAME=main",
					"GITHUB_TOKEN=test-token",
					"GITHUB_URL=http://127.0.0.1:1/",
				),
			)

			// then: yeet exits 1 with a calver validation error
			testastic.Equal(t, 1, result.ExitCode)
			testastic.AssertFile(t, tc.expectedFile, result.Stderr)
		})
	}
}

func TestReleaseAsPrereleaseRejected(t *testing.T) {
	t.Parallel()

	t.Run("github rejects Release-As pinning a prerelease version", func(t *testing.T) {
		t.Parallel()

		// given: a feat commit pinning Release-As to a prerelease version
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: ship\n\nRelease-As: 2.0.0-rc.1"},
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

		// then: yeet exits 1 saying the version must be stable
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_rejects_release_as_pinning_a_prerelease_version/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects Release-As that doesn't advance the version", func(t *testing.T) {
		t.Parallel()

		// given: a feat commit pinning Release-As below the current version
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2.0.0", Tag: "v2.0.0"},
				{Message: "feat: ship\n\nRelease-As: 1.0.0"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2.0.0",
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

		// then: yeet exits 1 saying the version must be greater
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_rejects_release_as_that_doesn_t_advance_the_version/stderr.expected.txt",
			result.Stderr,
		)
	})
}
