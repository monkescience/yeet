package integration_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseNewPROmitsNotesWarningOnce(t *testing.T) {
	t.Parallel()

	// given: release notes larger than the configured body limit and no pending release PR
	repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: " + strings.Repeat("additional release detail ", 150)},
		})
	server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
		Owner:         "testorg",
		Repo:          "testrepo",
		LatestTag:     "v1.0.0",
		BoundarySHA:   shas[0],
		BranchHeadSHA: shas[1],
		ExpectPRTitle: "chore: release 1.1.0",
	})
	configPath := absoluteTestFile(t, "testdata/release/new_pr_omits_notes_warning_once/input.yaml")

	// when: creating the release PR
	result := binary.RunWithOptions(t,
		[]string{"release", "--no-color", "--config", configPath},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
	)

	// then: the PR is created and the omission warning appears once
	testastic.Equal(t, 0, result.ExitCode)
	testastic.Equal(t, 1, strings.Count(result.Stderr, "created release pull request"))
	testastic.Equal(t, 1, strings.Count(result.Stderr, "omitted release notes from pull request body"))
}
