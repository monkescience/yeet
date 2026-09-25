package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseNotes(t *testing.T) {
	t.Parallel()

	t.Run("github renders notes in the release pull request and changelog", func(t *testing.T) {
		t.Parallel()

		// given: a breaking commit with a nested fence, a breaking commit with only a footer and a feature with a fence
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat(config)!: rename targets to units\n\n" +
					"````release-note\nRename `targets` to `units` in `.yeet.yaml`:\n\n```yaml\nunits:\n  - path: .\n```\n````"},
				{Message: "fix: drop legacy flag\n\nBREAKING CHANGE: Remove `--legacy`.\nUse `--mode` instead."},
				{Message: "feat(release): add auto-merge\n\n" +
					"```release-note\nSet `release.auto_merge: true` to merge release PRs automatically.\n```\n\nRefs: #12"},
			})
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[3],
			ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
				Title: "chore: release 2.0.0", Head: "yeet/release-main", Base: "main",
				BodyFile: "testdata/release/github_renders_release_notes/pull_request.expected.md",
			}},
			ExpectedUpdatedFileGoldens: map[string]string{
				"CHANGELOG.md": "testdata/release/github_renders_release_notes/changelog.expected.md",
			},
		})
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			ReferenceFooters: map[string]string{"Refs": ""},
		})

		// when: creating the release pull request
		result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

		// then: the pull request body and the written changelog contain the same notes section
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("github skips a malformed release-note fence with a warning", func(t *testing.T) {
		t.Parallel()

		// given: a feature whose release-note fence contains a section heading
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add flag\n\n```release-note\n### Upgrade\n\nSet the flag.\n```"},
			})
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
		})
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
		})

		// when: invoking `yeet release --dry-run`
		result := binary.RunWithOptions(t, []string{"release", "--dry-run", "--config", configPath},
			testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

		// then: yeet releases without the note and warns with the commit, the problem and a fix
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(t, "testdata/release/github_skips_a_malformed_release_note_fence/stderr.expected.txt",
			result.Stderr)
		testastic.Contains(t, result.Stdout, "add flag")
		testastic.NotContains(t, result.Stdout, "Set the flag.")
	})

	t.Run("github routes one note to every target the commit touches", func(t *testing.T) {
		t.Parallel()

		// given: one commit with a note that changes files in two path targets
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{
					Message: "feat: share auth client\n\n```release-note\nBoth services read `AUTH_URL`.\n```",
					Files:   map[string]string{"api/auth.go": "api\n", "web/auth.ts": "web\n"},
				},
			})
		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
			ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
				Title: "chore: release wave", Head: "yeet/release-main", Base: "main",
				BodyFile: "testdata/release/github_routes_one_note_to_every_target/pull_request.expected.md",
			}},
		})
		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			Targets: []fixture.TargetOptions{
				{Name: "api", Path: "api/", TagPrefix: "api/v"},
				{Name: "web", Path: "web/", TagPrefix: "web/v"},
			},
		})

		// when: creating the combined release pull request
		result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

		// then: both target entries render the same note
		testastic.Equal(t, 0, result.ExitCode)
	})
}
