package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestExcludedReleaseNoteWarnings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		body    string
		warning bool
	}{
		{
			name:    "unclosed note hides override",
			body:    "```release-note\nstray\n\nBEGIN_COMMIT_OVERRIDE\nfeat: add x\nEND_COMMIT_OVERRIDE",
			warning: true,
		},
		{
			name: "earlier invalid note does not hide override warning",
			body: "```release-note\n```\n\n```release-note\nstray\n\n" +
				"BEGIN_COMMIT_OVERRIDE\nfeat: add x\nEND_COMMIT_OVERRIDE",
			warning: true,
		},
		{
			name: "ordinary excluded unclosed note stays quiet",
			body: "```release-note\nstray",
		},
		{
			name: "closed note override example stays quiet",
			body: "```release-note\nBEGIN_COMMIT_OVERRIDE\nfeat: add x\nEND_COMMIT_OVERRIDE\n```",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: an excluded squash commit containing an override example or a malformed release note
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: "chore: squash\n\n" + tc.body},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			})

			// when: calculating a release through the CLI
			result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

			// then: no release is planned and only an override hidden by an unclosed note produces a warning
			testastic.Equal(t, 0, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)

			if tc.warning {
				testastic.Contains(t, result.Stderr, "ignored commit override inside unclosed release note")
				testastic.Contains(t, result.Stderr, "commit="+shas[1])
				testastic.Contains(t, result.Stderr, "close the release-note fence before BEGIN_COMMIT_OVERRIDE")
			} else {
				testastic.NotContains(t, result.Stderr, "WARN")
			}
		})
	}
}
