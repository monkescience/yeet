package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseUnreachableAncestor(t *testing.T) {
	t.Parallel()

	t.Run("github skips a newer unreachable tag in favor of an older reachable one", func(t *testing.T) {
		t.Parallel()

		// given: v2.0.0 lives on a side branch (unreachable from main) while
		// v1.0.0 is a proper ancestor of the release branch head
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v2.0.0", Tag: "v2.0.0", Branch: "side"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2.0.0",
			ExtraTags:     []string{"v1.0.0"},
			BoundarySHA:   shas[1],
			TagSHAs:       map[string]string{"v2.0.0": shas[1], "v1.0.0": shas[0]},
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

		// then: yeet falls back to v1.0.0 and plans v1.1.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_unreachable_ancestor/"+
				"github_skips_a_newer_unreachable_tag_in_favor_of_an_older_reachable_one/"+
				"stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github surfaces an unreachable boundary as a branch-ancestry error", func(t *testing.T) {
		t.Parallel()

		// given: the only advertised tag exists locally but is not an ancestor
		// of the release branch head
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: setup"},
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Branch: "side"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[1],
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

		// then: yeet exits 1 with a "not reachable" branch ancestry error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_unreachable_ancestor/"+
				"github_surfaces_an_unreachable_boundary_as_a_branch_ancestry_error/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseChannelChangelogFile(t *testing.T) {
	t.Parallel()

	t.Run("github prerelease writes to the channel's changelog_file", func(t *testing.T) {
		t.Parallel()

		// given: a beta channel that points its changelog_file to a separate path
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "beta",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files: map[string]string{
				"CHANGELOG-beta.md": "# Beta Changelog\n",
			},
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release_channel_changelog_file/"+
				"github_prerelease_writes_to_the_channel_s_changelog_file/input.yaml",
		)

		// when: invoking `yeet release --channel beta` on the beta branch
		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet writes to the per-channel changelog and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleasePRBodyHeaderFooter(t *testing.T) {
	t.Parallel()

	t.Run("github uses configured pr_body_header and pr_body_footer", func(t *testing.T) {
		t.Parallel()

		// given: a release config with custom pr_body_header and pr_body_footer values
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release_p_r_body_header_footer/"+
				"github_uses_configured_pr_body_header_and_pr_body_footer/input.yaml",
		)

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet opens the PR with the custom header/footer and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github title templates shape PR and commit subjects", func(t *testing.T) {
		t.Parallel()

		// given: a release config with branch-included PR and commit templates
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release_p_r_body_header_footer/github_title_templates_shape_subjects/"+
				"input.yaml",
		)

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet opens the PR with configured subjects and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
