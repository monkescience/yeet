package release

import (
	"context"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

type releaseAnalyzer struct {
	releaser        *Releaser
	bumpMapping     commit.BumpMapping
	commitCache     map[commitCacheKey][]provider.CommitEntry
	overrideCache   map[string]commitOverrideResult
	overrideTypes   map[string]struct{}
	analyzedTargets map[string]config.ResolvedTarget
	versionRefs     *releaseVersionRefs
	historyIndex    *monorepoHistoryIndex
	refReachable    map[string]bool
}

func newReleaseAnalyzer(releaser *Releaser) *releaseAnalyzer {
	return &releaseAnalyzer{
		releaser:      releaser,
		bumpMapping:   releaser.cfg.BumpTypes.ToBumpMapping(),
		commitCache:   make(map[commitCacheKey][]provider.CommitEntry),
		overrideCache: make(map[string]commitOverrideResult),
		overrideTypes: knownCommitTypes(releaser.cfg),
		refReachable:  make(map[string]bool),
	}
}

func (a *releaseAnalyzer) analyze(ctx context.Context, selectedTargetIDs []string) (*Result, error) {
	r := a.releaser
	result := &Result{BaseBranch: r.cfg.Branch}

	selection, err := a.selectTargets(selectedTargetIDs)
	if err != nil {
		return nil, err
	}

	a.analyzedTargets = selection.analyzedPathTargets

	err = a.buildSharedHistoryIndex(ctx, selection)
	if err != nil {
		return nil, err
	}

	pathPlans, err := a.planPathTargets(ctx, selection.analyzedPathTargets)
	if err != nil {
		return nil, err
	}

	derivedPlans, err := a.planDerivedTargets(ctx, selection.explicitTargets, pathPlans)
	if err != nil {
		return nil, err
	}

	result.Plans = append(result.Plans, orderedPlans(filterPlansByID(pathPlans, selection.emitPathTargetIDs))...)
	result.Plans = append(result.Plans, orderedPlans(derivedPlans)...)

	return result, nil
}
