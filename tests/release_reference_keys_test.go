package integration_test

import (
	"testing"

	"github.com/monkescience/testastic"
)

func TestReleaseReferenceKeyCollisions(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"top_level", "target", "inherited", "same_template"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			// given: reference keys that differ only by case in the effective configuration
			dir := "testdata/release/reference_key_collisions/" + scenario

			// when: validating the configuration through the CLI outside a repository
			result := binary.RunWithOptions(t,
				[]string{"release", "--dry-run", "--config", absoluteTestFile(t, dir+"/input.yaml")},
				testastic.WithRunWorkDir(t.TempDir()),
				testastic.WithRunEnv("GITHUB_REF_NAME=main"),
			)

			// then: validation identifies the colliding keys before repository access
			testastic.Equal(t, 1, result.ExitCode)
			testastic.Equal(t, "", result.Stdout)
			testastic.AssertFile(t, dir+"/stderr.expected.txt", result.Stderr)
		})
	}
}
