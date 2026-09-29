package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseRepositoryCoordinateOverrides(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name   string
		config string
		flags  []string
	}{
		{
			name: "github_owner_only", config: "auto",
			flags: []string{"--provider", "github", "--owner", "team"},
		},
		{
			name: "github_conflicting_project", config: "auto",
			flags: []string{
				"--provider", "github", "--owner", "team", "--repo", "service", "--project", "other/service",
			},
		},
		{
			name: "github_nested_owner", config: "auto",
			flags: []string{"--provider", "github", "--owner", "team/subgroup", "--repo", "service"},
		},
		{
			name: "github_nested_project", config: "auto",
			flags: []string{"--provider", "github", "--project", "team/subgroup/service"},
		},
		{
			name: "github_blank_owner", config: "auto",
			flags: []string{"--provider", "github", "--owner", " ", "--repo", "service"},
		},
		{
			name: "github_blank_project", config: "auto",
			flags: []string{"--provider", "github", "--project", "///"},
		},
		{
			name: "github_project_with_inner_whitespace", config: "auto",
			flags: []string{"--provider", "github", "--project", " / / "},
		},
		{
			name: "azure_missing_organization", config: "auto",
			flags: []string{"--provider", "azuredevops", "--project", "Project", "--repo", "service"},
		},
		{name: "azure_empty_project", config: "azuredevops", flags: []string{"--project", ""}},
		{name: "azure_empty_repo", config: "azuredevops", flags: []string{"--repo", ""}},
		{name: "invalid_provider", config: "auto", flags: []string{"--provider", "wrongo"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			// given: a valid configuration and invalid repository coordinate flags
			directory := "testdata/release/repository_coordinates/"
			configPath := absoluteTestFile(t, directory+scenario.config+".input.yaml")
			args := append([]string{"release", "--dry-run", "--config", configPath}, scenario.flags...)

			// when: validating the flag overrides through the compiled CLI
			result := binary.RunWithOptions(t, args, testastic.WithRunEnv("GITHUB_REF_NAME=main"))

			// then: the coordinate diagnostic and its repository context are preserved
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, directory+scenario.name+".stderr.expected.txt", result.Stderr)
		})
	}
}
