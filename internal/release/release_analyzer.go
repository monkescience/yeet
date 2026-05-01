package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

type releaseAnalyzer struct {
	releaser        *Releaser
	bumpMapping     commit.BumpMapping
	commitCache     map[commitCacheKey][]provider.CommitEntry
	overrideCache   map[string]commitOverrideResult
	analyzedTargets map[string]config.ResolvedTarget
}

type commitCacheKey struct {
	ref          string
	branch       string
	includePaths bool
}

type releaseSelection struct {
	explicitTargets     map[string]config.ResolvedTarget
	analyzedPathTargets map[string]config.ResolvedTarget
	emitPathTargetIDs   map[string]struct{}
}

func newReleaseAnalyzer(releaser *Releaser) *releaseAnalyzer {
	return &releaseAnalyzer{
		releaser:      releaser,
		bumpMapping:   releaser.cfg.BumpTypes.ToBumpMapping(),
		commitCache:   make(map[commitCacheKey][]provider.CommitEntry),
		overrideCache: make(map[string]commitOverrideResult),
	}
}

// needsPathFiltering returns true when commit paths are required for target filtering.
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

func (a *releaseAnalyzer) analyze(ctx context.Context, selectedTargetIDs []string) (*Result, error) {
	r := a.releaser
	result := &Result{BaseBranch: r.cfg.Branch}

	selection, err := a.selectTargets(selectedTargetIDs)
	if err != nil {
		return nil, err
	}

	a.analyzedTargets = selection.analyzedPathTargets

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

		commits = append(commits, commit.Parse(entry.Hash, entry.Message))
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

	body, found, err := a.releaser.overrides.CommitPullRequestBody(ctx, hash)
	if err != nil {
		return commitOverrideResult{}, fmt.Errorf("find commit override for %q: %w", hash, err)
	}

	if !found {
		result := commitOverrideResult{}
		a.overrideCache[hash] = result

		return result, nil
	}

	messages, found, err := commitOverrideMessages(body)
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
		commits = append(commits, commit.Parse(hash, message))
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

func (a *releaseAnalyzer) planDirectTarget(
	ctx context.Context,
	target config.ResolvedTarget,
) (TargetPlan, bool, error) {
	currentVersion, ref, err := a.currentVersionFromReleaseHistory(ctx, target)
	if err != nil {
		return TargetPlan{}, false, err
	}

	entries, err := a.commitsSince(ctx, ref, a.releaser.cfg.Branch, needsPathFiltering(a.analyzedTargets))
	if err != nil {
		return TargetPlan{}, false, err
	}

	filteredEntries := filterEntriesForTarget(entries, target)

	commits, err := a.parseCommits(ctx, filteredEntries)
	if err != nil {
		return TargetPlan{}, false, err
	}

	bumpType := commit.DetermineBump(commits, a.bumpMapping)

	nextVersion, nextBumpType, shouldRelease, err := a.nextVersionPlan(target, commits, currentVersion, bumpType)
	if err != nil {
		return TargetPlan{}, false, err
	}

	if !shouldRelease {
		return TargetPlan{}, false, nil
	}

	plan := a.newTargetPlan(
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
	currentVersion, ref, err := a.currentVersionFromReleaseHistory(ctx, target)
	if err != nil {
		return TargetPlan{}, false, err
	}

	allEntries := []provider.CommitEntry{}

	if target.Path != "" || len(childPlans) > 0 {
		entries, commitsErr := a.commitsSince(ctx, ref, a.releaser.cfg.Branch, needsPathFiltering(a.analyzedTargets))
		if commitsErr != nil {
			return TargetPlan{}, false, commitsErr
		}

		allEntries = entries
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
		nextVersion, err = a.releaser.strategyForTarget(target).strategy.Next(
			currentVersionWithInitial(target, currentVersion),
			finalBumpType,
		)
		if err != nil {
			return TargetPlan{}, false, fmt.Errorf("calculate next version: %w", err)
		}
	}

	plan := a.newTargetPlan(
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
	target config.ResolvedTarget,
	currentVersion string,
	baseVersion string,
	bumpType commit.BumpType,
	ref string,
	entries []provider.CommitEntry,
	commits []commit.Commit,
) TargetPlan {
	strategy := a.releaser.strategyForTarget(target)
	plan := TargetPlan{
		ID:             target.ID,
		Type:           string(target.Type),
		Path:           target.Path,
		CurrentVersion: currentVersion,
		BumpType:       bumpType,
		Files: map[string]string{
			"changelog_file": target.Changelog.File,
		},
		commitHashes: uniqueEntryHashes(entries),
	}

	plan.CommitCount = len(plan.commitHashes)
	if plan.CommitCount == 0 {
		plan.CommitCount = len(commits)
	}

	setPlanVersions(&plan, strategy, baseVersion)

	plan.Changelog = renderTargetChangelog(target, plan.NextTag, ref, plan.NextTag, commits, a.releaser)
	plan.PRChangelog = plan.Changelog

	if ref != "" && len(entries) > 0 {
		plan.PRCompareRef = strings.TrimSpace(entries[0].Hash)
		plan.PRChangelog = renderTargetChangelog(target, plan.NextTag, ref, entries[0].Hash, commits, a.releaser)
	}

	return plan
}

func setPlanVersions(plan *TargetPlan, strategy versionStrategy, nextVersion string) {
	plan.NextVersion = nextVersion
	plan.NextTag = strategy.prefix + nextVersion
}

func renderTargetChangelog(
	target config.ResolvedTarget,
	nextTag, ref, compareTarget string,
	commits []commit.Commit,
	releaser *Releaser,
) string {
	gen := &changelog.Generator{
		Sections:   target.Changelog.Sections,
		Include:    target.Changelog.Include,
		RepoURL:    releaser.metadata.RepoURL(),
		PathPrefix: releaser.metadata.PathPrefix(),
		References: target.Changelog.References,
	}

	entry := gen.Generate(nextTag, ref, commits)
	if ref != "" && compareTarget != "" {
		entry.CompareURL = compareURL(releaser.metadata.RepoURL(), releaser.metadata.PathPrefix(), ref, compareTarget)
	}

	return changelog.Render(entry)
}

func renderDerivedChangelog(
	target config.ResolvedTarget,
	nextTag string,
	ref string,
	directCommits []commit.Commit,
	childPlans []TargetPlan,
	prCompareRef string,
	prMode bool,
	releaser *Releaser,
) string {
	var body strings.Builder

	if len(directCommits) > 0 {
		directEntry := renderTargetChangelog(target, nextTag, ref, nextTag, directCommits, releaser)
		body.WriteString(changelogBodyWithoutHeading(directEntry))
	}

	for _, childPlan := range childPlans {
		if body.Len() > 0 && !strings.HasSuffix(body.String(), "\n\n") {
			body.WriteString("\n\n")
		}

		fmt.Fprintf(&body, "### %s\n\n", childPlan.ID)

		childChangelog := childPlan.Changelog
		if prMode && childPlan.PRChangelog != "" {
			childChangelog = childPlan.PRChangelog
		}

		body.WriteString(strings.TrimSpace(changelogBodyWithoutHeading(childChangelog)))
		body.WriteString("\n")
	}

	entry := changelog.Entry{
		Version: nextTag,
		Body:    strings.TrimSpace(body.String()) + "\n",
	}

	if ref != "" {
		compareTarget := nextTag
		if prMode {
			compareTarget = prCompareRef
		}

		if compareTarget != "" {
			entry.CompareURL = compareURL(releaser.metadata.RepoURL(), releaser.metadata.PathPrefix(), ref, compareTarget)
		}
	}

	return changelog.Render(entry)
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

func changelogBodyWithoutHeading(renderedEntry string) string {
	lines := strings.Split(strings.ReplaceAll(renderedEntry, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
		}
	}

	return strings.TrimSpace(renderedEntry)
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

func currentVersionOrInitial(target config.ResolvedTarget) string {
	strategy := versionStrategyForResolvedTarget(target)
	if semverStrategy, ok := strategy.strategy.(*version.SemVer); ok {
		return semverStrategy.InitialVersion()
	}

	return ""
}

func versionStrategyForResolvedTarget(target config.ResolvedTarget) versionStrategy {
	var strategy version.Strategy

	switch target.Versioning {
	case config.VersioningCalVer:
		strategy = &version.CalVer{
			Format: target.CalVer.Format,
			Prefix: target.TagPrefix,
		}
	case config.VersioningSemver:
		strategy = &version.SemVer{
			Prefix:                     target.TagPrefix,
			PreMajorBreakingBumpsMinor: target.PreMajorBreakingBumpsMinor,
			PreMajorFeaturesBumpPatch:  target.PreMajorFeaturesBumpPatch,
		}
	}

	return versionStrategy{strategy: strategy, prefix: target.TagPrefix}
}

func currentVersionWithInitial(target config.ResolvedTarget, currentVersion string) string {
	if currentVersion != "" {
		return currentVersion
	}

	return currentVersionOrInitial(target)
}

func (a *releaseAnalyzer) nextVersionPlan(
	target config.ResolvedTarget,
	commits []commit.Commit,
	currentVersion string,
	bumpType commit.BumpType,
) (string, commit.BumpType, bool, error) {
	strategy := a.releaser.strategyForTarget(target)

	releaseAsVersion, err := a.releaseAsVersion(target, commits)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	current := currentVersionWithInitial(target, currentVersion)

	return resolveNextVersion(
		strategy,
		target.Versioning,
		current,
		bumpType,
		releaseAsVersion,
		a.releaser.activePrereleaseIdentifier(),
	)
}

func resolveNextVersion(
	strategy versionStrategy,
	versioning config.VersioningStrategy,
	current string,
	bump commit.BumpType,
	releaseAsVersion string,
	prereleaseIdentifier string,
) (string, commit.BumpType, bool, error) {
	if prereleaseIdentifier != "" && versioning == config.VersioningSemver {
		return resolveNextPrereleaseVersion(strategy, current, bump, releaseAsVersion, prereleaseIdentifier)
	}

	if releaseAsVersion != "" && versioning == config.VersioningSemver {
		nextVersion, overrideBump, err := applyReleaseAs(current, releaseAsVersion)
		if err != nil {
			return "", commit.BumpNone, false, err
		}

		return nextVersion, overrideBump, true, nil
	}

	if bump == commit.BumpNone {
		return "", bump, false, nil
	}

	nextVersion, err := strategy.strategy.Next(current, bump)
	if err != nil {
		return "", commit.BumpNone, false, fmt.Errorf("calculate next version: %w", err)
	}

	return nextVersion, bump, true, nil
}

func (a *releaseAnalyzer) releaseAsVersion(target config.ResolvedTarget, commits []commit.Commit) (string, error) {
	if target.Versioning != config.VersioningSemver {
		return "", nil
	}

	return detectReleaseAs(commits)
}

func (a *releaseAnalyzer) currentVersionFromReleaseHistory(
	ctx context.Context,
	target config.ResolvedTarget,
) (string, string, error) {
	refs, err := a.versionHistoryRefs(ctx, target)
	if err != nil {
		return "", "", err
	}

	for _, ref := range refs {
		currentVersion, usable, useErr := a.currentVersionFromReachableRef(ctx, target, ref)
		if useErr != nil {
			return "", "", useErr
		}

		if usable {
			return currentVersion, ref, nil
		}
	}

	if len(refs) > 0 {
		return "", "", a.branchAncestryError(target, refs[0])
	}

	return "", "", nil
}

func (a *releaseAnalyzer) versionHistoryRefs(ctx context.Context, target config.ResolvedTarget) ([]string, error) {
	r := a.releaser
	refs := make([]string, 0)

	preferredRef, err := r.history.GetLatestVersionRef(ctx)
	if err != nil && !errors.Is(err, provider.ErrNoVersionRef) {
		return nil, fmt.Errorf("get latest version ref: %w", err)
	}

	if err == nil {
		refs = append(refs, preferredRef)
	}

	tags, err := r.history.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	refs = append(refs, tags...)

	return a.orderedVersionRefs(target, refs, ""), nil
}

func (a *releaseAnalyzer) currentVersionFromReachableRef(
	ctx context.Context,
	target config.ResolvedTarget,
	ref string,
) (string, bool, error) {
	currentVersion, ok := a.currentVersionFromRef(target, ref)
	if !ok {
		return "", false, nil
	}

	reachable, err := a.refReachableFromBranch(ctx, ref)
	if err != nil {
		return "", false, err
	}

	if !reachable {
		return "", false, nil
	}

	return currentVersion, true, nil
}

func (a *releaseAnalyzer) currentVersionFromRef(target config.ResolvedTarget, ref string) (string, bool) {
	strategy := a.releaser.strategyForTarget(target)

	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}

	currentVersion, err := strategy.strategy.Current(ref)
	if err != nil {
		return "", false
	}

	if target.Versioning == config.VersioningSemver && !a.semverRefAllowed(currentVersion) {
		return "", false
	}

	return currentVersion, true
}

func (a *releaseAnalyzer) semverRefAllowed(currentVersion string) bool {
	parsedVersion, err := semver.StrictNewVersion(currentVersion)
	if err != nil {
		return false
	}

	prerelease := strings.TrimSpace(parsedVersion.Prerelease())

	channelName := strings.TrimSpace(a.releaser.cfg.ActiveChannel)
	if channelName == "" {
		return prerelease == ""
	}

	if prerelease == "" {
		return true
	}

	channel, exists := a.releaser.cfg.Release.Channels[channelName]
	if !exists {
		return false
	}

	channelPrerelease := strings.TrimSpace(channel.Prerelease)

	return prerelease == channelPrerelease || strings.HasPrefix(prerelease, channelPrerelease+".")
}

func (a *releaseAnalyzer) refReachableFromBranch(ctx context.Context, ref string) (bool, error) {
	r := a.releaser

	_, err := r.history.GetCommitsSince(ctx, ref, r.cfg.Branch, false)
	if err != nil {
		if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	return true, nil
}

func (a *releaseAnalyzer) orderedVersionRefs(
	target config.ResolvedTarget,
	refs []string,
	excludeRef string,
) []string {
	orderedRefs := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	excludeRef = strings.TrimSpace(excludeRef)

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == excludeRef {
			continue
		}

		if _, exists := seen[ref]; exists {
			continue
		}

		if _, ok := a.currentVersionFromRef(target, ref); !ok {
			continue
		}

		orderedRefs = append(orderedRefs, ref)
		seen[ref] = struct{}{}
	}

	sort.SliceStable(orderedRefs, func(leftIdx, rightIdx int) bool {
		return a.versionRefLess(target, orderedRefs[rightIdx], orderedRefs[leftIdx])
	})

	return orderedRefs
}

func (a *releaseAnalyzer) versionRefLess(target config.ResolvedTarget, leftRef, rightRef string) bool {
	leftVersion, ok := a.currentVersionFromRef(target, leftRef)
	if !ok {
		return false
	}

	rightVersion, ok := a.currentVersionFromRef(target, rightRef)
	if !ok {
		return false
	}

	if target.Versioning == config.VersioningCalVer {
		return calVerVersionRefLess(target.CalVer.Format, leftVersion, rightVersion, leftRef, rightRef)
	}

	return semVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef)
}

func (a *releaseAnalyzer) branchAncestryError(target config.ResolvedTarget, ref string) error {
	return fmt.Errorf(
		"previous release ref %q is not reachable from release branch %q for target %q; "+
			"verify the latest tag/release and branch ancestry: %w",
		ref,
		a.releaser.cfg.Branch,
		target.ID,
		&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: a.releaser.cfg.Branch},
	)
}

func (a *releaseAnalyzer) commitsSince(
	ctx context.Context,
	ref, branch string,
	includePaths bool,
) ([]provider.CommitEntry, error) {
	key := commitCacheKey{ref: ref, branch: branch, includePaths: includePaths}
	if cached, exists := a.commitCache[key]; exists {
		return cached, nil
	}

	r := a.releaser

	entries, err := r.history.GetCommitsSince(ctx, ref, branch, includePaths)
	if err == nil {
		a.commitCache[key] = entries

		return entries, nil
	}

	if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
		return nil, fmt.Errorf(
			"previous release ref %q is not reachable from release branch %q; "+
				"verify the latest tag/release and branch ancestry: %w",
			ref,
			branch,
			&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: branch},
		)
	}

	return nil, fmt.Errorf("get commits from branch %q: %w", branch, err)
}

func semVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef string) bool {
	leftSemver, err := semver.StrictNewVersion(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightSemver, err := semver.StrictNewVersion(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if !leftSemver.Equal(rightSemver) {
		return leftSemver.LessThan(rightSemver)
	}

	return leftRef < rightRef
}

func calVerVersionRefLess(format, leftVersion, rightVersion, leftRef, rightRef string) bool {
	calver := &version.CalVer{Format: format}

	return calver.Less(leftVersion, rightVersion, leftRef, rightRef)
}

func parseCalVerVersion(rawVersion string) ([3]int, error) {
	calver := &version.CalVer{Format: version.DefaultCalVerFormat}

	normalizedVersion, err := calver.Current(strings.TrimSpace(rawVersion))
	if err != nil {
		return [3]int{}, fmt.Errorf("parse calver version %q: %w", rawVersion, err)
	}

	parts := strings.SplitN(normalizedVersion, ".", 3) //nolint:mnd // default calver has 3 segments
	if len(parts) != 3 {                               //nolint:mnd // default calver has 3 segments
		return [3]int{}, fmt.Errorf("%w: %q", version.ErrInvalidVersion, rawVersion)
	}

	values := [3]int{}

	for idx, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{}, fmt.Errorf("%w: %q: %w", version.ErrInvalidVersion, rawVersion, err)
		}

		values[idx] = value
	}

	return values, nil
}
