package release

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

// When there is a single root-path target with no excludes, all commits belong to it
// and path data is unnecessary, avoiding N+1 per-commit API calls.
func needsPathFiltering(targets map[string]config.ResolvedTarget) bool {
	if len(targets) != 1 {
		return true
	}

	for _, target := range targets {
		if target.Path != "." || len(target.ExcludePaths) > 0 {
			return true
		}
	}

	return false
}

func (a *releaseAnalyzer) parseCommits(ctx context.Context, entries []provider.CommitEntry) ([]commit.Commit, error) {
	commits := make([]commit.Commit, 0, len(entries))

	for _, entry := range entries {
		override, err := a.commitOverride(ctx, entry)
		if err != nil {
			return nil, err
		}

		if override.found {
			commits = append(commits, override.commits...)

			continue
		}

		commits = append(commits, commit.Parse(ctx, entry.Hash, entry.Message))
	}

	return commits, nil
}

func (a *releaseAnalyzer) commitOverride(
	ctx context.Context,
	entry provider.CommitEntry,
) (commitOverrideResult, error) {
	hash := strings.TrimSpace(entry.Hash)
	if hash == "" {
		return commitOverrideResult{}, nil
	}

	if cached, exists := a.overrideCache[hash]; exists {
		return cached, nil
	}

	messages, found, err := commitOverrideMessages(ctx, entry.Message, a.overrideTypes)
	if err != nil {
		return commitOverrideResult{}, fmt.Errorf("parse commit override for %q: %w", hash, err)
	}

	if !found {
		result := commitOverrideResult{}
		a.overrideCache[hash] = result

		return result, nil
	}

	commits := make([]commit.Commit, 0, len(messages))
	for _, message := range messages {
		commits = append(commits, commit.Parse(ctx, hash, message))
	}

	result := commitOverrideResult{commits: commits, found: true}
	a.overrideCache[hash] = result

	return result, nil
}

func (a *releaseAnalyzer) planPathTargets(
	ctx context.Context,
	selectedTargets map[string]config.ResolvedTarget,
) (map[string]TargetPlan, error) {
	r := a.core
	plans := make(map[string]TargetPlan)

	for _, targetID := range sortedTargetIDs(selectedTargets, config.TargetTypePath) {
		target := r.targets[targetID]

		plan, shouldRelease, err := a.planDirectTarget(ctx, target)
		if err != nil {
			return nil, err
		}

		if !shouldRelease {
			continue
		}

		plans[targetID] = plan
	}

	return plans, nil
}

func (a *releaseAnalyzer) planDerivedTargets(
	ctx context.Context,
	selectedTargets map[string]config.ResolvedTarget,
	pathPlans map[string]TargetPlan,
) (map[string]TargetPlan, error) {
	r := a.core
	plans := make(map[string]TargetPlan)
	selectedTargetIDs := make(map[string]struct{}, len(selectedTargets))

	for targetID := range selectedTargets {
		selectedTargetIDs[targetID] = struct{}{}
	}

	for _, targetID := range sortedTargetIDs(r.targets, config.TargetTypeDerived) {
		target := r.targets[targetID]

		if len(selectedTargetIDs) > 0 && !derivedTargetEligible(target, selectedTargetIDs) {
			continue
		}

		_, explicitlySelected := selectedTargetIDs[targetID]
		includeDirectCommits := len(selectedTargetIDs) == 0 || explicitlySelected

		childPlans := make([]TargetPlan, 0, len(target.Includes))
		for _, includeID := range target.Includes {
			childPlan, exists := pathPlans[includeID]
			if !exists {
				continue
			}

			childPlans = append(childPlans, childPlan)
		}

		plan, shouldRelease, err := a.planDerivedTarget(
			ctx,
			target,
			childPlans,
			includeDirectCommits,
		)
		if err != nil {
			return nil, err
		}

		if !shouldRelease {
			continue
		}

		plans[targetID] = plan
	}

	return plans, nil
}

