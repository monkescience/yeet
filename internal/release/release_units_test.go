//nolint:testpackage // This test exercises the internal pure release-unit planner seam.
package release

import (
	"slices"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestPlanReleaseUnits(t *testing.T) {
	t.Parallel()

	newCore := func(t *testing.T, cfg *config.Config) *releaseCore {
		t.Helper()

		run, err := resolveRun(cfg, cfg.Branch, Options{})
		testastic.NoError(t, err)

		core, err := newReleaseCore(t.Context(), cfg, &repoMetadataStub{}, run)
		testastic.NoError(t, err)

		return core
	}

	newConfig := func(t *testing.T) *config.Config {
		t.Helper()

		cfg, _, err := config.LoadResolvedQuiet(t.Context(), "testdata/release_units/config.input.yaml")
		testastic.NoError(t, err)

		return cfg
	}

	t.Run("emits singleton units with unique default branches", func(t *testing.T) {
		t.Parallel()

		// given: two independently releasable targets
		cfg := newConfig(t)
		core := newCore(t, cfg)
		plans := []TargetPlan{{ID: "api", Type: config.TargetTypePath}, {ID: "web", Type: config.TargetTypePath}}

		// when: planning release units
		units, err := planReleaseUnits(core, plans)

		// then: each target owns a deterministic branch
		testastic.NoError(t, err)
		testastic.Equal(t, 2, len(units))
		testastic.Equal(t, "target:api", units[0].ID)
		testastic.Equal(t, "target:web", units[1].ID)
		testastic.False(t, units[0].ReleaseBranch == units[1].ReleaseBranch)
		testastic.AssertJSON(
			t,
			"testdata/release_units/singleton_units.expected.json",
			releaseUnitSnapshots(units),
		)
	})

	t.Run("combined mode emits one release unit", func(t *testing.T) {
		t.Parallel()

		// given: two eligible targets under the compatibility layout
		cfg := newConfig(t)
		cfg.Release.PullRequestMode = config.PullRequestModeCombined
		core := newCore(t, cfg)
		plans := []TargetPlan{{ID: "api", Type: config.TargetTypePath}, {ID: "web", Type: config.TargetTypePath}}

		// when: planning release units
		units, err := planReleaseUnits(core, plans)

		// then: both targets remain in the one combined wave
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(units))
		testastic.Equal(t, combinedReleaseUnitID, units[0].ID)
		testastic.Equal(t, 2, len(units[0].Plans))
		testastic.AssertJSON(
			t,
			"testdata/release_units/combined_unit.expected.json",
			releaseUnitSnapshots(units),
		)
	})

	t.Run("keeps group identity stable for an eligible subset", func(t *testing.T) {
		t.Parallel()

		// given: an atomic group where only one member is currently eligible
		cfg := newConfig(t)
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"backend": {Targets: []string{"api", "web"}},
		}
		core := newCore(t, cfg)

		// when: planning first one eligible member and then both members
		one, oneErr := planReleaseUnits(core, []TargetPlan{{ID: "api", Type: config.TargetTypePath}})
		both, bothErr := planReleaseUnits(core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath},
			{ID: "web", Type: config.TargetTypePath},
		})

		// then: current eligibility does not change the group identity or branch
		testastic.NoError(t, oneErr)
		testastic.NoError(t, bothErr)
		testastic.Equal(t, "group:backend", one[0].ID)
		testastic.Equal(t, one[0].ID, both[0].ID)
		testastic.Equal(t, one[0].ReleaseBranch, both[0].ReleaseBranch)
		testastic.AssertJSON(
			t,
			"testdata/release_units/stable_group_identity.expected.json",
			map[string]any{
				"one_eligible_member": releaseUnitSnapshots(one),
				"all_members":         releaseUnitSnapshots(both),
			},
		)
	})

	t.Run("rejects cross-unit file overlap", func(t *testing.T) {
		t.Parallel()

		// given: separate units that would both write one changelog
		cfg := newConfig(t)
		cfg.Targets["api"] = config.Target{Type: config.TargetTypePath, Path: "api", TagPrefix: "api-v"}
		cfg.Targets["web"] = config.Target{Type: config.TargetTypePath, Path: "web", TagPrefix: "web-v"}
		core := newCore(t, cfg)

		// when: planning both eligible targets
		_, err := planReleaseUnits(core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath},
			{ID: "web", Type: config.TargetTypePath},
		})

		// then: planning fails before a provider workflow can run
		testastic.ErrorIs(t, err, errConflictingFileUpdate)
		testastic.AssertFile(
			t,
			"testdata/release_units/cross_unit_changelog_overlap/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("rejects incompatible shared version writes inside a group", func(t *testing.T) {
		t.Parallel()

		// given: grouped targets with different versions and one generic marker file
		cfg := newConfig(t)
		cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
			"apps": {Targets: []string{"api", "web"}},
		}
		cfg.Targets["api"] = config.Target{
			Type: config.TargetTypePath, Path: "api", TagPrefix: "api-v",
			Changelog:    config.ChangelogConfig{File: "api/CHANGELOG.md"},
			VersionFiles: []config.VersionFile{{Path: "VERSION.txt"}},
		}
		cfg.Targets["web"] = config.Target{
			Type: config.TargetTypePath, Path: "web", TagPrefix: "web-v",
			Changelog:    config.ChangelogConfig{File: "web/CHANGELOG.md"},
			VersionFiles: []config.VersionFile{{Path: "VERSION.txt"}},
		}
		core := newCore(t, cfg)

		// when: planning distinct target versions in the atomic unit
		_, err := planReleaseUnits(core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath, NextVersion: "1.0.0"},
			{ID: "web", Type: config.TargetTypePath, NextVersion: "2.0.0"},
		})

		// then: planning rejects the ambiguous write before branch mutation
		testastic.ErrorIs(t, err, errConflictingFileUpdate)
		testastic.AssertFile(
			t,
			"testdata/release_units/incompatible_grouped_version_writes/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("rejects cross-unit version file overlap", func(t *testing.T) {
		t.Parallel()

		// given: separate targets that both write one version file
		cfg := newConfig(t)
		api := cfg.Targets["api"]
		api.VersionFiles = []config.VersionFile{{
			Path: "versions.json", Format: config.VersionFileFormatJSON, JSONPointer: "/api",
		}}
		cfg.Targets["api"] = api

		web := cfg.Targets["web"]
		web.VersionFiles = []config.VersionFile{{
			Path: "versions.json", Format: config.VersionFileFormatJSON, JSONPointer: "/web",
		}}
		cfg.Targets["web"] = web
		core := newCore(t, cfg)

		// when: planning both target units
		_, err := planReleaseUnits(core, []TargetPlan{
			{ID: "api", Type: config.TargetTypePath, NextVersion: "1.0.0"},
			{ID: "web", Type: config.TargetTypePath, NextVersion: "2.0.0"},
		})

		// then: the shared path is rejected across release units
		testastic.ErrorIs(t, err, errConflictingFileUpdate)
		testastic.AssertFile(
			t,
			"testdata/release_units/cross_unit_version_overlap/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("orders a derived unit after its child unit", func(t *testing.T) {
		t.Parallel()

		// given: a derived target in a separate release unit from its child
		cfg := newConfig(t)
		cfg.Targets["root"] = config.Target{
			Type:      config.TargetTypeDerived,
			TagPrefix: "root-v",
			Includes:  []string{"api"},
			Changelog: config.ChangelogConfig{File: "root/CHANGELOG.md"},
		}
		core := newCore(t, cfg)
		plans := []TargetPlan{
			{ID: "root", Type: config.TargetTypeDerived, IncludedTargets: []string{"api"}},
			{ID: "api", Type: config.TargetTypePath},
		}

		// when: planning release units
		units, err := planReleaseUnits(core, plans)

		// then: dependency order precedes identity order
		testastic.NoError(t, err)
		testastic.True(t, slices.IndexFunc(units, func(unit releaseUnit) bool {
			return unit.ID == "target:api"
		}) < slices.IndexFunc(units, func(unit releaseUnit) bool {
			return unit.ID == "target:root"
		}))
		testastic.AssertJSON(
			t,
			"testdata/release_units/derived_after_child.expected.json",
			releaseUnitSnapshots(units),
		)
	})
}

