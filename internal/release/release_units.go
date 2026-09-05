package release

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

const combinedReleaseUnitID = "combined"

const minReleaseUnitsForOrdering = 2

type releaseUnit struct {
	ID            string
	BranchValue   string
	ReleaseBranch string
	Plans         []TargetPlan
}

func planReleaseUnits(core *releaseCore, plans []TargetPlan) ([]releaseUnit, error) {
	tmpl, err := newReleaseBranchTemplate(effectiveReleaseBranchTemplateSource(core.cfg))
	if err != nil {
		return nil, err
	}

	if core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		if len(plans) == 0 {
			return nil, nil
		}

		return []releaseUnit{{
			ID:            combinedReleaseUnitID,
			ReleaseBranch: core.run.releaseBranch,
			Plans:         slices.Clone(plans),
		}}, nil
	}

	units := independentReleaseUnits(core.layout, plans)
	for index := range units {
		units[index].ReleaseBranch, err = renderReleaseBranch(
			tmpl,
			core.run.baseBranch,
			core.run.channelName,
			units[index].BranchValue,
		)
		if err != nil {
			return nil, err
		}
	}

	err = validateReleaseUnitFileOwnership(core.targets, units)
	if err != nil {
		return nil, err
	}

	return orderReleaseUnits(units), nil
}

func configuredReleaseUnits(core *releaseCore) ([]releaseUnit, error) {
	plans := make([]TargetPlan, 0, len(core.targets))
	for _, targetID := range slices.Sorted(maps.Keys(core.targets)) {
		target := core.targets[targetID]
		plans = append(plans, TargetPlan{ID: targetID, Type: target.Type})
	}

	if core.cfg.Release.PullRequestMode != config.PullRequestModeIndependent {
		return []releaseUnit{{
			ID:            combinedReleaseUnitID,
			ReleaseBranch: core.run.releaseBranch,
			Plans:         plans,
		}}, nil
	}

	units := independentReleaseUnits(core.layout, plans)

	tmpl, err := newReleaseBranchTemplate(effectiveReleaseBranchTemplateSource(core.cfg))
	if err != nil {
		return nil, err
	}

	for index := range units {
		units[index].ReleaseBranch, err = renderReleaseBranch(
			tmpl,
			core.run.baseBranch,
			core.run.channelName,
			units[index].BranchValue,
		)
		if err != nil {
			return nil, err
		}
	}

	units = append(units, releaseUnit{
		ID:            combinedReleaseUnitID,
		ReleaseBranch: core.run.releaseBranch,
		Plans:         plans,
	})

	return units, nil
}

func (c *releaseCore) configuredUnitPlans(unitID string) []TargetPlan {
	if unitID == combinedReleaseUnitID {
		plans := make([]TargetPlan, 0, len(c.targets))
		for _, targetID := range slices.Sorted(maps.Keys(c.targets)) {
			target := c.targets[targetID]
			plans = append(plans, TargetPlan{ID: targetID, Type: target.Type})
		}

		return plans
	}

	plans := make([]TargetPlan, 0)

	for _, unit := range c.layout.Units() {
		if unit.ID != unitID {
			continue
		}

		for _, targetID := range unit.TargetIDs {
			target := c.targets[targetID]
			plans = append(plans, TargetPlan{ID: targetID, Type: target.Type})
		}
	}

	slices.SortFunc(plans, func(left, right TargetPlan) int {
		return strings.Compare(left.ID, right.ID)
	})

	return plans
}

func independentReleaseUnits(layout config.ReleaseLayout, plans []TargetPlan) []releaseUnit {
	unitByTarget := make(map[string]config.ConfiguredReleaseUnit)

	for _, unit := range layout.Units() {
		for _, targetID := range unit.TargetIDs {
			unitByTarget[targetID] = unit
		}
	}

	unitsByID := make(map[string]releaseUnit)

	for _, plan := range plans {
		configured := unitByTarget[plan.ID]
		unit := unitsByID[configured.ID]
		unit.ID = configured.ID
		unit.BranchValue = configured.BranchValue
		unit.Plans = append(unit.Plans, plan)
		unitsByID[unit.ID] = unit
	}

	units := slices.Collect(maps.Values(unitsByID))
	slices.SortFunc(units, func(left, right releaseUnit) int {
		return strings.Compare(left.ID, right.ID)
	})

	return units
}

