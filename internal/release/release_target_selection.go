package release

import (
	"fmt"
	"sort"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

type releaseSelection struct {
	explicitTargets     map[string]config.ResolvedTarget
	analyzedPathTargets map[string]config.ResolvedTarget
	emitPathTargetIDs   map[string]struct{}
}

func (a *releaseAnalyzer) selectTargets(selectedTargetIDs []string) (releaseSelection, error) {
	r := a.releaser
	if len(selectedTargetIDs) == 0 {
		return releaseSelection{
			explicitTargets:     r.targets,
			analyzedPathTargets: filterTargetsByType(r.targets, config.TargetTypePath),
			emitPathTargetIDs:   targetIDSet(filterTargetsByType(r.targets, config.TargetTypePath)),
		}, nil
	}

	selectedTargets := make(map[string]config.ResolvedTarget, len(selectedTargetIDs))
	analyzedPathTargets := make(map[string]config.ResolvedTarget)
	emitPathTargetIDs := make(map[string]struct{})

	for _, selectedTargetID := range selectedTargetIDs {
		normalizedTargetID := strings.TrimSpace(selectedTargetID)

		target, exists := r.targets[normalizedTargetID]
		if !exists {
			return releaseSelection{}, fmt.Errorf("%w: %s", ErrUnknownTarget, normalizedTargetID)
		}

		selectedTargets[normalizedTargetID] = target

		if target.Type == config.TargetTypePath {
			analyzedPathTargets[normalizedTargetID] = target
			emitPathTargetIDs[normalizedTargetID] = struct{}{}

			continue
		}

		for _, includeID := range target.Includes {
			includedTarget, exists := r.targets[includeID]
			if !exists {
				return releaseSelection{}, fmt.Errorf("%w: %s (included by %s)", ErrUnknownTarget, includeID, normalizedTargetID)
			}

			analyzedPathTargets[includeID] = includedTarget
		}
	}

	return releaseSelection{
		explicitTargets:     selectedTargets,
		analyzedPathTargets: analyzedPathTargets,
		emitPathTargetIDs:   emitPathTargetIDs,
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
