package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseSchemaDiagnostics(t *testing.T) {
	t.Parallel()

	scenarios := []string{
		"missing_targets",
		"empty_target_id",
		"missing_target_type",
		"derived_target_missing_includes",
		"version_file_missing_path",
		"version_file_pointer_on_generic",
		"changelog_pattern_missing_pattern",
		"changelog_pattern_url_not_string",
		"changelog_sections_empty_key",
		"changelog_section_heading_multiline",
		"changelog_section_headings_duplicate",
		"changelog_fallback_heading_duplicate",
		"duplicate_reviewers",
		"duplicate_extra_labels",
		"padded_pending_label",
		"negative_pr_body_max_length",
		"release_node_wrong_type",
		"release_channels_empty_key",
		"empty_exclude_path",
		"empty_minor_bump_type",
	}

	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			// given: a configuration fixture that violates one schema rule.
			fixtureDir := "testdata/release/" + scenario

			// when: validating the configuration through a release dry run.
			result := binary.RunWithOptions(
				t,
				[]string{"release", "--dry-run", "--config", absoluteTestFile(t, fixtureDir+"/input.yaml")},
				testastic.WithRunEnv("GITHUB_REF_NAME=main"),
			)

			// then: yeet rejects it with the scenario's user-facing diagnostic.
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, fixtureDir+"/stderr.expected.txt", result.Stderr)
		})
	}
}
