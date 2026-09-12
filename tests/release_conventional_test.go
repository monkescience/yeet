package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseTypeCase(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		message string
		version string
	}{
		{message: "FEAT!: remove API", version: "2.0.0"},
		{message: "Feat: add API", version: "1.1.0"},
		{message: "FIX: repair API", version: "1.0.1"},
	} {
		t.Run(scenario.message, func(t *testing.T) {
			t.Parallel()
			// given: a conventional commit whose type contains uppercase letters
			// when: creating its release through the CLI
			// then: the release request has the version implied by the type and breaking signal
			assertConventionalRelease(t, scenario.message, scenario.version, "")
		})
	}
}

func assertConventionalRelease(t *testing.T, message, version, bodyFile string) {
	t.Helper()
	repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: message},
		})
	server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
		Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
		ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
			Title: "chore: release " + version, Head: "yeet/release-main", Base: "main", BodyFile: bodyFile,
		}},
	})
	configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
		Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
		ReferenceFooters: map[string]string{"Refs": "", "Closes": ""},
	})
	result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
		testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))
	testastic.Equal(t, 0, result.ExitCode)
}

func TestReleaseBreakingFooterBoundaries(t *testing.T) {
	t.Parallel()

	// given: a breaking footer followed by reviewer metadata and a lowercase reference
	message := "fix: update API\n\nBREAKING CHANGE: remove API\nReviewed-by: Alice\nrefs: #123"

	// when: creating its release through the CLI
	// then: metadata is excluded from the breaking summary and the reference is rendered
	assertConventionalRelease(t, message, "2.0.0", "testdata/release/conventional_footers/pull_request.expected.md")
}

func TestReleaseBlankSeparatedFooters(t *testing.T) {
	t.Parallel()

	// given: two configured references separated by a blank line
	message := "fix: update API\n\nRefs: #123\n\nCloses: #456"

	// when: creating its release through the CLI
	// then: both references appear in the generated changelog
	assertConventionalRelease(t, message, "1.0.1", "testdata/release/blank_separated_footers/pull_request.expected.md")
}

func TestReleaseBreakingFooterValidity(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name    string
		message string
		version string
	}{
		{name: "missing space", message: "fix: update API\n\nBREAKING CHANGE:remove API", version: "1.0.1"},
		{name: "hyphen missing space", message: "fix: update API\n\nBREAKING-CHANGE:remove API", version: "1.0.1"},
		{name: "empty", message: "fix: update API\n\nBREAKING CHANGE:", version: "1.0.1"},
		{name: "whitespace only", message: "fix: update API\n\nBREAKING-CHANGE: \n \t\nRefs: #123", version: "1.0.1"},
		{name: "bang with malformed footer", message: "fix!: update API\n\nBREAKING CHANGE:", version: "2.0.0"},
		{name: "multiline description", message: "fix: update API\n\nBREAKING CHANGE: \nRemove API.", version: "2.0.0"},
		{
			name:    "valid after empty",
			message: "fix: update API\n\nBREAKING CHANGE: \nBREAKING-CHANGE: Remove API.",
			version: "2.0.0",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a fix with an optional bang and a breaking footer of varying validity
			// when: creating its release through the CLI
			// then: valid breaking signals alone cause a major release
			assertConventionalRelease(t, scenario.message, scenario.version, "")
		})
	}
}

func TestReleaseRequiresBodySeparator(t *testing.T) {
	t.Parallel()

	// given: a commit whose body immediately follows a breaking header
	repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat!: remove API\nBody without separator."},
		})
	server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
		Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
		FailOnMutation: true,
	})
	configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
		Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
	})

	// when: previewing the release through the CLI
	result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--verbose", "--config", configPath},
		testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

	// then: the malformed commit creates no release plan
	testastic.Equal(t, 0, result.ExitCode)
	testastic.AssertFile(t, "testdata/release/missing_body_separator/stdout.expected.txt", result.Stdout)
	testastic.Contains(t, result.Stderr, "invalid message structure, treating as no-bump")
	testastic.NotContains(t, result.Stderr, "non-conventional header")
}

func TestReleaseReferenceCaseInsensitive(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name    string
		message string
	}{
		{name: "reference_case", message: "feat: setup\n\nREFS: 123\nRefs: 456\nrefs: 789"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			// given: an open release whose source uses different casing for the same reference key
			dir := "testdata/release/" + scenario.name + "/"
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: scenario.message},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				ExistingOpenReleasePRBody: readTestFile(t, dir+"pull_request.input.md"),
				ExpectPRBodyFile:          dir + "pull_request.expected.md",
				Files:                     map[string]string{"CHANGELOG.md": "# Changelog\n"},
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
				ReferenceFooters: map[string]string{
					"Refs": "https://first.example/{value}",
				},
			})

			for range 2 {
				// when: refreshing the release through the CLI
				result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
					testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))
				// then: every spelling uses the same rule across refreshes
				testastic.Equal(t, 0, result.ExitCode)
			}
		})
	}
}
