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
	overrideCache map[string]commitOverrideResult
	overrideTypes map[string]struct{}
}

func newReleaseAnalyzer(core *releaseCore, history versionHistoryProvider) *releaseAnalyzer {
	return &releaseAnalyzer{
		core:          core,
		history:       history,
		bumpMapping:   core.cfg.BumpTypes.ToBumpMapping(),
		overrideCache: make(map[string]commitOverrideResult),
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

// scanHistory runs the ordered pass the whole analysis depends on: list tags,
// then resolve every target boundary the shared scan can cover in one range
// request. Its result carries the phase outputs the planning pass reads.
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
		commits:      make(map[commitCacheKey][]history.CommitEntry),
	}

	if err := a.buildSharedHistoryIndex(ctx, scan, targets); err != nil {
		return nil, err
	}

	return scan, nil
}

// withExtraTags folds in tags this run published itself. They come from the
// operation that created them, so they are known even when a forge tag listing
// has not caught up yet.
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
