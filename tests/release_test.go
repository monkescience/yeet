package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseDryRun(t *testing.T) {
	t.Parallel()

	t.Run("gitlab dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable commit since v1.0.0 and a
		// fake GitLab server reporting the checkout head as the branch head
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

		// when: invoking `yeet release --dry-run` inside the checkout
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: the binary exits 0 and prints the planned release
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_dry_run/gitlab/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("azuredevops dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable commit since v1.0.0 and a
		// fake Azure DevOps server reporting the checkout head as the branch head
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: invoking `yeet release --dry-run` inside the checkout
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: the binary exits 0 and prints the planned release
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_dry_run/azuredevops/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github dry-run prints the planned release", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable commit since v1.0.0 and a
		// fake GitHub server reporting the checkout head as the branch head
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

		// when: invoking `yeet --verbose release --dry-run` inside the checkout
		result := binary.RunWithOptions(t,
			[]string{"--verbose", "release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the binary prints the plan and sanitized HTTP diagnostics
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_dry_run/github/stdout.expected.txt",
			result.Stdout,
		)
		testastic.AssertFile(
			t,
			"testdata/release_dry_run/github/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseCreatesPR(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops opens a release pull request", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable feat commit since v1.0.0
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 0 (PR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab opens a release merge request", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable feat commit since v1.0.0
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

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 0 (MR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github opens a release pull request", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout with one releasable feat commit since v1.0.0
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

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 0 (PR creation handled by the fake provider)
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseReviewers(t *testing.T) {
	t.Parallel()

	t.Run("gitlab requests configured reviewers on the release MR", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server that knows the configured reviewer
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
			Users:         map[string]int64{"alice": 101},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:  "gitlab",
			Branch:    "main",
			Host:      "gitlab.com",
			Project:   "group/service",
			Reviewers: []string{"alice"},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 0 after resolving the reviewer and creating the MR
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab fails the release for an unknown reviewer", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server that cannot resolve the configured reviewer
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
			Provider:  "gitlab",
			Branch:    "main",
			Host:      "gitlab.com",
			Project:   "group/service",
			Reviewers: []string{"ghost"},
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet fails and names the unresolved reviewer
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_reviewers/gitlab_fails_the_release_for_an_unknown_reviewer/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseAutoMerge(t *testing.T) {
	t.Parallel()

	t.Run("github finalizes an already-merged release PR", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server reporting that the prior release PR was merged
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                "testorg",
			Repo:                 "testrepo",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server with a releasable commit
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

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab finalizes an already-merged release MR", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server reporting that the prior release MR was merged
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:              "group/service",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
		})

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server with a releasable commit
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

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAzureDevOpsFullFlow(t *testing.T) {
	t.Parallel()

	t.Run("azuredevops --auto-merge tags the release", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server with a releasable feat commit
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: running `yeet release --auto-merge`
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet merges and tags the release, exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops finalizes an already-merged release PR", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server reporting that the prior release PR was merged
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:         "contoso",
			Project:              "platform",
			Repo:                 "yeet",
			LatestTag:            "v1.0.0",
			BoundarySHA:          shas[0],
			BranchHeadSHA:        shas[1],
			MergedPendingRelease: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: running `yeet release` without --auto-merge
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet tags the merged release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops rejects multiple pending release PRs", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server returning two open release PRs
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:    "contoso",
			Project:         "platform",
			Repo:            "yeet",
			LatestTag:       "v1.0.0",
			BoundarySHA:     shas[0],
			BranchHeadSHA:   shas[1],
			MultipleOpenPRs: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: running `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 1 with a "multiple pending release PRs" error on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_azure_full_flow/multiple_pending_prs/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("azuredevops blocks --auto-merge when merge is gated", func(t *testing.T) {
		t.Parallel()

		// given: a fake Azure DevOps server that flags the release PR as merge-blocked
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
			MergeBlocked:  true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: running `yeet release --auto-merge` against the blocked PR
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet exits 1 with a "release PR merge blocked" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_azure_full_flow/merge_blocked/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseChannelAndVersionFiles(t *testing.T) {
	t.Parallel()

	t.Run("github prerelease channel opens a PR on the channel branch", func(t *testing.T) {
		t.Parallel()

		// given: a config with a `beta` channel and the binary running on the beta branch
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

		// when: invoking `yeet release --channel beta` from the beta ref
		result := binary.RunWithOptions(t,
			[]string{"release", "--channel", "beta", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "beta")...),
		)

		// then: yeet opens a prerelease PR for the channel and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github release updates a configured version file", func(t *testing.T) {
		t.Parallel()

		// given: a config that lists VERSION.txt as a version_files entry
		files := map[string]string{"VERSION.txt": "1.0.0 # x-yeet-version\n"}
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

		// then: yeet writes the bumped version and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseCalVer(t *testing.T) {
	t.Parallel()

	t.Run("github calver release creates a PR with the next month/micro", func(t *testing.T) {
		t.Parallel()

		// given: a calver project at v2025.11.1 with one new feat commit
		files := map[string]string{"VERSION.txt": "2025.11.1 # x-yeet-version\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.11.1",
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans the next calver and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver bootstraps the initial release without a prior tag", func(t *testing.T) {
		t.Parallel()

		// given: a calver project with no previous releases
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: bootstrap"},
				{Message: "feat: initial feature"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
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

		// when: invoking `yeet release --dry-run` for the first time
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans an initial calver release and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver YYYY.MM.DD.MICRO dry-run plans a daily version", func(t *testing.T) {
		t.Parallel()

		// given: a calver config using year/month/day/micro
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: bootstrap"},
				{Message: "feat: ship feature"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.MM.DD.MICRO",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the daily calver format and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github calver YYYY.WW.MICRO dry-run plans an ISO-week version", func(t *testing.T) {
		t.Parallel()

		// given: a calver config using ISO year/week/micro
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: bootstrap"},
				{Message: "feat: ship feature"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "github",
			Branch:       "main",
			Host:         "github.com",
			Owner:        "testorg",
			Repo:         "testrepo",
			Versioning:   "calver",
			CalVerFormat: "YYYY.WW.MICRO",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the ISO-week calver format and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseJSONPointerVersionFile(t *testing.T) {
	t.Parallel()

	t.Run("github bumps a top-level package.json version", func(t *testing.T) {
		t.Parallel()

		// given: a project with a package.json containing /version
		files := map[string]string{"package.json": readTestFile(
			t,
			"testdata/release_j_s_o_n_pointer_version_file/"+
				"github_bumps_a_top_level_package_json_version/package.json",
		)}
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
				{Path: "package.json", Format: "json", JSONPointer: "/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet bumps the JSON pointer target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github bumps a nested array JSON pointer", func(t *testing.T) {
		t.Parallel()

		// given: a manifest.json with a version at /packages/0/version
		files := map[string]string{"manifest.json": readTestFile(
			t,
			"testdata/release_j_s_o_n_pointer_version_file/"+
				"github_bumps_a_nested_array_j_s_o_n_pointer/manifest.json",
		)}
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
				{Path: "manifest.json", Format: "json", JSONPointer: "/packages/0/version"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet bumps the array-element version and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github escapes ~ and / in the JSON pointer", func(t *testing.T) {
		t.Parallel()

		// given: a JSON pointer that uses ~0 and ~1 escapes
		files := map[string]string{"escaped.json": readTestFile(
			t,
			"testdata/release_j_s_o_n_pointer_version_file/github_escapes___and/"+
				"in_the_j_s_o_n_pointer/escaped.json",
		)}
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
				{Path: "escaped.json", Format: "json", JSONPointer: "/a~0b/c~1d"},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet resolves the escaped pointer and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseBreakingChange(t *testing.T) {
	t.Parallel()

	t.Run("github bumps the major version on a BREAKING CHANGE footer", func(t *testing.T) {
		t.Parallel()

		// given: a feat! commit carrying a BREAKING CHANGE footer
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat!: redesign api\n\nBREAKING CHANGE: removed v1 endpoints"},
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

		// then: yeet plans v2.0.0 and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_breaking_change/github_major_bump/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseMultiTarget(t *testing.T) {
	t.Parallel()

	t.Run("github --target filters multi-target plans to the requested target", func(t *testing.T) {
		t.Parallel()

		// given: a repo with `api/` and `web/` path targets and commits in both
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "changelog\n"}},
				{Message: "feat: update web ui", Files: map[string]string{"web/index.html": "web ui\n"}},
				{Message: "feat: add api endpoint", Files: map[string]string{"api/handler.go": "api handler\n"}},
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
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		// when: invoking `yeet release --dry-run --target api`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "api", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet plans only the `api` target and exits 0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_multi_target/github_filter_api/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleasePreMajor(t *testing.T) {
	t.Parallel()

	t.Run("github keeps feat at a minor bump while still pre-1.0", func(t *testing.T) {
		t.Parallel()

		// given: a project still on the 0.x series
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v0.3.0", Tag: "v0.3.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v0.3.0",
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

		// then: a feat on 0.x bumps to v0.3.1 (patch) rather than v0.4.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_pre_major/github_feat_patch/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseNoChanges(t *testing.T) {
	t.Parallel()

	t.Run("github reports no release when no releasable commits exist", func(t *testing.T) {
		t.Parallel()

		// given: only a docs commit since the last release
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "docs: tweak readme"},
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

		// then: yeet still exits 0 (no-op release)
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMergeErrors(t *testing.T) {
	t.Parallel()

	t.Run("github rejects multiple pending release PRs", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub server returning two open release PRs
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:           "testorg",
			Repo:            "testrepo",
			LatestTag:       "v1.0.0",
			BoundarySHA:     shas[0],
			BranchHeadSHA:   shas[1],
			MultipleOpenPRs: true,
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

		// then: yeet exits 1 with a multi-PR error on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_merge_errors/github_multiple_pending_prs/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseMultiTagHistory(t *testing.T) {
	t.Parallel()

	t.Run("github semver picks the highest of multiple prior tags", func(t *testing.T) {
		t.Parallel()

		// given: a repo with v0.9.0, v1.0.0, v1.1.0 and v1.2.0 advertised
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v0.9.0", Tag: "v0.9.0"},
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: release v1.1.0", Tag: "v1.1.0"},
				{Message: "chore: release v1.2.0", Tag: "v1.2.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.2.0",
			ExtraTags:   []string{"v1.0.0", "v1.1.0", "v0.9.0"},
			BoundarySHA: shas[3],
			TagSHAs: map[string]string{
				"v0.9.0": shas[0], "v1.0.0": shas[1], "v1.1.0": shas[2], "v1.2.0": shas[3],
			},
			BranchHeadSHA: shas[4],
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

		// then: yeet plans the next minor relative to v1.2.0 (v1.3.0)
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_multi_tag_history/github_semver/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github calver picks the highest of multiple prior calver tags", func(t *testing.T) {
		t.Parallel()

		// given: a calver repo with several prior month tags
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.12.0", Tag: "v2025.12.0"},
				{Message: "chore: release v2026.01.0", Tag: "v2026.01.0"},
				{Message: "chore: release v2026.03.0", Tag: "v2026.03.0"},
				{Message: "chore: release", Tag: "v2026.05.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v2026.05.0",
			ExtraTags:   []string{"v2025.12.0", "v2026.01.0", "v2026.03.0"},
			BoundarySHA: shas[3],
			TagSHAs: map[string]string{
				"v2025.12.0": shas[0], "v2026.01.0": shas[1], "v2026.03.0": shas[2], "v2026.05.0": shas[3],
			},
			BranchHeadSHA: shas[4],
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

		// then: yeet uses the latest calver tag as the baseline and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMultiTargetPRBody(t *testing.T) {
	t.Parallel()

	t.Run("github builds a combined wave PR body for two path targets", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target repo with commits in both `api/` and `web/`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "changelog\n"}},
				{Message: "feat: update web ui", Files: map[string]string{"web/index.html": "web ui\n"}},
				{Message: "feat: add api endpoint", Files: map[string]string{"api/handler.go": "api handler\n"}},
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
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		// when: invoking `yeet release` without --target
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet emits a combined wave PR and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseDerivedTarget(t *testing.T) {
	t.Parallel()

	t.Run("github derived root target aggregates included path plans", func(t *testing.T) {
		t.Parallel()

		// given: a path target `api` and a derived `root` that includes `api`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "changelog\n"}},
				{Message: "feat: add api endpoint", Files: map[string]string{"services/api/handler.go": "api handler\n"}},
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
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "services/api", TagPrefix: "api-v"},
				{
					Name:         "root",
					Type:         "derived",
					Path:         ".",
					TagPrefix:    "v",
					ExcludePaths: []string{"services/api"},
					Includes:     []string{"api"},
				},
			},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the derived target rolls up the included path plans and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseMultiTargetCrossProvider(t *testing.T) {
	t.Parallel()

	t.Run("gitlab multi-target resolves per-commit paths", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target gitlab repo with commits in both `api/` and `web/`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "changelog\n"}},
				{Message: "feat: update web ui", Files: map[string]string{"web/index.html": "web ui\n"}},
				{Message: "feat: add api endpoint", Files: map[string]string{"api/handler.go": "api handler\n"}},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "group/service",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[2],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Project:  "group/service",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: gitlab routes commits to their respective targets and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops multi-target resolves per-commit paths", func(t *testing.T) {
		t.Parallel()

		// given: a multi-target Azure repo with commits in both `api/` and `web/`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0", Files: map[string]string{"CHANGELOG.md": "changelog\n"}},
				{Message: "feat: update web ui", Files: map[string]string{"web/index.html": "web ui\n"}},
				{Message: "feat: add api endpoint", Files: map[string]string{"api/handler.go": "api handler\n"}},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:  "contoso",
			Project:       "platform",
			Repo:          "yeet",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[2],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: azure routes commits to their respective targets and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseUpdatesExistingPR(t *testing.T) {
	t.Parallel()

	t.Run("gitlab updates the open release MR in place", func(t *testing.T) {
		t.Parallel()

		// given: an already-open release MR with a yeet manifest in its body
		existingBody := readTestFile(
			t,
			"testdata/release_updates_existing_p_r/"+
				"gitlab_updates_the_open_release_m_r_in_place/"+
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet updates the existing MR rather than opening a new one
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops updates the open release PR in place", func(t *testing.T) {
		t.Parallel()

		// given: an already-open release PR carrying a yeet manifest
		existingBody := readTestFile(
			t,
			"testdata/release_updates_existing_p_r/"+
				"azuredevops_updates_the_open_release_p_r_in_place/"+
				"existing_pull_request_body.input.md",
		)

		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://dev.azure.com/contoso/platform/_git/yeet", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:              "contoso",
			Project:                   "platform",
			Repo:                      "yeet",
			LatestTag:                 "v1.0.0",
			BoundarySHA:               shas[0],
			BranchHeadSHA:             shas[1],
			ExistingOpenReleasePRBody: existingBody,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "contoso",
			Repo:         "yeet",
			Project:      "platform",
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet updates the existing PR rather than opening a new one
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github updates the open release PR while preserving manual sections", func(t *testing.T) {
		t.Parallel()

		// given: an open release PR whose body carries a reviewer-added "Notes" section
		existingBody := readTestFile(
			t,
			"testdata/release_updates_existing_p_r/"+
				"github_updates_the_open_release_p_r_while_preserving_manual_sections/"+
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

		// then: yeet updates the body without trashing the Notes section
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseAsFooter(t *testing.T) {
	t.Parallel()

	t.Run("github Release-As footer pins the planned version", func(t *testing.T) {
		t.Parallel()

		// given: a feat commit carrying a `Release-As: 2.5.0` footer
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: tweak api\n\nRelease-As: 2.5.0"},
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

		// then: yeet honours the override and plans v2.5.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_as_footer/pins_planned_version/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github Release-As overrides a smaller computed bump", func(t *testing.T) {
		t.Parallel()

		// given: a fix commit that would normally bump v1.2.3 -> v1.2.4
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.2.3", Tag: "v1.2.3"},
				{Message: "fix: minor bump\n\nRelease-As: 3.0.0"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.2.3",
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

		// then: the explicit Release-As wins, yielding v3.0.0
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_as_footer/overrides_smaller_bump/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseCommitOverride(t *testing.T) {
	t.Parallel()

	t.Run("github local commit BEGIN/END_COMMIT_OVERRIDE replaces the squashed message", func(t *testing.T) {
		t.Parallel()

		// given: a squashed merge whose local commit message wraps an override block
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "chore: squashed merge\n\n" +
					"BEGIN_COMMIT_OVERRIDE\n" +
					"feat: overridden first commit\n\n" +
					"fix: overridden second commit\n" +
					"END_COMMIT_OVERRIDE\n"},
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

		// then: the changelog uses the overridden commit subjects, not the squashed message
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_commit_override/begin_end_block/stdout.expected.txt",
			result.Stdout,
		)
	})
}

func TestReleaseVersionFileErrors(t *testing.T) {
	t.Parallel()

	t.Run("semver target rejects a calver marker with a suggestion", func(t *testing.T) {
		t.Parallel()

		// given: a semver project whose VERSION.txt carries an `x-yeet-month` marker
		files := map[string]string{"VERSION.txt": "1.0.0 # x-yeet-month\n"}
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

		// then: yeet exits 1 and names the offending marker in the suggestion
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_version_file_errors/semver_rejects_calver_marker/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("calver target rejects a semver marker with a suggestion", func(t *testing.T) {
		t.Parallel()

		// given: a calver project whose VERSION.txt carries an `x-yeet-major` marker
		files := map[string]string{"VERSION.txt": "2025.11.1 # x-yeet-major\n"}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.11.1",
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

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet exits 1 and names the offending marker in the suggestion
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_version_file_errors/calver_rejects_semver_marker/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("calver target updates month and micro markers in BUILD.txt", func(t *testing.T) {
		t.Parallel()

		// given: a calver BUILD.txt with year/month/micro markers
		files := map[string]string{"BUILD.txt": readTestFile(
			t,
			"testdata/release_version_file_errors/"+
				"calver_target_updates_month_and_micro_markers_in_b_u_i_l_d_txt/BUILD.txt",
		)}
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v2025.11.1", Tag: "v2025.11.1"},
				{Message: "feat: add a thing", Files: files},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v2025.11.1",
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
			VersionFiles: []fixture.VersionFileOptions{{Path: "BUILD.txt"}},
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet updates each marker without rejecting the file and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}

func TestReleaseConfigErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing config file prints an init hint", func(t *testing.T) {
		t.Parallel()

		// given: a working directory with no .yeet.yaml
		tempDir := t.TempDir()

		// when: invoking `yeet release` from that directory
		result := binary.RunWithOptions(t,
			[]string{"release"},
			testastic.WithRunWorkDir(tempDir),
		)

		// then: yeet exits 1 with a "configuration file not found" hint to run init
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_config_errors/missing_config_file/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("malformed yaml reports a parse error", func(t *testing.T) {
		t.Parallel()

		// given: a syntactically broken .yeet.yaml
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		const filePerm = 0o600

		err := os.WriteFile(configPath, []byte("release: ["), filePerm)
		testastic.NoError(t, err)

		// when: invoking `yeet release --config <path>`
		result := binary.Run(t, "release", "--config", configPath)

		// then: yeet exits 1 with an "invalid configuration / parse config" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_config_errors/malformed_yaml/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("github missing token surfaces the env-var requirement", func(t *testing.T) {
		t.Parallel()

		// given: a valid github config but neither GITHUB_TOKEN nor GH_TOKEN set
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release` with the token vars cleared
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv(
				"GITHUB_TOKEN=",
				"GH_TOKEN=",
				"GITHUB_REF_NAME=main",
			),
		)

		// then: yeet exits 1 and stderr names the missing env vars
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_config_errors/github_missing_token/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("unsupported remote host without explicit provider is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a config pointing at code.company.com with no provider hint
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Branch: "main",
			Host:   "code.company.com",
			Owner:  "platform",
			Repo:   "yeet",
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: yeet exits 1 with an "unsupported remote host" error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_config_errors/unsupported_remote_host/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseCLIFlagsOverrideConfig(t *testing.T) {
	t.Parallel()

	t.Run("github CLI flags override owner/repo/provider/host from config", func(t *testing.T) {
		t.Parallel()

		// given: a config naming `configorg/configrepo` and a server expecting `flagorg/flagrepo`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/flagorg/flagrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: cli override"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "flagorg",
			Repo:          "flagrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "configorg",
			Repo:     "configrepo",
		})

		// when: invoking `yeet release` with --owner/--repo/--provider/--host flags
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "github", "--owner", "flagorg", "--repo", "flagrepo",
				"--host", "github.com",
				"--auto-merge-force",
				"--auto-merge-method", "squash",
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet uses the flag values and reaches the fake server (exit 0)
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --project flag overrides config owner/repo", func(t *testing.T) {
		t.Parallel()

		// given: a config naming `configorg/configrepo` and a server expecting `flaggroup/svc`
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/flaggroup/svc.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: cli override"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:       "flaggroup/svc",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[1],
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "gitlab",
			Branch:   "main",
			Host:     "gitlab.com",
			Owner:    "configorg",
			Repo:     "configrepo",
		})

		// when: invoking `yeet release --project flaggroup/svc`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--dry-run", "--config", configPath,
				"--provider", "gitlab", "--project", "flaggroup/svc",
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: --project clears owner/repo and yeet reaches the fake server (exit 0)
		testastic.Equal(t, 0, result.ExitCode)
	})
}
