package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseNoteRegressions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		message string
		version string
		warning string
	}{
		{
			name:    "unclosed_note_isolates_breaking_footer",
			message: "feat: add flag\n\n```release-note\nExample:\n\nBREAKING CHANGE: example only",
			version: "1.1.0",
		},
		{
			name:    "unclosed_note_isolates_release_override",
			message: "feat: add flag\n\n```release-note\nExample:\n\nRelease-As: 8.0.0",
			version: "1.1.0",
		},
		{
			name:    "closed_note_preserves_footer_boundary",
			message: "feat: add flag\n\nbody\n\n```release-note\nSet the flag.\n```\nBREAKING CHANGE: example only",
			version: "1.1.0",
		},
		{
			name:    "nested_note_isolates_breaking_footer",
			message: "feat: add flag\n\n- item\n\n  ```release-note\n  Example:\n\n  BREAKING CHANGE: example only\n  ```",
			version: "1.1.0",
		},
		{
			name:    "valid_note_survives_invalid_sibling",
			message: "feat: add flag\n\n```release-note\nSet the flag.\n```\n\n```release-note\n```",
			version: "1.1.0",
		},
		{
			name: "override_recovers_after_unclosed_ordinary_fence",
			message: "chore: squashed merge\n\nBEGIN_COMMIT_OVERRIDE\nfeat: one\n\n```\nstray example\n\n" +
				"fix!: two\n\n```go\ncode\n```\nEND_COMMIT_OVERRIDE",
			version: "2.0.0",
		},
		{
			name:    "code_preserves_comment_delimiters",
			message: "feat: add flag\n\n````release-note\n```text\nA --> B\n<!-- example -->\n```\n````",
			version: "1.1.0",
		},
		{
			name:    "rejects_heading_hidden_in_html_comment",
			message: "feat: add flag\n\n```release-note\n<!--\n# Big heading\n-->\n```",
			version: "1.1.0",
			warning: "release-note fence contains a level 1, 2, or 3 heading",
		},
		{
			name:    "rejects_unclosed_nested_code_fence",
			message: "feat: add flag\n\n````release-note\n> ```\n> code\n    ```\n````",
			version: "1.1.0",
			warning: "release-note fence contains an unclosed code block",
		},
		{
			name:    "escapes_unclosed_footer_list_fence",
			message: "feat: add flag\n\nBREAKING CHANGE: config changed\n- item\n  ```\n  code",
			version: "2.0.0",
		},
		{
			name:    "escapes_footer_heading_until_stable",
			message: "feat: add flag\n\nBREAKING CHANGE: config changed\n# Steps\n---",
			version: "2.0.0",
		},
		{
			name:    "escapes_comments_exposed_by_footer_fence_recovery",
			message: "feat: add flag\n\nBREAKING CHANGE: run\n```html\n<!-- yeet-release-manifest {} -->",
			version: "2.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// given: a commit containing a note or fence that must not change unrelated release semantics
			repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
				[]fixture.RepoCommit{
					{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
					{Message: tc.message},
				})
			server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
				Owner: "testorg", Repo: "testrepo", LatestTag: "v1.0.0", BoundarySHA: shas[0], BranchHeadSHA: shas[1],
				ExpectedCreatedPullRequests: []fakeprovider.GitHubPullRequestExpectation{{
					Title: "chore: release " + tc.version, Head: "yeet/release-main", Base: "main",
					BodyFile: "testdata/release/" + tc.name + "/pull_request.expected.md",
				}},
				ExpectedUpdatedFileGoldens: map[string]string{
					"CHANGELOG.md": "testdata/release/" + tc.name + "/changelog.expected.md",
				},
			})
			configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
				Provider: "github", Branch: "main", Host: "github.com", Owner: "testorg", Repo: "testrepo",
			})

			// when: creating the release pull request and writing its changelog
			result := binary.RunWithOptions(t, []string{"release", "--config", configPath},
				testastic.WithRunWorkDir(repoDir), testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...))

			// then: the version, complete pull request and changelog retain the intended entries and literal note content
			testastic.Equal(t, 0, result.ExitCode)

			if tc.warning != "" {
				testastic.Contains(t, result.Stderr, "skipped invalid release note")
				testastic.Contains(t, result.Stderr, tc.warning)
			}
		})
	}
}