func (a *releaseAnalyzer) planDirectTarget(
	ctx context.Context,
	target config.ResolvedTarget,
) (TargetPlan, bool, error) {
	inputs, err := a.loadDirectPlanContext(ctx, target)
	if err != nil {
		return TargetPlan{}, false, err
	}

	bumpType := commit.DetermineBump(inputs.commits, a.bumpMapping)

	nextVersion, nextBumpType, shouldRelease, err := a.nextVersionPlan(
		target,
		inputs.commits,
		inputs.history.currentVersion,
		bumpType,
	)
	if err != nil {
		return TargetPlan{}, false, err
	}

	slog.DebugContext(ctx, "release plan decision",
		slog.String("target", target.ID),
		slog.Any("bump_type", bumpType),
		slog.Any("next_bump_type", nextBumpType),
		slog.String("next_version", nextVersion),
		slog.Bool("should_release", shouldRelease),
	)

	if !shouldRelease {
		return TargetPlan{}, false, nil
	}

	plan := a.newTargetPlan(
		ctx,
		target,
		inputs.history.currentVersion,
		nextVersion,
		nextBumpType,
		inputs.history.ref,
		inputs.entries,
		inputs.commits,
	)

	return plan, true, nil
}

type directPlanContext struct {
	history targetHistory
	entries []provider.CommitEntry
	commits []commit.Commit
}

func (a *releaseAnalyzer) loadDirectPlanContext(
	ctx context.Context,
	target config.ResolvedTarget,
) (directPlanContext, error) {
	history, err := a.loadTargetHistory(ctx, target, true)
	if err != nil {
		return directPlanContext{}, err
	}

	slog.DebugContext(ctx, "planning target",
		slog.String("target", target.ID),
		slog.String("current_version", history.currentVersion),
		slog.String("boundary_ref", history.ref),
		slog.String("branch", a.core.cfg.Branch),
	)

	entries := filterEntriesForTarget(history.entries, target)

	slog.DebugContext(ctx, "commits since boundary",
		slog.String("target", target.ID),
		slog.Int("total", len(history.entries)),
		slog.Int("filtered", len(entries)),
	)

	commits, err := a.parseCommits(ctx, entries)
	if err != nil {
		return directPlanContext{}, err
	}

	logParsedCommits(ctx, target.ID, commits)

	return directPlanContext{history: history, entries: entries, commits: commits}, nil
}

type derivedPlanContext struct {
	history              targetHistory
	directEntries        []provider.CommitEntry
	childEntries         []provider.CommitEntry
	directCommits        []commit.Commit
	childPlans           []TargetPlan
	includeDirectCommits bool
}

func (a *releaseAnalyzer) planDerivedTarget(
	ctx context.Context,
	target config.ResolvedTarget,
	childPlans []TargetPlan,
	includeDirectCommits bool,
) (TargetPlan, bool, error) {
	inputs, err := a.loadDerivedPlanInputs(ctx, target, childPlans, includeDirectCommits)
	if err != nil {
		return TargetPlan{}, false, err
	}

	nextVersion, bumpType, shouldRelease, err := a.derivedVersionPlan(target, inputs)
	if err != nil {
		return TargetPlan{}, false, err
	}

	if !shouldRelease {
		return TargetPlan{}, false, nil
	}

	return a.newDerivedTargetPlan(ctx, target, nextVersion, bumpType, inputs), true, nil
}

func (a *releaseAnalyzer) loadDerivedPlanInputs(
	ctx context.Context,
	target config.ResolvedTarget,
	childPlans []TargetPlan,
	includeDirectCommits bool,
) (derivedPlanContext, error) {
	history, err := a.loadTargetHistory(ctx, target, target.Path != "" || len(childPlans) > 0)
	if err != nil {
		return derivedPlanContext{}, err
	}

	directEntries := []provider.CommitEntry{}
	if includeDirectCommits && target.Path != "" {
		directEntries = filterEntriesForTarget(history.entries, target)
	}

	directCommits, err := a.parseCommits(ctx, directEntries)
	if err != nil {
		return derivedPlanContext{}, err
	}

	return derivedPlanContext{
		history:              history,
		directEntries:        directEntries,
		childEntries:         filterEntriesForPlans(history.entries, childPlans, a.core.targets),
		directCommits:        directCommits,
		childPlans:           childPlans,
		includeDirectCommits: includeDirectCommits,
	}, nil
}

func (a *releaseAnalyzer) loadTargetHistory(
	ctx context.Context,
	target config.ResolvedTarget,
	includeEntries bool,
) (targetHistory, error) {
	if history, ok := a.sharedTargetHistory(target); ok {
		if !includeEntries {
			history.entries = nil
		}

		return history, nil
	}

	if a.historyIndex != nil {
		slog.DebugContext(ctx, "shared history miss: per-target lookup",
			slog.String("target", target.ID),
		)
	}

	currentVersion, ref, err := a.currentVersionFromReleaseHistory(ctx, target)
	if err != nil {
		return targetHistory{}, err
	}

	history := targetHistory{currentVersion: currentVersion, ref: ref}
	if !includeEntries {
		return history, nil
	}

	history.entries, err = a.commitsSince(ctx, ref, a.core.cfg.Branch, needsPathFiltering(a.analyzedTargets))
	if err != nil {
		return targetHistory{}, err
	}

	return history, nil
}

