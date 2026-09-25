package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseManifestAfterUnclosedManualFence(t *testing.T) {
	t.Parallel()

	for _, fence := range []string{"backticks", "tildes"} {
		t.Run(fence, func(t *testing.T) {
			t.Parallel()

			// given: a merged release PR with an unclosed manual code block above its generated manifest
			dir := "testdata/release/manifest_after_unclosed_" + fence + "/"
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: "chore: release v1.1.0", Files: map[string]string{
						"CHANGELOG.md": readTestFile(t, dir+"changelog.input.md"),
					}},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				MergedPendingRelease:     true,
				MergedPendingReleaseBody: readTestFile(t, dir+"pull_request.input.md"),
				AssertPublication:        true,
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			})

			// when: finalizing the merged release through the CLI
			result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

			// then: the malformed manual Markdown does not prevent tagging and publishing the release
			testastic.Equal(t, 0, result.ExitCode)
		})
	}
}
