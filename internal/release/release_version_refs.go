package release

import (
	"context"
	"fmt"
	"slices"
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
	return a.core.strategies[target.ID].OrderedRefs(scan.tags, a.core.run.prerelease)
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
	strategy := a.core.strategies[target.ID]

	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}

	currentVersion, err := strategy.Current(ref)
	if err != nil {
		return "", false
	}

	if strategy.SupportsPrerelease() && !a.channelRefAllowed(strategy, currentVersion) {
		return "", false
	}

	return currentVersion, true
}

func (a *releaseAnalyzer) channelRefAllowed(strategy version.Strategy, currentVersion string) bool {
	return strategy.PrereleaseAllowed(currentVersion, a.core.run.prerelease)
}

func (a *releaseAnalyzer) refReachableFromBranch(ctx context.Context, scan *historyScan, ref string) (bool, error) {
	if reachable, ok := scan.reachable[ref]; ok {
		return reachable, nil
	}

	history, err := a.history.GetCommitsSinceRefs(
		ctx,
		[]string{ref},
		scan.includePaths,
		scan.extraTags,
	)
	if err != nil {
		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	reachable := !slices.Contains(history.MissingRefs, ref)
	scan.reachable[ref] = reachable

	if reachable {
		scan.commits[ref] = history.EntriesByRef[ref]
	}

	return reachable, nil
}

func (a *releaseAnalyzer) branchAncestryError(target config.ResolvedTarget, ref string) error {
	return fmt.Errorf(
		"previous release ref %q is not reachable from release branch %q for target %q. "+
			"Verify the latest tag or release and branch ancestry: %w",
		ref,
		a.core.run.baseBranch,
		target.ID,
		&forge.CommitBoundaryNotFoundError{Ref: ref, Branch: a.core.run.baseBranch, Target: target.ID},
	)
}
