package release

import (
	"context"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

type releaseAnalyzer struct {
	core            *releaseCore
	history         versionHistoryProvider
	prs             releasePRProvider
	bumpMapping     commit.BumpMapping
	commitCache     map[commitCacheKey][]provider.CommitEntry
	overrideCache   map[string]commitOverrideResult
	overrideTypes   map[string]struct{}
	analyzedTargets map[string]config.ResolvedTarget
	versionRefs     *releaseVersionRefs
	historyIndex    *monorepoHistoryIndex
	refReachable    map[string]bool
}

func newReleaseAnalyzer(core *releaseCore, history versionHistoryProvider, prs releasePRProvider) *releaseAnalyzer {
	return &releaseAnalyzer{
		core:          core,
		history:       history,
		prs:           prs,
		bumpMapping:   core.cfg.BumpTypes.ToBumpMapping(),
		commitCache:   make(map[commitCacheKey][]provider.CommitEntry),
		overrideCache: make(map[string]commitOverrideResult),
		overrideTypes: knownCommitTypes(core.cfg),
		refReachable:  make(map[string]bool),
	}
}

func (a *releaseAnalyzer) analyze(ctx context.Context, selectedTargetIDs []string) (*Result, error) {
	r := a.core
	result := &Result{BaseBranch: r.cfg.Branch}

	selection, err := a.selectTargets(selectedTargetIDs)
	if err != nil {
		return nil, err
	}

	a.analyzedTargets = selection.analyzedPathTargets

	if err := a.buildSharedHistoryIndex(ctx, selection); err != nil {
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
