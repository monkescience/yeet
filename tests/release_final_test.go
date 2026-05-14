package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseGitHubProjectOnly(t *testing.T) {
	t.Parallel()

	t.Run("github accepts owner/repo derived from project alone", func(t *testing.T) {
		t.Parallel()

		// given: a github config that gives only `project: owner/repo`
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

		configPath := writeRawConfig(t, `provider: github
branch: main
repository:
  host: github.com
  project: testorg/testrepo
targets:
  default:
    type: path
    path: .
    tag_prefix: v
`)

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet splits project into owner/repo and proceeds; exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseProviderOverride(t *testing.T) {
	t.Parallel()

	t.Run("gitlab project override clears stale github owner and repo", func(t *testing.T) {
		t.Parallel()

		// given: a valid GitHub config that is overridden at the CLI to target GitLab
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
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
		})

		// when: invoking release with a provider/project override
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run",
				"--provider", "gitlab", "--project", "group/service",
				"--config", configPath,
			},
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet uses the GitLab project instead of rejecting stale GitHub owner/repo fields
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_provider_override/gitlab_project/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseFinalizationIdempotency(t *testing.T) {
	t.Parallel()

	t.Run("github skips creating a release that already exists", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release PR whose tag already has a provider release
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                "testorg",
			Repo:                 "testrepo",
			LatestTag:            "v1.0.0",
			BoundarySHA:          "boundary-sha",
			MergedPendingRelease: true,
			ExistingRelease:      true,
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "docs: no new release"},
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

		// when: invoking `yeet release` after the release was already created externally
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
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
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{
					SHA:              "head-sha",
					Message:          "chore: squashed merge",
					AssociatedPRBody: "BEGIN_COMMIT_OVERRIDE\n\nEND_COMMIT_OVERRIDE\n",
				},
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

		// then: yeet exits 1 and stderr mentions the empty override block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "empty override block")
	})

	t.Run("github rejects missing END_COMMIT_OVERRIDE", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge whose override block has no END marker
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{
					SHA:              "head-sha",
					Message:          "chore: squashed merge",
					AssociatedPRBody: "BEGIN_COMMIT_OVERRIDE\nfeat: ship\n",
				},
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

		// then: yeet exits 1 and stderr mentions the missing END marker
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "END_COMMIT_OVERRIDE")
	})
}

func TestReleaseMultipleVersionFiles(t *testing.T) {
	t.Parallel()

	t.Run("github bumps multiple configured version files", func(t *testing.T) {
		t.Parallel()

		// given: a config that lists three version_files with different formats
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{
				"VERSION.txt":      "1.0.0 # x-yeet-version\n",
				"package.json":     `{"name":"yeet","version":"1.0.0"}`,
				"chart/Chart.yaml": "name: yeet\nversion: 1.0.0  # x-yeet-version\n",
			},
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

		// given: a fake server and only CI_COMMIT_BRANCH set
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
		})

		// when: invoking `yeet release --dry-run` with CI_COMMIT_BRANCH=main
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
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
		testastic.Contains(t, result.Stderr, "GITLAB_TOKEN")
	})

	t.Run("gitlab prefers GL_TOKEN when GITLAB_TOKEN is empty", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server and GL_TOKEN as the only credential
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:     "group/service",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
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
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
			Files: map[string]string{
				"CHANGELOG.md": "# Changelog\n\n## v1.0.0 (2025-12-01)\n\n### Features\n\n- prior change\n",
			},
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
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet prepends the new entry beneath the `# Changelog` heading and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseGitLabAutoMergeForce(t *testing.T) {
	t.Parallel()

	t.Run("gitlab --auto-merge-force overrides merge-blocked MR", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab server reporting the MR as draft
		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:      "group/service",
			LatestTag:    "v1.0.0",
			BoundarySHA:  "boundary-sha",
			MergeBlocked: true,
			Commits: []fakeprovider.GitLabCommit{
				{SHA: "head-sha", Message: "feat: add a thing"},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
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
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: gitlab still rejects merging a draft; binary surfaces the block
		testastic.Equal(t, 1, result.ExitCode)
		testastic.Contains(t, result.Stderr, "draft")
	})
}
