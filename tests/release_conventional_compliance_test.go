package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseUnicodeType(t *testing.T) {
	t.Parallel()

	for _, message := range []string{"sécurité!: remove API", "SÉCURITÉ!: remove API", "安全!: remove API"} {
		t.Run(message, func(t *testing.T) {
			t.Parallel()

			// given: a breaking commit with a custom Unicode type
			// when: creating its release through the CLI
			// then: the custom type still triggers a major release
			assertConventionalRelease(t, message, "2.0.0", "")
		})
	}
}

func TestReleaseWordFooterBoundaries(t *testing.T) {
	t.Parallel()

	for _, token := range []string{"Reviewed_by", "Référence", "Re\u0301fe\u0301rence", "参考"} {
		for _, separator := range []string{": ", " #"} {
			t.Run(token+separator, func(t *testing.T) {
				t.Parallel()

				// given: a breaking footer followed by metadata with a Unicode or underscore token
				message := "fix: update API\n\nBREAKING CHANGE: remove API\n" + token + separator + "123\nrefs: #123"

				// when: creating its release through the CLI
				// then: the breaking summary excludes the metadata and the reference remains visible
				assertConventionalRelease(t, message, "2.0.0",
					"testdata/release/conventional_footers/pull_request.expected.md")
			})
		}
	}
}

func TestReleaseRejectsHeaderWhitespace(t *testing.T) {
	t.Parallel()

	for _, message := range []string{"  fix: resolve timeout", "fix(   )!: remove API", "fix(\u00a0): resolve timeout"} {
		t.Run(message, func(t *testing.T) {
			t.Parallel()

			// given: a commit with leading whitespace or a whitespace-only scope
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: message},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				FailOnMutation: true,
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			})

			// when: previewing its release through the CLI
			result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

			// then: the malformed header creates no release plan
			testastic.Equal(t, 0, result.ExitCode)
			testastic.AssertFile(t, "testdata/release/missing_body_separator/stdout.expected.txt", result.Stdout)
		})
	}
}
