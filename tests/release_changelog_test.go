package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseChangelogReferences(t *testing.T) {
	t.Parallel()

	t.Run("github links inline pattern matches and footer references", func(t *testing.T) {
		t.Parallel()

		// given: a config with both inline reference patterns and footer references
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: ship ABC-123 update\n\nRefs: PROJ-99"},
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
			ReferencePatterns: []fixture.ReferencePatternOptions{
				{Pattern: "ABC-\\d+", URL: "https://issues.example.test/{value}"},
			},
			ReferenceFooters: map[string]string{
				"Refs": "https://tracker.example.test/{value}",
			},
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the changelog hyperlinks the inline match and appends the footer reference
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_patterns_and_footers/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github includes bare footer reference when pattern is empty", func(t *testing.T) {
		t.Parallel()

		// given: a footer reference whose URL pattern is blank, value is appended verbatim
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: ship update\n\nReviewed-by: alice"},
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
			ReferenceFooters: map[string]string{
				"Reviewed-by": "",
			},
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the changelog appends the bare footer value
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/github_bare_footer/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github renders folded comma-separated and encoded footer references", func(t *testing.T) {
		t.Parallel()

		// given: commits with multi-line squash bodies, wrapped comma-separated references, and URL-special characters
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "fix: handle edge case\n\nCloses #12\n\n## Testing\nRan unit tests."},
				{Message: "feat: track work\n\nRefs: PROJ-1,\n  PROJ-2\n\n## Notes\nExtra details."},
				{Message: "feat: link query ref\n\nTicket: team A&B (1)\nExtra unindented line"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[3],
			ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
				Title:    "chore: release 1.1.0",
				Head:     "yeet/release-main",
				Base:     "main",
				BodyFile: "testdata/release/github_multiline_and_comma_footers/pull_request.expected.md",
			}},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			ReferenceFooters: map[string]string{
				"Closes": "",
				"Refs":   "https://jira.example.test/browse/{value}",
				"Ticket": "https://tracker.example.test/search?q={value}",
			},
		})

		// when: creating the release pull request through the CLI
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: each footer renders its first line and indented or comma-continued lines as valid Markdown references
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github does not nest links for overlapping reference patterns", func(t *testing.T) {
		t.Parallel()

		// given: overlapping inline reference patterns and commits containing overlapping and already-linked references
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "fix: link PROJ-12 and OTHER-34 without nesting"},
				{Message: "feat: keep [PROJ-99](https://existing.example.test/PROJ-99) and link PROJ-100"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:         "testorg",
			Repo:          "testrepo",
			LatestTag:     "v1.0.0",
			BoundarySHA:   shas[0],
			BranchHeadSHA: shas[2],
			ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
				Title:    "chore: release 1.1.0",
				Head:     "yeet/release-main",
				Base:     "main",
				BodyFile: "testdata/release/github_overlapping_patterns/pull_request.expected.md",
			}},
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github",
			Branch:   "main",
			Host:     "github.com",
			Owner:    "testorg",
			Repo:     "testrepo",
			ReferencePatterns: []fixture.ReferencePatternOptions{
				{Pattern: `\bPROJ-\d+\b`, URL: "https://jira.example.test/browse/{value}"},
				{Pattern: `\b[A-Z]+-\d+\b`, URL: "https://other.example.test/{value}"},
			},
		})

		// when: creating the release pull request through the CLI
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: earlier patterns win on overlap and existing Markdown links remain intact
		testastic.Equal(t, 0, result.ExitCode)
	})
}
