package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

type releaseVersionRefs struct {
	preferredRef string
	hasPreferred bool
	tags         []string
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
	refs := make([]string, 0)

	historyRefs, err := a.rawVersionHistoryRefs(ctx)
	if err != nil {
		return nil, err
	}

	if historyRefs.hasPreferred {
		refs = append(refs, historyRefs.preferredRef)
	}

	refs = append(refs, historyRefs.tags...)

	return a.orderedVersionRefs(target, refs, ""), nil
}

func (a *releaseAnalyzer) rawVersionHistoryRefs(ctx context.Context) (releaseVersionRefs, error) {
	if a.versionRefs != nil {
		return *a.versionRefs, nil
	}

	refs := releaseVersionRefs{}

	preferredRef, err := a.history.GetLatestVersionRef(ctx)
	if err != nil && !errors.Is(err, provider.ErrNoVersionRef) {
		return releaseVersionRefs{}, fmt.Errorf("get latest version ref: %w", err)
	}

	if err == nil {
		refs.preferredRef = preferredRef
		refs.hasPreferred = true
	}

	tags, err := a.history.ListTags(ctx)
	if err != nil {
		return releaseVersionRefs{}, fmt.Errorf("list tags: %w", err)
	}

	refs.tags = append([]string(nil), tags...)
	a.versionRefs = &refs

	return refs, nil
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
	strategy := versionStrategyForResolvedTarget(target)

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

	channelName := strings.TrimSpace(a.core.cfg.ActiveChannel)
	if channelName == "" {
		return prerelease == ""
	}

	if prerelease == "" {
		return true
	}

	channel, exists := a.core.cfg.Release.Channels[channelName]
	if !exists {
		return false
	}

	channelPrerelease := strings.TrimSpace(channel.Prerelease)

	return prerelease == channelPrerelease || strings.HasPrefix(prerelease, channelPrerelease+".")
}

func (a *releaseAnalyzer) refReachableFromBranch(ctx context.Context, ref string) (bool, error) {
	if reachable, ok := a.refReachable[ref]; ok {
		return reachable, nil
	}

	history, err := a.history.GetCommitsSinceRefs(ctx, []string{ref}, a.core.cfg.Branch, false)
	if err != nil {
		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	reachable := !slices.Contains(history.MissingRefs, ref)
	a.refReachable[ref] = reachable

	if reachable {
		a.commitCache[commitCacheKey{ref: ref, branch: a.core.cfg.Branch, includePaths: false}] = history.EntriesByRef[ref]
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
		&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: a.core.cfg.Branch},
	)
}
