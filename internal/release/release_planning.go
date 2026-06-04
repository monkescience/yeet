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

	body, found, err := a.releaser.prs.CommitPullRequestBody(ctx, hash)
	if err != nil {
		return commitOverrideResult{}, fmt.Errorf("find commit override for %q: %w", hash, err)
	}

	if !found {
		result := commitOverrideResult{}
		a.overrideCache[hash] = result

		return result, nil
	}

	messages, found, err := commitOverrideMessages(ctx, body, a.overrideTypes)
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
	r := a.releaser
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
	r := a.releaser
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

//nolint:funlen // Direct target planning is straight-through. Debug logs make it longer but no clearer to split.
func (a *releaseAnalyzer) planDirectTarget(
	ctx context.Context,
	target config.ResolvedTarget,
) (TargetPlan, bool, error) {
	var (
		entries        []provider.CommitEntry
		currentVersion string
		ref            string
		err            error
	)

	sharedHistory, ok := a.sharedTargetHistory(target)
	if ok {
		currentVersion = sharedHistory.currentVersion
		ref = sharedHistory.ref
		entries = sharedHistory.entries
	} else {
		if a.historyIndex != nil {
			slog.DebugContext(ctx, "shared history miss: per-target lookup",
				slog.String("target", target.ID),
			)
		}

		currentVersion, ref, err = a.currentVersionFromReleaseHistory(ctx, target)
		if err != nil {
			return TargetPlan{}, false, err
		}
	}

	slog.DebugContext(ctx, "planning target",
		slog.String("target", target.ID),
		slog.String("current_version", currentVersion),
		slog.String("boundary_ref", ref),
		slog.String("branch", a.releaser.cfg.Branch),
	)

	if !ok {
		entries, err = a.commitsSince(ctx, ref, a.releaser.cfg.Branch, needsPathFiltering(a.analyzedTargets))
		if err != nil {
			return TargetPlan{}, false, err
		}
	}

	filteredEntries := filterEntriesForTarget(entries, target)

	slog.DebugContext(ctx, "commits since boundary",
		slog.String("target", target.ID),
		slog.Int("total", len(entries)),
		slog.Int("filtered", len(filteredEntries)),
	)

	commits, err := a.parseCommits(ctx, filteredEntries)
	if err != nil {
		return TargetPlan{}, false, err
	}

	logParsedCommits(ctx, target.ID, commits)

	bumpType := commit.DetermineBump(commits, a.bumpMapping)

	nextVersion, nextBumpType, shouldRelease, err := a.nextVersionPlan(target, commits, currentVersion, bumpType)
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
		currentVersion,
		nextVersion,
		nextBumpType,
		ref,
		filteredEntries,
		commits,
	)

	return plan, true, nil
}

