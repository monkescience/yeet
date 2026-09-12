package integration_test

import (
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseBreakingMarkerDiagnostics(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name     string
		body     string
		rejected int
	}{
		{name: "lowercase hyphen", body: "breaking-change: private migration details", rejected: 1},
		{name: "lowercase space", body: "breaking change: private migration details", rejected: 1},
		{name: "missing separator space", body: "BREAKING CHANGE:private migration details", rejected: 1},
		{name: "missing description", body: "BREAKING CHANGE:", rejected: 1},
		{name: "bare marker", body: "BREAKING CHANGE", rejected: 1},
		{name: "hash separator", body: "BREAKING-CHANGE #123", rejected: 1},
		{name: "ordinary prose", body: "Breaking changes are listed in the migration guide."},
		{name: "different token", body: "BREAKING-CHANGELOG: private migration details"},
		{name: "valid marker", body: "BREAKING CHANGE: private migration details"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a fix with a breaking marker or ordinary prose in its body
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: "fix: update API\n\n" + scenario.body},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				FailOnMutation: true,
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			})

			// when: previewing the release with verbose diagnostics
			result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--verbose", "--config", configPath},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

			// then: only rejected marker tokens produce diagnostics and their values stay private
			testastic.Equal(t, 0, result.ExitCode)
			testastic.Equal(t, scenario.rejected, strings.Count(result.Stderr, "breaking marker rejected"))
			testastic.NotContains(t, result.Stderr, "private migration details")
		})
	}
}
