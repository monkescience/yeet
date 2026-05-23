package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseChangelogReferences(t *testing.T) {
	t.Parallel()

	t.Run("github links inline pattern matches and footer references", func(t *testing.T) {
		t.Parallel()

		// given: a config with both inline reference patterns and footer references
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{
					SHA:     "head-sha",
					Message: "feat: ship ABC-123 update\n\nRefs: PROJ-99",
				},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
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
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the changelog hyperlinks the inline match and appends the footer reference
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_references/github_patterns_and_footers/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("github includes bare footer reference when pattern is empty", func(t *testing.T) {
		t.Parallel()

		// given: a footer reference whose URL pattern is blank, value is appended verbatim
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:       "testorg",
			Repo:        "testrepo",
			LatestTag:   "v1.0.0",
			BoundarySHA: "boundary-sha",
			Commits: []fakeprovider.GitHubCommit{
				{
					SHA:     "head-sha",
					Message: "feat: ship update\n\nReviewed-by: alice",
				},
				{SHA: "boundary-sha", Message: "chore: release v1.0.0"},
			},
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
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the changelog appends the bare footer value
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release_changelog_references/github_bare_footer/stdout.expected.txt",
			result.Stdout,
		)
	})
}