func validateReleaseUnitFileOwnership(
	targets map[string]config.ResolvedTarget,
	units []releaseUnit,
) error {
	owners := make(map[string]string)

	for _, unit := range units {
		err := validateReleaseUnitFileEffects(targets, unit)
		if err != nil {
			return err
		}

		unitPaths := make(map[string]struct{})

		for _, plan := range unit.Plans {
			target, exists := targets[plan.ID]
			if !exists {
				return fmt.Errorf("%w: %s", errUnknownTarget, plan.ID)
			}

			unitPaths[target.Changelog.File] = struct{}{}
			for _, versionFile := range target.VersionFiles {
				unitPaths[versionFile.Path] = struct{}{}
			}
		}

		for path := range unitPaths {
			if previous, exists := owners[path]; exists && previous != unit.ID {
				return fmt.Errorf(
					"%w: release units %q and %q both write %q, configure separate files or place the targets in one atomic group",
					errConflictingFileUpdate,
					previous,
					unit.ID,
					path,
				)
			}

			owners[path] = unit.ID
		}
	}

	return nil
}

type plannedVersionFileEffect struct {
	format      config.VersionFileFormat
	jsonPointer string
	versioning  config.VersioningStrategy
	nextVersion string
}

func validateReleaseUnitFileEffects(targets map[string]config.ResolvedTarget, unit releaseUnit) error {
	changelogPaths := make(map[string]struct{})
	versionEffects := make(map[string][]plannedVersionFileEffect)

	for _, plan := range unit.Plans {
		target := targets[plan.ID]
		changelogPaths[target.Changelog.File] = struct{}{}

		for _, versionFile := range target.VersionFiles {
			effect := plannedVersionFileEffect{
				format:      versionFile.Format,
				jsonPointer: versionFile.JSONPointer,
				versioning:  target.Versioning,
				nextVersion: plan.NextVersion,
			}

			for _, existing := range versionEffects[versionFile.Path] {
				if versionFileEffectsConflict(existing, effect) {
					return fmt.Errorf(
						"%w: release unit %q has incompatible version writes to %q, configure separately addressable version files",
						errConflictingFileUpdate,
						unit.ID,
						versionFile.Path,
					)
				}
			}

			versionEffects[versionFile.Path] = append(versionEffects[versionFile.Path], effect)
		}
	}

	for path := range changelogPaths {
		if len(versionEffects[path]) > 0 {
			return fmt.Errorf(
				"%w: release unit %q writes %q as both a changelog and version file",
				errConflictingFileUpdate,
				unit.ID,
				path,
			)
		}
	}

	return nil
}

func versionFileEffectsConflict(left, right plannedVersionFileEffect) bool {
	if left.format != right.format {
		return true
	}

	if left.format == config.VersionFileFormatJSON && left.jsonPointer != right.jsonPointer {
		return false
	}

	return left.nextVersion != right.nextVersion || left.versioning != right.versioning
}

func orderReleaseUnits(units []releaseUnit) []releaseUnit {
	if len(units) < minReleaseUnitsForOrdering {
		return units
	}

	unitByTarget := make(map[string]string)
	unitByID := make(map[string]releaseUnit, len(units))

	for _, unit := range units {
		unitByID[unit.ID] = unit
		for _, plan := range unit.Plans {
			unitByTarget[plan.ID] = unit.ID
		}
	}

	dependencies := make(map[string]map[string]struct{}, len(units))
	for _, unit := range units {
		dependencies[unit.ID] = make(map[string]struct{})
		for _, plan := range unit.Plans {
			for _, childID := range plan.IncludedTargets {
				childUnitID, exists := unitByTarget[childID]
				if exists && childUnitID != unit.ID {
					dependencies[unit.ID][childUnitID] = struct{}{}
				}
			}
		}
	}

	ordered := make([]releaseUnit, 0, len(units))
	remaining := maps.Clone(unitByID)

	for len(remaining) > 0 {
		ready := make([]string, 0)

		for unitID := range remaining {
			if len(dependencies[unitID]) == 0 {
				ready = append(ready, unitID)
			}
		}

		slices.Sort(ready)

		if len(ready) == 0 {
			return units
		}

		for _, unitID := range ready {
			ordered = append(ordered, remaining[unitID])
			delete(remaining, unitID)

			for otherID := range remaining {
				delete(dependencies[otherID], unitID)
			}
		}
	}

	return ordered
}