//nolint:funlen // Derived target planning keeps child and direct-commit logic together.
func (a *releaseAnalyzer) planDerivedTarget(
	ctx context.Context,
	target config.ResolvedTarget,
	childPlans []TargetPlan,
	includeDirectCommits bool,
) (TargetPlan, bool, error) {
	var (
		currentVersion string
		ref            string
		err            error
	)

	sharedHistory, ok := a.sharedTargetHistory(target)
	if ok {
		currentVersion = sharedHistory.currentVersion
		ref = sharedHistory.ref
	} else {
		if a.historyIndex != nil {
			slog.DebugContext(ctx, "shared history miss: per-target lookup",
				slog.String("target", target.ID),
			)
		}

		currentVersion, ref, err = a.currentVersionFromReleaseHistory(ctx, target)
		if err != nil {
			return TargetPlan{}, false, err
		}
	}

	allEntries := []provider.CommitEntry{}

	if target.Path != "" || len(childPlans) > 0 {
		if ok {
			allEntries = sharedHistory.entries
		} else {
			entries, commitsErr := a.commitsSince(ctx, ref, a.releaser.cfg.Branch, needsPathFiltering(a.analyzedTargets))
			if commitsErr != nil {
				return TargetPlan{}, false, commitsErr
			}

			allEntries = entries
		}
	}

	directEntries := []provider.CommitEntry{}

	if includeDirectCommits && target.Path != "" {
		directEntries = filterEntriesForTarget(allEntries, target)
	}

	childEntries := filterEntriesForPlans(allEntries, childPlans, a.releaser.targets)

	directCommits, err := a.parseCommits(ctx, directEntries)
	if err != nil {
		return TargetPlan{}, false, err
	}

	directBumpType := commit.DetermineBump(directCommits, a.bumpMapping)

	directNextVersion, directNextBumpType, directShouldRelease, err := a.nextVersionPlan(
		target,
		directCommits,
		currentVersion,
		directBumpType,
	)
	if err != nil {
		return TargetPlan{}, false, err
	}

	finalBumpType := directNextBumpType
	for _, childPlan := range childPlans {
		if releaseBumpOrder(childPlan.BumpType) > releaseBumpOrder(finalBumpType) {
			finalBumpType = childPlan.BumpType
		}
	}

	if finalBumpType == commit.BumpNone {
		return TargetPlan{}, false, nil
	}

	nextVersion := directNextVersion
	if !directShouldRelease || releaseBumpOrder(finalBumpType) > releaseBumpOrder(directNextBumpType) {
		nextVersion, err = versionStrategyForResolvedTarget(target).strategy.Next(
			currentVersionWithInitial(target, currentVersion),
			finalBumpType,
		)
		if err != nil {
			return TargetPlan{}, false, fmt.Errorf("calculate next version: %w", err)
		}
	}

	plan := a.newTargetPlan(
		ctx,
		target,
		currentVersion,
		nextVersion,
		finalBumpType,
		ref,
		directEntries,
		directCommits,
	)
	plan.PRCompareRef = derivedPRCompareRef(
		allEntries,
		target,
		childPlans,
		includeDirectCommits,
		a.releaser.targets,
	)
	plan.commitHashes = uniqueEntryHashes(directEntries, childEntries)

	plan.CommitCount = len(plan.commitHashes)
	if plan.CommitCount == 0 {
		plan.CommitCount = len(directCommits)

		for _, childPlan := range childPlans {
			plan.CommitCount += childPlan.CommitCount
		}
	}

	plan.IncludedTargets = make([]string, 0, len(childPlans))
	plan.Changelog = renderDerivedChangelog(
		ctx,
		target,
		plan.NextTag,
		ref,
		directCommits,
		childPlans,
		plan.PRCompareRef,
		false,
		a.releaser,
	)
	plan.PRChangelog = renderDerivedChangelog(
		ctx,
		target,
		plan.NextTag,
		ref,
		directCommits,
		childPlans,
		plan.PRCompareRef,
		true,
		a.releaser,
	)

	for _, childPlan := range childPlans {
		plan.IncludedTargets = append(plan.IncludedTargets, childPlan.ID)
	}

	return plan, true, nil
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
		Type:           string(target.Type),
		Path:           target.Path,
		CurrentVersion: currentVersion,
		BumpType:       bumpType,
		Files: map[string]string{
			changelogFileKey: target.Changelog.File,
		},
		commitHashes: uniqueEntryHashes(entries),
	}

	plan.CommitCount = len(plan.commitHashes)
	if plan.CommitCount == 0 {
		plan.CommitCount = len(commits)
	}

	setPlanVersions(&plan, strategy, baseVersion)

	plan.Changelog = renderTargetChangelog(ctx, target, plan.NextTag, ref, plan.NextTag, commits, a.releaser)
	plan.PRChangelog = plan.Changelog

	if ref != "" && len(entries) > 0 {
		plan.PRCompareRef = strings.TrimSpace(entries[0].Hash)
		plan.PRChangelog = renderTargetChangelog(ctx, target, plan.NextTag, ref, entries[0].Hash, commits, a.releaser)
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
			slog.String("description", parsed.Description),
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
