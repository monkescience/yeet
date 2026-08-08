package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseChangelogPreservation(t *testing.T) {
	t.Parallel()

	t.Run("github preserves manual sections when refreshing the release PR", func(t *testing.T) {
		t.Parallel()

		// given: a release branch CHANGELOG carrying hand-written sections around the generated one
		dir := "testdata/release_changelog_preservation/github_preserves_manual_sections/"

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
			Files:                     map[string]string{"CHANGELOG.md": readTestFile(t, dir+"changelog.input.md")},
			ExistingOpenReleasePRBody: readTestFile(t, dir+"existing_pull_request_body.input.md"),
			ExpectPRBodyFile:          dir + "pull_request_body.expected.md",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
		})

		// when: invoking `yeet release` so the open PR is refreshed
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the refreshed body keeps both manual sections in place around the regenerated one
		testastic.Equal(t, 0, result.ExitCode)
	})
}
