package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseGitHubProjectOnly(t *testing.T) {
	t.Parallel()

	t.Run("github accepts owner/repo derived from project alone", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable commit and a github config
		// that gives only `project: owner/repo`
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
			"testdata/release/github_accepts_owner/"+
				"repo_derived_from_project_alone/input.yaml",
		)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet splits project into owner/repo and proceeds, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseProviderOverride(t *testing.T) {
	t.Parallel()

	t.Run("gitlab project override clears stale github owner and repo", func(t *testing.T) {
		t.Parallel()

		// given: a valid GitHub config that is overridden at the CLI to target GitLab
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
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

		// when: invoking release with a provider/project override
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run",
				"--provider", "gitlab", "--project", "group/service",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet uses the GitLab project instead of rejecting stale GitHub owner/repo fields
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_project/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseFinalizationIdempotency(t *testing.T) {
	t.Parallel()

	t.Run("github skips creating a release that already exists", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR whose tag already has a provider release
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "docs: no new release"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                "testorg",
			Repo:                 "testrepo",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
			ExistingRelease:      true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release` after the release was already created externally
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet treats finalization as idempotent and exits successfully
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseCommitOverrideEmptyBlock(t *testing.T) {
	t.Parallel()

	t.Run("github rejects an empty commit override block", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge with an empty BEGIN/END_COMMIT_OVERRIDE block
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: squashed merge\n\nBEGIN_COMMIT_OVERRIDE\n\nEND_COMMIT_OVERRIDE\n"},
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

		// then: yeet exits 1 and stderr mentions the empty override block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_rejects_an_empty_commit_override_block/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github rejects missing END_COMMIT_OVERRIDE", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge whose override block has no END marker
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: squashed merge\n\nBEGIN_COMMIT_OVERRIDE\nfeat: ship\n"},
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

		// then: yeet exits 1 and stderr mentions the missing END marker
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"github_rejects_missing_e_n_d_c_o_m_m_i_t_o_v_e_r_r_i_d_e/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseMultipleVersionFiles(t *testing.T) {
	t.Parallel()

	t.Run("github bumps multiple configured version files", func(t *testing.T) {
		t.Parallel()

		// given: a config that lists three version_files with different formats
		files := map[string]string{
			"VERSION.txt": readTestFile(
				t,
				"testdata/release/"+
					"github_bumps_multiple_configured_version_files/VERSION.txt",
			),
			"package.json": readTestFile(
				t,
				"testdata/release/"+
					"github_bumps_multiple_configured_version_files/package.json",
			),
			"chart/Chart.yaml": readTestFile(
				t,
				"testdata/release/"+
					"github_bumps_multiple_configured_version_files/chart/Chart.yaml",
			),
		}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			Files:         files,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			VersionFiles: []fixture.VersionFileOptions{
				{Path: "VERSION.txt"},
				{Path: "package.json", Format: "json", JSONPointer: "/version"},
				{Path: "chart/Chart.yaml"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet updates all three files and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseEnvOnlyBranch(t *testing.T) {
	t.Parallel()

	t.Run("github uses CI_COMMIT_BRANCH when GITHUB_REF_NAME is empty", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout, a fake server, and only CI_COMMIT_BRANCH set
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

		// when: invoking `yeet release --dry-run` with CI_COMMIT_BRANCH=main
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITHUB_TOKEN=test-token",
				"GITHUB_URL="+server.URL+"/api/v3/",
				"GITHUB_REF_NAME=",
				"CI_COMMIT_BRANCH=main",
			),
		)

		// then: yeet picks up the CI branch env and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMissingProviderTokens(t *testing.T) {
	t.Parallel()

	t.Run("gitlab missing token surfaces env-var requirement", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab config with no GITLAB_TOKEN/GL_TOKEN set
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		// when: invoking `yeet release --config <path>` with both tokens cleared
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITLAB_TOKEN=",
				"GL_TOKEN=",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet exits 1 and stderr names the missing env vars
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"gitlab_missing_token_surfaces_env_var_requirement/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab prefers GL_TOKEN when GITLAB_TOKEN is empty", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout, a fake GitLab server, and GL_TOKEN as the only credential
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		// when: invoking with GL_TOKEN set and GITLAB_TOKEN empty
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(
				"GITLAB_TOKEN=",
				"GL_TOKEN=gl-token",
				"GITLAB_URL="+server.URL+"/api/v4",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet authenticates with GL_TOKEN and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseChangelogPrepend(t *testing.T) {
	t.Parallel()

	t.Run("github prepends a new entry to a `# Changelog`-headed file", func(t *testing.T) {
		t.Parallel()

		// given: a CHANGELOG.md whose first line is `# Changelog` (not a level-2 heading)
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
			Files: map[string]string{"CHANGELOG.md": readTestFile(
				t,
				"testdata/release/"+
					"github_prepends_a_new_entry_to_a____changelog__headed_file/CHANGELOG.md",
			)},
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

		// then: yeet prepends the new entry beneath the `# Changelog` heading and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseGitLabAutoMergeForce(t *testing.T) {
	t.Parallel()

	t.Run("gitlab --auto-merge-force does not bypass draft state", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab server reporting the MR as draft
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			MergeBlocked:  true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		// when: invoking `yeet release --auto-merge --auto-merge-force` against a draft MR
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-force",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: gitlab still rejects merging a draft and the binary surfaces the block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/"+
				"gitlab___auto_merge_force_does_not_bypass_draft_state/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseGitLabAutoMergeWaitsForAsynchronousAccept(t *testing.T) {
	t.Parallel()

	// given: GitLab accepts the release MR before its asynchronous merge has completed
	repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: add a thing"},
		})

	server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
		Project:           "group/service",
		LatestTag:         "v1.0.0",
		BoundarySHA:       shas[0],
		BranchHeadSHA:     shas[1],
		AsynchronousMerge: true,
	})

	configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
		Provider: "gitlab",
		Branch:   "main",
		Host:     "gitlab.com",
		Project:  "group/service",
	})

	// when: invoking `yeet release --auto-merge`
	result := binary.RunWithOptions(t,
		[]string{"release", "--auto-merge", "--config", configPath},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
	)

	// then: yeet waits for the merged MR before creating the release
	testastic.Equal(t, 0, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)
}

func TestReleaseGitLabFinalizesFastForwardMerge(t *testing.T) {
	t.Parallel()

	// given: a merged release MR whose only final commit SHA is its source tip
	files := map[string]string{
		"CHANGELOG.md": "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n",
	}
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
		FastForwardMerge:     true,
		Files:                files,
	})

	configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
		Provider: "gitlab",
		Branch:   "main",
		Host:     "gitlab.com",
		Project:  "group/service",
	})

	// when: invoking `yeet release`
	result := binary.RunWithOptions(t,
		[]string{"release", "--config", configPath},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
	)

	// then: yeet creates the release at the fast-forwarded source commit
	testastic.Equal(t, 0, result.ExitCode)
	testastic.Equal(t, "", result.Stdout)
}
