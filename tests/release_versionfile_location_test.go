package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseVersionFileLocations(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name       string
		webChange  string
		compatible bool
	}{
		{name: "separate_json_pointers", webChange: "fix(web): repair navigation", compatible: true},
		{name: "identical_json_writes", webChange: "feat(web): add navigation", compatible: true},
		{name: "identical_marker_writes", webChange: "feat(web): add navigation", compatible: true},
		{name: "conflicting_json_writes", webChange: "fix(web): repair navigation"},
		{name: "mixed_formats_same_version", webChange: "feat(web): add navigation"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: two grouped targets that address the same version file
			directory := "testdata/release/versionfile_locations/" + scenario.name + "/"
			files := map[string]string{"VERSION": readTestFile(t, directory+"VERSION")}
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore(api): release", Tag: "api-v1.0.0", Files: files},
					{Message: "chore(web): release", Tag: "web-v1.0.0"},
					{Message: "feat(api): add endpoint", Files: map[string]string{"api/main.go": "package api\n"}},
					{Message: scenario.webChange, Files: map[string]string{"web/main.go": "package web\n"}},
				})

			opts := fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "api-v1.0.0", ExtraTags: []string{"web-v1.0.0"},
				BoundarySHA: shas[0], BranchHeadSHA: shas[3], Files: files,
				TagSHAs:        map[string]string{"api-v1.0.0": shas[0], "web-v1.0.0": shas[1]},
				FailOnMutation: !scenario.compatible,
			}
			if scenario.compatible {
				opts.ExpectedUpdatedFileGoldens = map[string]string{"VERSION": directory + "VERSION.expected.txt"}
				opts.ExpectedCreatedPullRequests = []fakeprovider.GitHubPullRequestExpectation{{
					Title: "chore: release wave", Head: "yeet/release-main-group-apps-8e4ec7f388", Base: "main",
				}}
			}

			server := fakeprovider.NewGitHub(t, opts)
			configPath := absoluteTestFile(t, directory+"input.yaml")

			// when: creating the grouped release through the compiled CLI
			result := binary.RunWithOptions(t,
				[]string{"release", "--config", configPath},
				testastic.WithRunWorkDir(repoDir),
				testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
			)

			// then: compatible writes compose and incompatible writes fail before mutation
			if scenario.compatible {
				testastic.Equal(t, 0, result.ExitCode)
				testastic.Equal(t, "", result.Stdout)
			} else {
				testastic.Equal(t, 1, result.ExitCode)
				testastic.AssertFile(t, directory+"stderr.expected.txt", result.Stderr)
			}
		})
	}
}

func TestReleaseVersionFilePointerValidation(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"missing_pointer_slash", "incomplete_pointer_escape"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			// given: a JSON version file with an invalid pointer in its configuration
			directory := "testdata/release/versionfile_locations/" + scenario + "/"
			configPath := absoluteTestFile(t, directory+"input.yaml")

			// when: validating a release through the compiled CLI
			result := binary.RunWithOptions(t,
				[]string{"release", "--dry-run", "--config", configPath},
				testastic.WithRunEnv("GITHUB_REF_NAME=main"),
			)

			// then: the existing configuration diagnostic is preserved
			testastic.Equal(t, 1, result.ExitCode)
			testastic.AssertFile(t, directory+"stderr.expected.txt", result.Stderr)
		})
	}
}
