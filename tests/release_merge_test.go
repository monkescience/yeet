package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseAutoMergeMethods(t *testing.T) {
	t.Parallel()

	t.Run("github --auto-merge-method rebase merges via rebase", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout and a fake GitHub server with a releasable commit
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

		// when: invoking `yeet release --auto-merge --auto-merge-method rebase`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "rebase",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet picks the rebase strategy and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github --auto-merge-method merge picks the merge-commit strategy", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout and a fake GitHub server with a releasable commit
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

		// when: invoking `yeet release --auto-merge --auto-merge-method merge`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "merge",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: yeet picks the merge-commit strategy and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --auto-merge-method rebase reports incompatible project setting", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab project whose merge_method is "merge" (not "rebase_merge")
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

		// when: invoking `yeet release --auto-merge --auto-merge-method rebase`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "rebase",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 1 and stderr names the incompatible merge_method
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_auto_merge_methods/"+
				"gitlab___auto_merge_method_rebase_reports_incompatible_project_setting/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab --auto-merge-method squash succeeds when project allows squash", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab project whose squash_option permits squash
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

		// when: invoking `yeet release --auto-merge --auto-merge-method squash`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "squash",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet picks squash and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab --auto-merge-method merge succeeds for plain merge project", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab project whose merge_method is "merge"
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

		// when: invoking `yeet release --auto-merge --auto-merge-method merge`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "merge",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet picks merge and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("gitlab rejects multiple pending release MRs", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server returning two open release MRs
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:         "group/service",
			LatestTag:       "v1.0.0",
			BoundarySHA:     shas[0],
			BranchHeadSHA:   shas[1],
			MultipleOpenPRs: true,
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

		// then: yeet exits 1 with the multi-MR error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_auto_merge_methods/gitlab_rejects_multiple_pending_release_m_rs/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab blocks --auto-merge when MR is gated", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitLab server reporting a merge-blocked MR
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

		// when: invoking `yeet release --auto-merge` against the gated MR
		result := binary.RunWithOptions(t,
			[]string{"release", "--auto-merge", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitLabEnv(server, "main")...),
		)

		// then: yeet exits 1 with a merge-blocked error
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_auto_merge_methods/gitlab_blocks___auto_merge_when_m_r_is_gated/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("azuredevops --auto-merge-method rebase merges via rebase strategy", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout and a fake Azure server with a releasable commit
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

		// when: invoking `yeet release --auto-merge --auto-merge-method rebase`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "rebase",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet picks Azure's rebase strategy and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops --auto-merge-method merge merges via no-ff strategy", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout and a fake Azure server with a releasable commit
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

		// when: invoking `yeet release --auto-merge --auto-merge-method merge`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "merge",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet picks Azure's NoFastForward strategy and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("azuredevops --auto-merge-method squash merges via squash", func(t *testing.T) {
		t.Parallel()

		// given: a local checkout and a fake Azure server with a releasable commit
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

		// when: invoking `yeet release --auto-merge --auto-merge-method squash`
		result := binary.RunWithOptions(t,
			[]string{
				"release", "--auto-merge", "--auto-merge-method", "squash",
				"--config", configPath,
			},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet picks Azure's squash strategy and exits 0
		testastic.Equal(t, 0, result.ExitCode)
	})
}