func (a *releaseAnalyzer) derivedVersionPlan(
	target config.ResolvedTarget,
	inputs derivedPlanContext,
) (string, commit.BumpType, bool, error) {
	currentVersion := inputs.history.currentVersion
	directBumpType := commit.DetermineBump(inputs.directCommits, a.bumpMapping)

	directNextVersion, directNextBumpType, directShouldRelease, err := a.nextVersionPlan(
		target,
		inputs.directCommits,
		currentVersion,
		directBumpType,
	)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	finalBumpType := directNextBumpType
	for _, childPlan := range inputs.childPlans {
		if commit.CompareBump(childPlan.BumpType, finalBumpType) > 0 {
			finalBumpType = childPlan.BumpType
		}
	}

	if finalBumpType == commit.BumpNone {
		return "", commit.BumpNone, false, nil
	}

	if directShouldRelease && commit.CompareBump(finalBumpType, directNextBumpType) <= 0 {
		return directNextVersion, finalBumpType, true, nil
	}

	nextVersion, _, _, err := resolveNextVersion(
		versionStrategyForResolvedTarget(target),
		target.Versioning,
		currentVersionWithInitial(target, currentVersion),
		finalBumpType,
		"",
		a.core.activePrereleaseIdentifier(),
	)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	return nextVersion, finalBumpType, true, nil
}

func (a *releaseAnalyzer) newDerivedTargetPlan(
	ctx context.Context,
	target config.ResolvedTarget,
	nextVersion string,
	bumpType commit.BumpType,
	inputs derivedPlanContext,
) TargetPlan {
	plan := a.newTargetPlan(
		ctx,
		target,
		inputs.history.currentVersion,
		nextVersion,
		bumpType,
		inputs.history.ref,
		inputs.directEntries,
		inputs.directCommits,
	)
	plan.PRCompareRef = derivedPRCompareRef(
		inputs.history.entries,
		target,
		inputs.childPlans,
		inputs.includeDirectCommits,
		a.core.targets,
	)
	plan.commitHashes = uniqueEntryHashes(inputs.directEntries, inputs.childEntries)
	plan.CommitCount = derivedCommitCount(plan.commitHashes, inputs.directCommits, inputs.childPlans)
	plan.IncludedTargets = make([]string, 0, len(inputs.childPlans))

	for _, childPlan := range inputs.childPlans {
		plan.IncludedTargets = append(plan.IncludedTargets, childPlan.ID)
	}

	plan.Changelog = renderDerivedChangelog(
		ctx,
		target,
		plan.NextTag,
		inputs.history.ref,
		inputs.directCommits,
		inputs.childPlans,
		plan.PRCompareRef,
		derivedChangelogRelease,
		a.core.metadata,
	)
	plan.PRChangelog = renderDerivedChangelog(
		ctx,
		target,
		plan.NextTag,
		inputs.history.ref,
		inputs.directCommits,
		inputs.childPlans,
		plan.PRCompareRef,
		derivedChangelogPreview,
		a.core.metadata,
	)

	return plan
}

func derivedCommitCount(hashes []string, directCommits []commit.Commit, childPlans []TargetPlan) int {
	if len(hashes) > 0 {
		return len(hashes)
	}

	count := len(directCommits)
	for _, childPlan := range childPlans {
		count += childPlan.CommitCount
	}

	return count
}

func (a *releaseAnalyzer) newTargetPlan(
	ctx context.Context,
	target config.ResolvedTarget,
	currentVersion string,
	baseVersion string,
	bumpType commit.BumpType,
	ref string,
	entries []provider.CommitEntry,
	commits []commit.Commit,
) TargetPlan {
	strategy := versionStrategyForResolvedTarget(target)
	plan := TargetPlan{
		ID:             target.ID,
		Type:           target.Type,
		CurrentVersion: currentVersion,
		BumpType:       bumpType,
		ChangelogFile:  target.Changelog.File,
		commitHashes:   uniqueEntryHashes(entries),
		previousRef:    strings.TrimSpace(ref),
	}

	plan.CommitCount = len(plan.commitHashes)
	if plan.CommitCount == 0 {
		plan.CommitCount = len(commits)
	}

	setPlanVersions(&plan, strategy, baseVersion)

	entry := newTargetChangelogEntry(ctx, target, plan.NextTag, ref, commits, a.core.metadata)
	plan.Changelog = renderChangelogEntry(entry, ref, plan.NextTag, a.core.metadata)
	plan.PRChangelog = plan.Changelog

	if ref != "" && len(entries) > 0 {
		plan.PRCompareRef = strings.TrimSpace(entries[0].Hash)
		plan.PRChangelog = renderChangelogEntry(entry, ref, entries[0].Hash, a.core.metadata)
	}

	return plan
}

