package integration_test

import (
	"fmt"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseExistingPRPerProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab MergedPendingRelease + version files", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab fake with both merged-pending-release and version files
		files := map[string]string{"VERSION.txt": "1.0.0 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:              "group/service",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
			Files:                files,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "gitlab",
			Branch:       "main",
			Host:         "gitlab.com",
			Project:      "group/service",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet finalizes the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver auto-merge updates VERSION.txt with markers", func(t *testing.T) {
		t.Parallel()

		// given: a calver project with version_files and a feat commit
		files := map[string]string{"VERSION.txt": "2025.05.0 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release", Tag: "v2025.05.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.05.0",
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
			CalVerFormat: "YYYY.0M.MICRO",
			VersionFiles: []fixture.VersionFileOptions{{Path: "VERSION.txt"}},
		})

		// when: invoking `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet runs the calver auto-merge flow and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

// TestReleaseProviderPagination used to prove yeet followed the provider's
// paginated compare API. That path no longer exists: history comes from the
// local checkout. The test is repurposed to prove the local-history equivalent:
// a repository with more than 100 commits since the tag (past any provider page
// size) still yields a complete changelog.
func TestReleaseProviderPagination(t *testing.T) {
	t.Parallel()

	t.Run("github local history serves more than 100 commits since the tag", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with 105 releasable commits since v1.0.0
		const commitCount = 105

		commits := []fixture.RepoCommit{{Message: "chore: release v1.0.0", Tag: "v1.0.0"}}
		for i := 1; i <= commitCount; i++ {
			commits = append(commits, fixture.RepoCommit{Message: fmt.Sprintf("feat: change %03d", i)})
		}

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main", commits)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[len(shas)-1],
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

		// then: the changelog covers the whole range, including the oldest commit
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_provider_pagination/"+
				"github_local_history_serves_more_than_100_commits_since_the_tag/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseExistingOpenPRUpdate(t *testing.T) {
	t.Parallel()

	t.Run("github updates open release PR while replacing CHANGELOG sections", func(t *testing.T) {
		t.Parallel()

		// given: an open release PR whose body already contains Features but is stale
		existingBody := readTestFile(
			t,
			"testdata/release_existing_open_p_r_update/"+
				"github_updates_open_release_p_r_while_replacing_c_h_a_n_g_e_l_o_g_sections/"+
				"existing_pull_request_body.input.md",
		)

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                     "testorg",
			Repo:                      "testrepo",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			ExistingOpenReleasePRBody: existingBody,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet replaces the stale body and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseFlagsRepositoryOverrides(t *testing.T) {
	t.Parallel()

	t.Run("github --owner alone overrides only the owner coordinate", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server expecting overridden owner + config's repo
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/flagorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "flagorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "configorg",
			Repo:     "testrepo",
		})

		// when: invoking with --owner override but no --repo
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--owner", "flagorg",
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet swaps owner only and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github --remote override from CLI", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server and CLI flag overrides
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

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking with --remote flag
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--remote", "origin",
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet still works and exits 0
		if result.ExitCode != 0 {
			t.Logf("stderr=%s", result.Stderr)
		}

		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseTargetFilterErrors(t *testing.T) {
	t.Parallel()

	t.Run("github rejects --target naming an unknown target", func(t *testing.T) {
		t.Parallel()

		// given: a single-target config and a --target flag naming a non-existent
		// target; the run fails before any history or provider access
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:          "testorg",
			Repo:           "testrepo",
			LatestTag:      "v1.0.0",
			FailOnMutation: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release --target nope`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "nope", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr names the missing target
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_target_filter_errors/github_rejects___target_naming_an_unknown_target/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseGitLabExistingPRUpdate(t *testing.T) {
	t.Parallel()

	t.Run("gitlab updates open release MR with stale body sections", func(t *testing.T) {
		t.Parallel()

		// given: an open release MR with stale body
		existingBody := readTestFile(
			t,
			"testdata/release_git_lab_existing_p_r_update/"+
				"gitlab_updates_open_release_m_r_with_stale_body_sections/"+
				"existing_pull_request_body.input.md",
		)

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                   "group/service",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			ExistingOpenReleasePRBody: existingBody,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		// when: invoking `yeet release` against the open MR
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet replaces stale entries and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAuthSelection(t *testing.T) {
	t.Parallel()

	t.Run("github prefers GH_TOKEN when GITHUB_TOKEN is empty", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout, a fake GitHub server, and GH_TOKEN as the only credential
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

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking with GH_TOKEN set and GITHUB_TOKEN empty
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITHUB_TOKEN=",
				"GH_TOKEN=gh-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet authenticates with GH_TOKEN and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
