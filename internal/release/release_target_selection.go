package release

import (
	"fmt"
	"sort"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

type releaseSelection struct {
	selectedTargets      map[string]config.ResolvedTarget
	pathTargetsToAnalyze map[string]config.ResolvedTarget
	pathTargetIDsToEmit  map[string]struct{}
}

func selectTargets(core *releaseCore, selectedTargetIDs []string) (releaseSelection, error) {
	if len(selectedTargetIDs) == 0 {
		return releaseSelection{
			selectedTargets:      core.targets,
			pathTargetsToAnalyze: filterTargetsByType(core.targets, config.TargetTypePath),
			pathTargetIDsToEmit:  targetIDSet(filterTargetsByType(core.targets, config.TargetTypePath)),
		}, nil
	}

	selectedTargets := make(map[string]config.ResolvedTarget, len(selectedTargetIDs))
	pathTargetsToAnalyze := make(map[string]config.ResolvedTarget)
	pathTargetIDsToEmit := make(map[string]struct{})

	for _, selectedTargetID := range selectedTargetIDs {
		normalizedTargetID := strings.TrimSpace(selectedTargetID)

		target, exists := core.targets[normalizedTargetID]
		if !exists {
			return releaseSelection{}, fmt.Errorf("%w: %s", errUnknownTarget, normalizedTargetID)
		}

		selectedTargets[normalizedTargetID] = target

		if target.Type == config.TargetTypePath {
			pathTargetsToAnalyze[normalizedTargetID] = target
			pathTargetIDsToEmit[normalizedTargetID] = struct{}{}

			continue
		}

		for _, includeID := range target.Includes {
			includedTarget, exists := core.targets[includeID]
			if !exists {
				return releaseSelection{}, fmt.Errorf("%w: %s (included by %s)", errUnknownTarget, includeID, normalizedTargetID)
			}

			pathTargetsToAnalyze[includeID] = includedTarget
		}
	}

	return releaseSelection{
		selectedTargets:      selectedTargets,
		pathTargetsToAnalyze: pathTargetsToAnalyze,
		pathTargetIDsToEmit:  pathTargetIDsToEmit,
	}, nil
}

func derivedTargetEligible(target config.ResolvedTarget, selectedTargetIDs map[string]struct{}) bool {
	if _, exists := selectedTargetIDs[target.ID]; exists {
		return true
	}

	for _, includeID := range target.Includes {
		if _, exists := selectedTargetIDs[includeID]; exists {
			return true
		}
	}

	return false
}

func filterTargetsByType(
	targets map[string]config.ResolvedTarget,
	targetType config.TargetType,
) map[string]config.ResolvedTarget {
	filteredTargets := make(map[string]config.ResolvedTarget)

	for targetID, target := range targets {
		if target.Type != targetType {
			continue
		}

		filteredTargets[targetID] = target
	}

	return filteredTargets
}

func targetIDSet(targets map[string]config.ResolvedTarget) map[string]struct{} {
	targetIDs := make(map[string]struct{}, len(targets))

	for targetID := range targets {
		targetIDs[targetID] = struct{}{}
	}

	return targetIDs
}

func filterPlansByID(plans map[string]TargetPlan, includedIDs map[string]struct{}) map[string]TargetPlan {
	if len(includedIDs) == 0 {
		return map[string]TargetPlan{}
	}

	filteredPlans := make(map[string]TargetPlan)

	for planID, plan := range plans {
		if _, exists := includedIDs[planID]; !exists {
			continue
		}

		filteredPlans[planID] = plan
	}

	return filteredPlans
}

func orderedPlans(plans map[string]TargetPlan) []TargetPlan {
	ordered := make([]TargetPlan, 0, len(plans))

	for _, plan := range plans {
		ordered = append(ordered, plan)
	}

	sort.SliceStable(ordered, func(leftIdx, rightIdx int) bool {
		leftPlan := ordered[leftIdx]
		rightPlan := ordered[rightIdx]

		if leftPlan.Type != rightPlan.Type {
			return leftPlan.Type < rightPlan.Type
		}

		return leftPlan.ID < rightPlan.ID
	})

	return ordered
}

func sortedTargetIDs(targets map[string]config.ResolvedTarget, targetType config.TargetType) []string {
	ids := make([]string, 0, len(targets))

	for targetID, target := range targets {
		if target.Type != targetType {
			continue
		}

		ids = append(ids, targetID)
	}

	sort.Strings(ids)

	return ids
}

func sortedHistoryTargetIDs(targets map[string]config.ResolvedTarget) []string {
	ids := make([]string, 0, len(targets))

	for id := range targets {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}
