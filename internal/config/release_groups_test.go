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

func TestIndependentReleaseUnitFileOwnership(t *testing.T) {
	t.Parallel()

	newConfig := func(t *testing.T) *config.Config {
		t.Helper()

		cfg := config.Default()
		cfg.Release.PullRequestMode = config.PullRequestModeIndependent
		cfg.Targets = map[string]config.Target{
			"api": {
				Type:      config.TargetTypePath,
				Path:      "api",
				TagPrefix: "api-v",
				Changelog: config.ChangelogConfig{File: "api/CHANGELOG.md"},
			},
			"web": {
				Type:      config.TargetTypePath,
				Path:      "web",
				TagPrefix: "web-v",
				Changelog: config.ChangelogConfig{File: "web/CHANGELOG.md"},
			},
		}

		return cfg
	}

	t.Run("rejects shared inherited changelog across ungrouped targets", func(t *testing.T) {
		t.Parallel()

		// given: separate release units that inherit the same changelog
		cfg := newConfig(t)
		api := cfg.Targets["api"]
		api.Changelog = config.ChangelogConfig{}
		cfg.Targets["api"] = api
		web := cfg.Targets["web"]
		web.Changelog = config.ChangelogConfig{}
		cfg.Targets["web"] = web

		// when: validating the complete independent layout
		err := cfg.Validate()

		// then: the conflict is rejected before release planning
		testastic.Error(t, err)

		if err == nil {
			return
		}

		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_unit_file_ownership/shared_ungrouped_changelog/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("rejects shared inherited version file across ungrouped targets", func(t *testing.T) {
		t.Parallel()

		// given: separate release units that inherit the same version file
		cfg := newConfig(t)
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION"}}

		// when: validating the complete independent layout
		err := cfg.Validate()

		// then: the resolved version file conflict is rejected
		testastic.Error(t, err)

		if err == nil {
			return
		}

		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_unit_file_ownership/shared_ungrouped_version_file/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("rejects shared changelog across separate groups", func(t *testing.T) {
		t.Parallel()

		// given: separate atomic groups that inherit the same changelog
		cfg := newConfig(t)
		api := cfg.Targets["api"]
		api.Changelog = config.ChangelogConfig{}
		cfg.Targets["api"] = api
		web := cfg.Targets["web"]
		web.Changelog = config.ChangelogConfig{}
		cfg.Targets["web"] = web
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"backend":  {Targets: []string{"api"}},
			"frontend": {Targets: []string{"web"}},
		}

		// when: validating the complete independent layout
		err := cfg.Validate()

		// then: group boundaries do not hide the conflict
		testastic.Error(t, err)

		if err == nil {
			return
		}

		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_unit_file_ownership/shared_group_changelog/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("allows shared outputs inside one atomic group", func(t *testing.T) {
		t.Parallel()

		// given: targets in one atomic group that share resolved outputs
		cfg := newConfig(t)
		cfg.VersionFiles = []config.VersionFile{{Path: "VERSION"}}
		api := cfg.Targets["api"]
		api.Changelog = config.ChangelogConfig{}
		cfg.Targets["api"] = api
		web := cfg.Targets["web"]
		web.Changelog = config.ChangelogConfig{}
		cfg.Targets["web"] = web
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"apps": {Targets: []string{"api", "web"}},
		}

		// when: validating the complete independent layout
		err := cfg.Validate()

		// then: one atomic owner may intentionally share its files
		testastic.NoError(t, err)
	})
}
