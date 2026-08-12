package release

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/version"
)

func (a *releaseAnalyzer) currentVersionFromReleaseHistory(
	ctx context.Context,
	scan *historyScan,
	target config.ResolvedTarget,
) (string, string, error) {
	refs := a.versionHistoryRefs(scan, target)

	for _, ref := range refs {
		currentVersion, usable, useErr := a.currentVersionFromReachableRef(ctx, scan, target, ref)
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

func (a *releaseAnalyzer) versionHistoryRefs(scan *historyScan, target config.ResolvedTarget) []string {
	return a.orderedVersionRefs(target, scan.tags, "")
}

func (a *releaseAnalyzer) currentVersionFromReachableRef(
	ctx context.Context,
	scan *historyScan,
	target config.ResolvedTarget,
	ref string,
) (string, bool, error) {
	currentVersion, ok := a.currentVersionFromRef(target, ref)
	if !ok {
		return "", false, nil
	}

	reachable, err := a.refReachableFromBranch(ctx, scan, ref)
	if err != nil {
		return "", false, err
	}

	if !reachable {
		return "", false, nil
	}

	return currentVersion, true, nil
}

func (a *releaseAnalyzer) currentVersionFromRef(target config.ResolvedTarget, ref string) (string, bool) {
	strategy := versionStrategyForResolvedTarget(target)

	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}

	currentVersion, err := strategy.strategy.Current(ref)
	if err != nil {
		return "", false
	}

	if strategy.strategy.SupportsPrerelease() && !a.channelRefAllowed(strategy.strategy, currentVersion) {
		return "", false
	}

	return currentVersion, true
}

func (a *releaseAnalyzer) channelRefAllowed(strategy version.Strategy, currentVersion string) bool {
	// A stable version belongs to every channel, so an active channel this config
	// does not define must not reject it.
	if strategy.PrereleaseAllowed(currentVersion, "") {
		return true
	}

	channelName := strings.TrimSpace(a.core.cfg.ActiveChannel)
	if channelName == "" {
		return false
	}

	channel, exists := a.core.cfg.Release.Channels[channelName]
	if !exists {
		return false
	}

	return strategy.PrereleaseAllowed(currentVersion, strings.TrimSpace(channel.Prerelease))
}

func (a *releaseAnalyzer) refReachableFromBranch(ctx context.Context, scan *historyScan, ref string) (bool, error) {
	if reachable, ok := scan.reachable[ref]; ok {
		return reachable, nil
	}

	history, err := a.history.GetCommitsSinceRefs(
		ctx,
		[]string{ref},
		a.core.cfg.Branch,
		scan.includePaths,
		scan.extraTags,
	)
	if err != nil {
		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	reachable := !slices.Contains(history.MissingRefs, ref)
	scan.reachable[ref] = reachable

	if reachable {
		scan.commits[commitCacheKey{
			ref:          ref,
			branch:       a.core.cfg.Branch,
			includePaths: scan.includePaths,
		}] = history.EntriesByRef[ref]
	}

	return reachable, nil
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

	return versionStrategyForResolvedTarget(target).strategy.Less(leftVersion, rightVersion, leftRef, rightRef)
}

func (a *releaseAnalyzer) branchAncestryError(target config.ResolvedTarget, ref string) error {
	return fmt.Errorf(
		"previous release ref %q is not reachable from release branch %q for target %q. "+
			"Verify the latest tag or release and branch ancestry: %w",
		ref,
		a.core.cfg.Branch,
		target.ID,
		&forge.CommitBoundaryNotFoundError{Ref: ref, Branch: a.core.cfg.Branch},
	)
}
