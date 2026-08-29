package config_test

import (
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestReleaseGroups(t *testing.T) {
	t.Parallel()

	t.Run("combined remains the default", func(t *testing.T) {
		t.Parallel()

		// given: the built-in configuration
		cfg := config.Default()

		// when: inspecting the release layout
		mode := cfg.Release.PullRequestMode

		// then: existing repositories retain the combined release wave
		testastic.Equal(t, config.PullRequestModeCombined, mode)
	})

	t.Run("accepts independent atomic groups", func(t *testing.T) {
		t.Parallel()

		// given: two targets configured as one atomic release group
		cfg, _, loadErr := config.LoadResolvedQuiet(t.Context(), "testdata/release_groups/valid/input.yaml")
		testastic.NoError(t, loadErr)

		// when: validating the configuration
		err := cfg.Validate()

		// then: the independent grouping policy is accepted
		testastic.NoError(t, err)
	})

	t.Run("rejects groups in combined mode", func(t *testing.T) {
		t.Parallel()

		// given: a grouping policy without independent mode
		path := "testdata/release_groups/groups_in_combined_mode/input.yaml"

		// when: validating the configuration
		_, _, err := config.LoadResolvedQuiet(t.Context(), path)

		// then: the ignored grouping policy is rejected
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_groups/groups_in_combined_mode/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("rejects empty unknown and duplicate memberships", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			path string
		}{
			{name: "empty name", path: "empty_name"},
			{name: "empty group", path: "empty_group"},
			{name: "unknown target", path: "unknown_target"},
			{name: "duplicate target", path: "duplicate_target"},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				// given: an invalid independent grouping policy
				path := "testdata/release_groups/" + test.path + "/input.yaml"

				// when: validating the configuration
				_, _, err := config.LoadResolvedQuiet(t.Context(), path)

				// then: the invalid membership is identified
				testastic.ErrorIs(t, err, config.ErrInvalidConfig)
				testastic.AssertFile(
					t,
					"testdata/release_groups/"+test.path+"/error.expected.txt",
					err.Error(),
				)
			})
		}
	})
}
