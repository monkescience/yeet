package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseUnlabeledOpenPR(t *testing.T) {
	t.Parallel()

	t.Run("gitlab adopts a release MR that lost its pending label", func(t *testing.T) {
		t.Parallel()

		// given: an open release MR on the release branch carrying no labels at all
		existingBody := readTestFile(
			t,
			"testdata/release/gitlab_adopts_unlabeled_release_m_r/"+
				"existing_merge_request_body.input.md",
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
			UnlabeledOpenReleaseMR:    true,
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

		// then: yeet relabels the interrupted MR instead of opening a second one
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_adopts_unlabeled_release_m_r/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("gitlab refuses a release MR carrying an unrelated label", func(t *testing.T) {
		t.Parallel()

		// given: an open release MR labelled with something other than the pending label
		existingBody := readTestFile(
			t,
			"testdata/release/gitlab_adopts_unlabeled_release_m_r/"+
				"existing_merge_request_body.input.md",
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
			ForeignLabelOpenReleaseMR: true,
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

		// then: yeet stops rather than guessing that renamed labels are safe to overwrite
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/gitlab_refuses_foreign_label_release_m_r/"+
				"stderr.expected.txt",
			result.Stderr,
		)
	})
}

func TestReleaseUnlabeledMergedPR(t *testing.T) {
	t.Parallel()

	t.Run("github finalizes a merged release PR that lost its pending label", func(t *testing.T) {
		t.Parallel()

		// given: a merged release PR on the release branch carrying no labels at all
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{
					Message: "feat: add a thing",
					Files: map[string]string{
						"CHANGELOG.md": "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n",
					},
				},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:                  "testorg",
			Repo:                   "testrepo",
			LatestTag:              "v1.0.0",
			BoundarySHA:            shas[0],
			BranchHeadSHA:          shas[1],
			MergedPendingRelease:   true,
			UnlabeledMergedRelease: true,
			AssertPublication:      true,
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

		// then: yeet finalizes the unlabeled merged PR and tags the release instead of opening a new PR
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stderr, "finalized release")
		testastic.Contains(t, result.Stderr, "v1.1.0")
		testastic.NotContains(t, result.Stderr, "created release pull request")
	})

	t.Run("gitlab finalizes a merged release MR that lost its pending label", func(t *testing.T) {
		t.Parallel()

		// given: a merged release MR on the release branch carrying no labels at all
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://gitlab.com/group/service.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{
					Message: "feat: add a thing",
					Files: map[string]string{
						"CHANGELOG.md": "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n",
					},
				},
			})

		server := fakeprovider.NewGitLab(t, fakeprovider.GitLabOptions{
			Project:                "group/service",
			LatestTag:              "v1.0.0",
			BoundarySHA:            shas[0],
			BranchHeadSHA:          shas[1],
			MergedPendingRelease:   true,
			UnlabeledMergedRelease: true,
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

		// then: yeet finalizes the unlabeled merged MR and tags the release instead of opening a new MR
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stderr, "finalized release")
		testastic.Contains(t, result.Stderr, "v1.1.0")
		testastic.NotContains(t, result.Stderr, "created release pull request")
	})

	t.Run("azure devops finalizes a merged release PR that lost its pending label", func(t *testing.T) {
		t.Parallel()

		// given: a completed release PR on the release branch carrying no labels at all
		repoDir, shas := fixture.WriteRepoWithHistory(
			t,
			"https://dev.azure.com/testorg/testproj/_git/testrepo",
			"main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{
					Message: "feat: add a thing",
					Files: map[string]string{
						"CHANGELOG.md": "## Changelog\n\n## [v1.1.0]\n\n* feat: add a thing\n",
					},
				},
			},
		)

		server := fakeprovider.NewAzure(t, fakeprovider.AzureOptions{
			Organization:           "testorg",
			Project:                "testproj",
			Repo:                   "testrepo",
			LatestTag:              "v1.0.0",
			BoundarySHA:            shas[0],
			BranchHeadSHA:          shas[1],
			MergedPendingRelease:   true,
			UnlabeledMergedRelease: true,
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:     "azuredevops",
			Branch:       "main",
			Host:         "dev.azure.com",
			Organization: "testorg",
			Project:      "testproj",
			Repo:         "testrepo",
		})

		// when: invoking `yeet release`
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.AzureEnv(server, "main")...),
		)

		// then: yeet finalizes the unlabeled completed PR and tags the release instead of opening a new PR
		testastic.Equal(t, 0, result.ExitCode)
		testastic.Contains(t, result.Stderr, "finalized release")
		testastic.Contains(t, result.Stderr, "v1.1.0")
		testastic.NotContains(t, result.Stderr, "created release pull request")
	})
}
