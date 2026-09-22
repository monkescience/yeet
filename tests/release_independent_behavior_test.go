package integration_test

import (
	"net/http"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseIndependentBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("selected group member plans the complete atomic unit", func(t *testing.T) {
		t.Parallel()

		// given: two changed targets in one atomic group
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{
					Message: "chore(api): release api-v1.0.0",
					Files:   map[string]string{"api/CHANGELOG.md": "api release\n"},
					Tag:     "api-v1.0.0",
				},
				{
					Message: "chore(worker): release worker-v2.0.0",
					Files:   map[string]string{"worker/CHANGELOG.md": "worker release\n"},
					Tag:     "worker-v2.0.0",
				},
				{Message: "feat(api): add endpoint", Files: map[string]string{"api/handler.go": "package api\n"}},
				{Message: "fix(worker): repair queue", Files: map[string]string{"worker/queue.go": "package worker\n"}},
			},
		)

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:          "testorg",
			Repo:           "testrepo",
			LatestTag:      "api-v1.0.0",
			ExtraTags:      []string{"worker-v2.0.0"},
			BoundarySHA:    shas[0],
			TagSHAs:        map[string]string{"api-v1.0.0": shas[0], "worker-v2.0.0": shas[1]},
			BranchHeadSHA:  shas[3],
			FailOnMutation: true,
		})

		configPath := absoluteTestFile(t, "testdata/release/independent_group_target/input.yaml")

		// when: previewing a release selected by one group member
		result := binary.RunWithOptions(t,
			[]string{"release", "--dry-run", "--target", "api", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the preview contains both eligible members in one group unit
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_group_target/stdout.expected.txt",
			result.Stdout,
		)
	})

	t.Run("target selection cannot hide shared changelog ownership", func(t *testing.T) {
		t.Parallel()

		// given: independent targets that would both write the default changelog
		configPath := absoluteTestFile(t, "testdata/release/independent_file_overlap/input.yaml")

		// when: limiting the release to one of the conflicting targets
		result := binary.RunWithOptions(t,
			[]string{"release", "--target", "api", "--config", configPath},
			testastic.WithRunEnv("GITHUB_REF_NAME=main"),
		)

		// then: configuration validation still compares every configured release unit
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_file_overlap/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("grouped incompatible version writes report the version-file remedy", func(t *testing.T) {
		t.Parallel()

		// given: one atomic group whose units write incompatible versions to one file
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.FailOnMutation = true
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/incompatible_grouped_version_writes/input.yaml")

		// when: planning the release
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the remedy points at the version files, not at grouping the targets
		testastic.Equal(t, 1, result.ExitCode)
		testastic.NotContains(t, result.Stderr, "atomic group")
		testastic.AssertFile(
			t,
			"testdata/release/incompatible_grouped_version_writes/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("changelog reused as a version file reports the path remedy", func(t *testing.T) {
		t.Parallel()

		// given: a target whose changelog file is also listed as a version file
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.FailOnMutation = true
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/changelog_is_also_a_version_file/input.yaml")

		// when: planning the release
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the conflict names the file and how to separate the two roles
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/changelog_is_also_a_version_file/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("later unit is attempted after an earlier provider failure", func(t *testing.T) {
		t.Parallel()

		// given: two independent units where only the api pull request creation fails
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.Files = map[string]string{
			"api/CHANGELOG.md": "api release\n",
			"web/CHANGELOG.md": "web release\n",
		}
		opts.ExpectedCreatedPullRequests = []fakeprovider.GitHubPullRequestExpectation{
			{
				Title:      "chore: release 1.1.0",
				Head:       "yeet/release-main-target-api-21f150df25",
				Base:       "main",
				StatusCode: http.StatusUnprocessableEntity,
			},
			{
				Title: "chore: release 2.0.1",
				Head:  "yeet/release-main-target-web-1355f0b5d0",
				Base:  "main",
			},
		}

		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/independent_partial_failure/input.yaml")

		// when: running both release units in one invocation
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the command reports the failed unit after attempting both pull requests
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/independent_partial_failure/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("legacy combined pull request blocks independent mutation", func(t *testing.T) {
		t.Parallel()

		// given: independent mode with an open combined-mode release pull request
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.ExistingOpenReleasePRBody = readTestFile(
			t,
			"testdata/release/legacy_combined_request/existing_pull_request_body.input.md",
		)
		opts.FailOnMutation = true
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/independent_create/input.yaml")

		// when: running the independent release workflow
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the command fails closed before any provider mutation
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/legacy_combined_request/stderr.expected.txt",
			result.Stderr,
		)
	})

	t.Run("invalid merged manifest blocks new independent planning", func(t *testing.T) {
		t.Parallel()

		// given: a merged pending release with a malformed manifest
		repoDir, shas := writeIndependentMonorepoHistory(t)
		opts := independentGitHubOptions(shas)
		opts.MergedPendingRelease = true
		opts.MergedPendingReleaseBody = "<!-- yeet-release-manifest\n{not-json}\n-->"
		opts.FailOnMutation = true
		server := fakeprovider.NewGitHub(t, opts)
		configPath := absoluteTestFile(t, "testdata/release/independent_create/input.yaml")

		// when: running the independent release workflow
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: finalization validation fails before release analysis or provider mutation
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/release/invalid_merged_manifest/stderr.expected.txt",
			result.Stderr,
		)
	})
}

func writeIndependentMonorepoHistory(t *testing.T) (string, []string) {
	t.Helper()

	return fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
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
}

func independentGitHubOptions(shas []string) fakeprovider.GitHubOptions {
	return fakeprovider.GitHubOptions{
		Owner:         "testorg",
		Repo:          "testrepo",
		LatestTag:     "api-v1.0.0",
		ExtraTags:     []string{"web-v2.0.0"},
		BoundarySHA:   shas[0],
		TagSHAs:       map[string]string{"api-v1.0.0": shas[0], "web-v2.0.0": shas[1]},
		BranchHeadSHA: shas[3],
	}
}