func setPlanVersions(plan *TargetPlan, strategy versionStrategy, nextVersion string) {
	plan.NextVersion = nextVersion
	plan.NextTag = strategy.prefix + nextVersion
}

func derivedPRCompareRef(
	entries []provider.CommitEntry,
	directTarget config.ResolvedTarget,
	childPlans []TargetPlan,
	includeDirectCommits bool,
	targets map[string]config.ResolvedTarget,
) string {
	compareTargets := make([]config.ResolvedTarget, 0, len(childPlans)+1)

	if includeDirectCommits && directTarget.Path != "" {
		compareTargets = append(compareTargets, directTarget)
	}

	for _, childPlan := range childPlans {
		childTarget, exists := targets[childPlan.ID]
		if !exists {
			continue
		}

		compareTargets = append(compareTargets, childTarget)
	}

	for _, entry := range entries {
		for _, compareTarget := range compareTargets {
			if !entryBelongsToTarget(entry, compareTarget) {
				continue
			}

			return strings.TrimSpace(entry.Hash)
		}
	}

	return ""
}

func filterEntriesForPlans(
	entries []provider.CommitEntry,
	plans []TargetPlan,
	targets map[string]config.ResolvedTarget,
) []provider.CommitEntry {
	includedTargets := make([]config.ResolvedTarget, 0, len(plans))

	for _, plan := range plans {
		target, exists := targets[plan.ID]
		if !exists {
			continue
		}

		includedTargets = append(includedTargets, target)
	}

	return filterEntriesForTargets(entries, includedTargets)
}

func filterEntriesForTargets(entries []provider.CommitEntry, targets []config.ResolvedTarget) []provider.CommitEntry {
	filteredEntries := make([]provider.CommitEntry, 0, len(entries))

	for _, entry := range entries {
		if slices.ContainsFunc(targets, func(target config.ResolvedTarget) bool {
			return entryBelongsToTarget(entry, target)
		}) {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	return filteredEntries
}

func uniqueEntryHashes(entryGroups ...[]provider.CommitEntry) []string {
	seen := make(map[string]struct{})
	hashes := make([]string, 0)

	for _, entries := range entryGroups {
		for _, entry := range entries {
			hash := strings.TrimSpace(entry.Hash)
			if hash == "" {
				continue
			}

			if _, exists := seen[hash]; exists {
				continue
			}

			seen[hash] = struct{}{}
			hashes = append(hashes, hash)
		}
	}

	return hashes
}

func logParsedCommits(ctx context.Context, targetID string, commits []commit.Commit) {
	for _, parsed := range commits {
		slog.DebugContext(ctx, "parsed commit",
			slog.String("target", targetID),
			slog.String("hash", parsed.Hash),
			slog.String("type", parsed.Type),
			slog.Bool("breaking", parsed.Breaking),
		)
	}
}

func filterEntriesForTarget(entries []provider.CommitEntry, target config.ResolvedTarget) []provider.CommitEntry {
	filteredEntries := make([]provider.CommitEntry, 0, len(entries))

	for _, entry := range entries {
		if !entryBelongsToTarget(entry, target) {
			continue
		}

		filteredEntries = append(filteredEntries, entry)
	}

	return filteredEntries
}

func entryBelongsToTarget(entry provider.CommitEntry, target config.ResolvedTarget) bool {
	if target.Path == "" {
		return false
	}

	if len(entry.Paths) == 0 {
		return target.Path == "."
	}

	for _, changedPath := range entry.Paths {
		normalizedPath := strings.TrimSpace(changedPath)
		if normalizedPath == "" {
			continue
		}

		if !config.RepoPathContains(target.Path, normalizedPath) {
			continue
		}

		isExcluded := slices.ContainsFunc(target.ExcludePaths, func(excludePath string) bool {
			return config.RepoPathContains(excludePath, normalizedPath)
		})

		if !isExcluded {
			return true
		}
	}

	return false
}
