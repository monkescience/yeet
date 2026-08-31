package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func versionFileHistory(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()

	return fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: add a thing", Files: files},
		})
}

func TestReleaseVersionFileBlocks(t *testing.T) {
	t.Parallel()

	t.Run("github updates an existing file after a truncated tree response", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt and a GitHub API that truncates the recursive base tree
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_replaces_semver_inside_an_x_yeet_start/end_block/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                 "testorg",
			Repo:                  "testrepo",
			LatestTag:             "v1.0.0",
			BoundarySHA:           shas[0],
			BranchHeadSHA:         shas[1],
			Files:                 files,
			TruncateRecursiveTree: true,
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

		// then: the release update succeeds without writing to stdout
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)
	})

	t.Run("github replaces semver inside an x-yeet-start/end block", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that wraps prose in an x-yeet-start-version block
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_replaces_semver_inside_an_x_yeet_start/end_block/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_rejects_an_unclosed_x_yeet_start_block/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		testastic.AssertFile(
			t,
			"testdata/release/github_rejects_an_unclosed_x_yeet_start_block/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects nested x-yeet-start markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that opens a second block inside an already-open one
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_rejects_nested_x_yeet_start_markers/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		testastic.AssertFile(
			t,
			"testdata/release/github_rejects_nested_x_yeet_start_markers/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects a file that has no yeet markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt with no markers at all
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_rejects_a_file_that_has_no_yeet_markers/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		testastic.AssertFile(
			t,
			"testdata/release/github_rejects_a_file_that_has_no_yeet_markers/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseVersionFileSemverScopes(t *testing.T) {
	t.Parallel()

	t.Run("github updates major, minor and patch inline markers", func(t *testing.T) {
		t.Parallel()

		// given: a VERSION.txt that breaks the version across major/minor/patch markers
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_updates_major__minor_and_patch_inline_markers/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"github_surfaces_semver_suggestion_for_a_calver_only_marker/VERSION.txt",
		)}
		repoDir, shas := versionFileHistory(t, files)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_surfaces_semver_suggestion_for_a_calver_only_marker/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseCalVerMarkerSuggestions(t *testing.T) {
	t.Parallel()

	t.Run("week format suggests x-yeet-week for x-yeet-minor", func(t *testing.T) {
		t.Parallel()

		// given: a calver config with a week-based format and an x-yeet-minor marker
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"week_format_suggests_x_yeet_week_for_x_yeet_minor/VERSION.txt",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release", Tag: "v2026.05.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2026.05.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
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
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"week_format_suggests_x_yeet_week_for_x_yeet_minor/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("day-aware format substitutes the x-yeet-day marker", func(t *testing.T) {
		t.Parallel()

		// given: a calver format that exposes year/month/day/micro markers
		files := map[string]string{"VERSION.txt": readTestFile(
			t,
			"testdata/release/"+
				"day_aware_format_substitutes_the_x_yeet_day_marker/VERSION.txt",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: bootstrap"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			BranchHeadSHA: shas[1],
			Files:         files,
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
