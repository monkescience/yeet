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
		testastic.Contains(t, result.Stderr, "adopting unlabelled release PR")
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