func TestExpandReleaseGroupSelection(t *testing.T) {
	t.Parallel()

	// given: one selected member of an atomic group
	cfg, _, err := config.LoadResolvedQuiet(t.Context(), "testdata/release_units/config.input.yaml")
	testastic.NoError(t, err)

	cfg.Release.Groups = map[string]config.ReleaseGroupConfig{
		"backend": {Targets: []string{"api", "web"}},
	}

	// when: expanding target selection before analysis
	selected := cfg.ReleaseLayout().ExpandSelection([]string{"api"})

	// then: the full group is selected exactly once
	testastic.SliceEqual(t, []string{"api", "web"}, selected)
	testastic.AssertJSON(t, "testdata/release_units/expanded_selection.expected.json", selected)
}

func TestIndependentReleaseBranchValidation(t *testing.T) {
	t.Parallel()

	newConfig := func(t *testing.T) *config.Config {
		t.Helper()

		cfg, _, err := config.LoadResolvedQuiet(t.Context(), "testdata/release_units/config.input.yaml")
		testastic.NoError(t, err)

		return cfg
	}

	t.Run("default template disambiguates units", func(t *testing.T) {
		t.Parallel()

		// given: independent mode with the default branch template
		cfg := newConfig(t)

		// when: validating every possible unit branch
		err := validateReleaseBranchTemplates(cfg)

		// then: the built-in unit suffix keeps branches unique
		testastic.NoError(t, err)
	})

	t.Run("custom template must use unit data when needed", func(t *testing.T) {
		t.Parallel()

		// given: a custom template that renders the same branch for every unit
		cfg := newConfig(t)
		cfg.Release.BranchTemplate = "automation/{{ .Branch }}/release"

		// when: validating every possible unit branch
		err := validateReleaseBranchTemplates(cfg)

		// then: configuration points to the available disambiguation field
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_units/ambiguous_custom_branch/error.expected.txt",
			err.Error(),
		)
	})

	t.Run("unit-only custom template must distinguish prerelease channels", func(t *testing.T) {
		t.Parallel()

		// given: a custom template that distinguishes units but not channels
		cfg := newConfig(t)
		cfg.Release.BranchTemplate = "automation/{{ .Unit }}"
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: validating every stable and prerelease unit branch
		err := validateReleaseBranchTemplates(cfg)

		// then: the stable and beta branch collision is rejected
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.AssertFile(
			t,
			"testdata/release_units/prerelease_branch_collision/error.expected.txt",
			err.Error(),
		)
	})
}

type releaseUnitTestSnapshot struct {
	ID            string   `json:"id"`
	BranchValue   string   `json:"branch_value,omitempty"`
	ReleaseBranch string   `json:"release_branch"`
	Plans         []string `json:"plans"`
}

func releaseUnitSnapshots(units []releaseUnit) []releaseUnitTestSnapshot {
	snapshots := make([]releaseUnitTestSnapshot, 0, len(units))

	for _, unit := range units {
		plans := make([]string, 0, len(unit.Plans))
		for _, plan := range unit.Plans {
			plans = append(plans, plan.ID)
		}

		snapshots = append(snapshots, releaseUnitTestSnapshot{
			ID:            unit.ID,
			BranchValue:   unit.BranchValue,
			ReleaseBranch: unit.ReleaseBranch,
			Plans:         plans,
		})
	}

	return snapshots
}
