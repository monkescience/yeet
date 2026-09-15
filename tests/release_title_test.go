package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/testsupport/fakeprovider"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestReleaseTitleTemplates(t *testing.T) {
	t.Parallel()

	t.Run("renders the configured pr title and commit subject", func(t *testing.T) {
		t.Parallel()

		// given: a fake GitHub repository and templates for the PR title and commit subject
		repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
			[]fixture.RepoCommit{
				{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
				{Message: "feat: add a thing"},
			})

		server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
			Owner:               "testorg",
			Repo:                "testrepo",
			LatestTag:           "v1.0.0",
			BoundarySHA:         shas[0],
			BranchHeadSHA:       shas[1],
			ExpectPRTitle:       "ship default v1.1.0 from main",
			ExpectCommitSubject: "chore: cut 1.1.0",
		})

		configPath := fixture.WriteConfig(t, fixture.ConfigOptions{
			Provider:      "github",
			Branch:        "main",
			Host:          "github.com",
			Owner:         "testorg",
			Repo:          "testrepo",
			PRTitle:       "ship {{ $.Target }} {{ .Tag }} from {{ .Branch }}",
			CommitSubject: "chore: cut {{ .Version }}",
		})

		// when: invoking `yeet release` against the fake server
		result := binary.RunWithOptions(t,
			[]string{"release", "--config", configPath},
			testastic.WithRunWorkDir(repoDir),
			testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
		)

		// then: the fake server received the rendered title and commit subject
		testastic.Equal(t, 0, result.ExitCode)
	})

	t.Run("rejects an unknown field in the pr title", func(t *testing.T) {
		t.Parallel()

		// given: a PR title template referencing an unavailable field

		// when: validating the template during a dry-run release

		// then: the unavailable field is reported
		assertReleaseTitleRejected(t, "unknown_field", fixture.ConfigOptions{
			PRTitle: "ship {{ .Nope }}",
		})
	})

	t.Run("rejects an unknown field inside a conditional", func(t *testing.T) {
		t.Parallel()

		// given: a PR title conditional referencing an unavailable field

		// when: validating the template during a dry-run release

		// then: the unavailable conditional field is reported
		assertReleaseTitleRejected(t, "unknown_field_in_conditional", fixture.ConfigOptions{
			PRTitle: "ship{{ if .Nope }} it{{ end }}",
		})
	})

	t.Run("rejects a field reached through a variable", func(t *testing.T) {
		t.Parallel()

		// given: a PR title template reaching an unavailable field through a variable

		// when: validating the template during a dry-run release

		// then: the indirect field access is reported as unavailable
		assertReleaseTitleRejected(t, "field_through_variable", fixture.ConfigOptions{
			PRTitle: "{{ $plan := . }}ship {{ $plan.Target }}",
		})
	})

	t.Run("rejects a single-release field in the group pr title", func(t *testing.T) {
		t.Parallel()

		// given: a group PR title template using a single-release field

		// when: validating the template during a dry-run release

		// then: the single-release field is reported as unavailable
		assertReleaseTitleRejected(t, "single_field_in_group_title", fixture.ConfigOptions{
			PRTitleGroup: "ship {{ .Target }}",
		})
	})

	parseFailures := []struct {
		name     string
		scenario string
		template string
	}{
		{name: "unclosed action", scenario: "invalid_template_syntax", template: "ship {{"},
		{name: "unexpected end", scenario: "template_unexpected_end", template: "{{ if .Branch }}ship"},
		{
			name: "private unterminated quoted string", scenario: "template_unterminated_quoted_string",
			template: `{{ printf "private-template-value }}`,
		},
		{
			name: "private unterminated raw quoted string", scenario: "template_unterminated_raw_quoted_string",
			template: "{{ printf `private-template-value }}",
		},
		{
			name: "private undefined function", scenario: "template_undefined_function",
			template: "{{ privateTemplateFunction }}",
		},
	}
	for _, failure := range parseFailures {
		t.Run("reports "+failure.name, func(t *testing.T) {
			t.Parallel()

			// given: a PR title template with invalid syntax

			// when: parsing the template during a dry-run release

			// then: the safe parser reason is reported without template contents
			assertReleaseTitleRejected(t, failure.scenario, fixture.ConfigOptions{PRTitle: failure.template})
		})
	}

	renderFailures := []struct {
		name     string
		scenario string
		template string
	}{
		{name: "index out of range", scenario: "render_failure", template: "ship {{ index .Branch 99 }}"},
		{name: "incompatible type", scenario: "render_incompatible_type", template: "ship {{ printf 7 }}"},
		{name: "function failure", scenario: "render_function_failure", template: "ship {{ len 7 }}"},
	}
	for _, failure := range renderFailures {
		t.Run("reports "+failure.name, func(t *testing.T) {
			t.Parallel()

			// given: a PR title template that fails during execution

			// when: rendering the template during a dry-run release

			// then: the safe render reason is reported without template contents
			assertReleaseTitleRejected(t, failure.scenario, fixture.ConfigOptions{PRTitle: failure.template})
		})
	}

	t.Run("rejects a pr title that renders empty", func(t *testing.T) {
		t.Parallel()

		// given: a PR title template that renders empty

		// when: validating the template during a dry-run release

		// then: the empty rendered title is rejected
		assertReleaseTitleRejected(t, "renders_empty", fixture.ConfigOptions{
			PRTitle: "{{ .Channel }}",
		})
	})

	t.Run("rejects a pr title that renders more than one line", func(t *testing.T) {
		t.Parallel()

		// given: a PR title template that renders more than one line

		// when: validating the template during a dry-run release

		// then: the multiline rendered title is rejected
		assertReleaseTitleRejected(t, "renders_multiple_lines", fixture.ConfigOptions{
			PRTitle: `ship{{ "\n" }}{{ .Tag }}`,
		})
	})
}

func assertReleaseTitleRejected(t *testing.T, scenario string, opts fixture.ConfigOptions) {
	t.Helper()

	repoDir, shas := fixture.WriteRepoWithHistory(t, "https://github.com/testorg/testrepo.git", "main",
		[]fixture.RepoCommit{
			{Message: "chore: release v1.0.0", Tag: "v1.0.0"},
			{Message: "feat: add a thing"},
		})

	server := fakeprovider.NewGitHub(t, fakeprovider.GitHubOptions{
		Owner:          "testorg",
		Repo:           "testrepo",
		LatestTag:      "v1.0.0",
		BoundarySHA:    shas[0],
		BranchHeadSHA:  shas[1],
		FailOnMutation: true,
	})

	opts.Provider = "github"
	opts.Branch = "main"
	opts.Host = "github.com"
	opts.Owner = "testorg"
	opts.Repo = "testrepo"

	result := binary.RunWithOptions(t,
		[]string{"release", "--dry-run", "--config", fixture.WriteConfig(t, opts)},
		testastic.WithRunWorkDir(repoDir),
		testastic.WithRunEnv(fixture.GitHubEnv(server, "main")...),
	)

	testastic.Equal(t, 1, result.ExitCode)
	testastic.AssertFile(t, "testdata/release/"+scenario+"/stderr.expected.txt", result.Stderr)
}
