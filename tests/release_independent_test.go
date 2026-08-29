package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseIndependentMonorepo(t *testing.T) {
	t.Parallel()

	t.Run("github dry-run plans one pull request per release unit", func(t *testing.T) {
		t.Parallel()

		// given: two independently releasable targets with separate tags and changes
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{
					Message: "chore(api): release api-v1.0.0",
					Files:   map[string]string{"api/CHANGELOG.md": "api release\n"},
					Tag:     "api-v1.0.0",
				},
				{
					Message: "chore(web): release web-v2.0.0",
					Files:   map[string]string{"web/CHANGELOG.md": "web release\n"},
					Tag:     "web-v2.0.0",
				},
				{Message: "feat(api): add endpoint", Files: map[string]string{"api/handler.go": "package api\n"}},
				{Message: "fix(web): repair navigation", Files: map[string]string{"web/index.html": "<nav></nav>\n"}},
			},
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "api-v1.0.0",
			ExtraTags:     []string{"web-v2.0.0"},
			BoundarySHA:   shas[0],
			TagSHAs:       map[string]string{"api-v1.0.0": shas[0], "web-v2.0.0": shas[1]},
			BranchHeadSHA: shas[3],
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release/independent_monorepo/input.yaml",
		)

		// when: previewing the independent release from the compiled CLI
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: each target receives its own release unit and pull request plan
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_monorepo/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github creates one pull request per release unit", func(t *testing.T) {
		t.Parallel()

		// given: two independently releasable targets with separate changelogs
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{
					Message: "chore(api): release api-v1.0.0",
					Files:   map[string]string{"api/CHANGELOG.md": "api release\n"},
					Tag:     "api-v1.0.0",
				},
				{
					Message: "chore(web): release web-v2.0.0",
					Files:   map[string]string{"web/CHANGELOG.md": "web release\n"},
					Tag:     "web-v2.0.0",
				},
				{Message: "feat(api): add endpoint", Files: map[string]string{"api/handler.go": "package api\n"}},
				{Message: "fix(web): repair navigation", Files: map[string]string{"web/index.html": "<nav></nav>\n"}},
			},
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "api-v1.0.0",
			ExtraTags:     []string{"web-v2.0.0"},
			BoundarySHA:   shas[0],
			TagSHAs:       map[string]string{"api-v1.0.0": shas[0], "web-v2.0.0": shas[1]},
			BranchHeadSHA: shas[3],
			ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{
				{
					Title:    "chore: release 1.1.0",
					Head:     "yeet/release-main-target-api-21f150df25",
					Base:     "main",
					BodyFile: "testdata/release/independent_create/api_body.expected.md",
				},
				{
					Title:    "chore: release 2.0.1",
					Head:     "yeet/release-main-target-web-1355f0b5d0",
					Base:     "main",
					BodyFile: "testdata/release/independent_create/web_body.expected.md",
				},
			},
			Files: map[string]string{
				"api/CHANGELOG.md": "api release\n",
				"web/CHANGELOG.md": "web release\n",
			},
		})

		configPath := absoluteTestFile(
			t,
			"testdata/release/independent_create/input.yaml",
		)

		// when: running the independent release from the compiled CLI
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: both release units create their exact pull request and stdout remains empty
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_create/stdout.expected.txt",
			result.Stdout,
		)
	})
}
