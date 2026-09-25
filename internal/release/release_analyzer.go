package release

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

type releaseAnalyzer struct {
	core          *releaseCore
	history       versionHistoryProvider
	bumpMapping   commit.BumpMapping
	parseCache    map[string][]parsedCommit
	overrideTypes map[string]struct{}
}

func newReleaseAnalyzer(core *releaseCore, history versionHistoryProvider) *releaseAnalyzer {
	return &releaseAnalyzer{
		core:          core,
		history:       history,
		bumpMapping:   core.cfg.BumpTypes.ToBumpMapping(),
		parseCache:    make(map[string][]parsedCommit),
		overrideTypes: knownCommitTypes(core.cfg),
	}
}

func analyze(
	ctx context.Context,
	core *releaseCore,
	source releaseSource,
	selection releaseSelection,
	extraTags []forge.TagRef,
) ([]TargetPlan, error) {
	a := newReleaseAnalyzer(core, source)

	scan, err := a.scanHistory(ctx, selection, extraTags)
	if err != nil {
		return nil, err
	}

	pathPlans, err := a.planPathTargets(ctx, scan, selection.pathTargetsToAnalyze)
	if err != nil {
		return nil, err
	}

	derivedPlans, err := a.planDerivedTargets(ctx, scan, selection.selectedTargets, pathPlans)
	if err != nil {
		return nil, err
	}

	plans := make([]TargetPlan, 0, len(pathPlans)+len(derivedPlans))
	plans = append(plans, orderedPlans(filterPlansByID(pathPlans, selection.pathTargetIDsToEmit))...)
	plans = append(plans, orderedPlans(derivedPlans)...)

	return plans, nil
}

func (a *releaseAnalyzer) scanHistory(
	ctx context.Context,
	selection releaseSelection,
	extraTags []forge.TagRef,
) (*historyScan, error) {
	tags, err := a.history.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	targets := a.sharedHistoryTargets(selection)

	scan := &historyScan{
		tags:         withExtraTags(tags, extraTags),
		extraTags:    extraTags,
		includePaths: needsPathFiltering(targets),
		reachable:    make(map[string]bool),
		commits:      make(map[string][]history.CommitEntry),
	}

	err = a.buildSharedHistoryIndex(ctx, scan, targets)
	if err != nil {
		return nil, err
	}

	return scan, nil
}

func withExtraTags(tags []string, extraTags []forge.TagRef) []string {
	if len(extraTags) == 0 {
		return tags
	}

	merged := slices.Clone(tags)
	known := make(map[string]struct{}, len(merged))

	for _, tag := range merged {
		known[tag] = struct{}{}
	}

	for _, extraTag := range extraTags {
		name := strings.TrimSpace(extraTag.Name)
		if name == "" {
			continue
		}

		if _, exists := known[name]; exists {
			continue
		}

		known[name] = struct{}{}
		merged = append(merged, name)
	}

	return merged
}

func publishedTagRefs(releases []FinalizedRelease) []forge.TagRef {
	refs := make([]forge.TagRef, 0, len(releases))

	for _, release := range releases {
		if release.Release == nil {
			continue
		}

		refs = append(refs, forge.TagRef{Name: release.Release.TagName, CommitSHA: release.CommitSHA})
	}

	return refs
}
