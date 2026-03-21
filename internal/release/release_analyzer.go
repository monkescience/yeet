package release

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

type releaseAnalyzer struct {
	releaser *Releaser
}

func newReleaseAnalyzer(releaser *Releaser) *releaseAnalyzer {
	return &releaseAnalyzer{releaser: releaser}
}

func (a *releaseAnalyzer) analyze(
	ctx context.Context,
	preview bool,
	previewHashLength int,
) (*Result, error) {
	r := a.releaser
	result := &Result{}

	if preview && previewHashLength <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidPreviewHashLength, previewHashLength)
	}

	currentVersion, ref, err := a.currentVersionFromReleaseHistory(ctx)
	if err != nil {
		return nil, err
	}

	result.CurrentVersion = currentVersion

	entries, err := a.commitsSince(ctx, ref)
	if err != nil {
		return nil, err
	}

	commits := provider.ParseCommits(entries)
	result.CommitCount = len(commits)
	result.BumpType = commit.DetermineBump(commits)

	nextVersion, bumpType, shouldRelease, err := a.nextVersionPlan(commits, result.CurrentVersion, result.BumpType)
	if err != nil {
		return nil, err
	}

	result.BumpType = bumpType

	if !shouldRelease {
		return result, nil
	}

	err = a.setResultVersions(result, nextVersion, entries, preview, previewHashLength)
	if err != nil {
		return nil, err
	}

	r.setResultChangelogs(result, ref, entries, commits)

	return result, nil
}

func (a *releaseAnalyzer) nextVersionPlan(
	commits []commit.Commit,
	currentVersion string,
	bumpType commit.BumpType,
) (string, commit.BumpType, bool, error) {
	r := a.releaser

	releaseAsVersion, err := a.releaseAsVersion(commits)
	if err != nil {
		return "", commit.BumpNone, false, err
	}

	current := currentVersion
	if current == "" {
		if sv, ok := r.strategy.strategy.(*version.SemVer); ok {
			current = sv.InitialVersion()
		}
	}

	return r.resolveNextVersion(current, bumpType, releaseAsVersion)
}

func (a *releaseAnalyzer) releaseAsVersion(commits []commit.Commit) (string, error) {
	r := a.releaser

	if r.cfg.Versioning != config.VersioningSemver {
		return "", nil
	}

	return detectReleaseAs(commits)
}

func (a *releaseAnalyzer) currentVersionFromReleaseHistory(ctx context.Context) (string, string, error) {
	refs, err := a.versionHistoryRefs(ctx)
	if err != nil {
		return "", "", err
	}

	for _, ref := range refs {
		currentVersion, usable, useErr := a.currentVersionFromReachableRef(ctx, ref)
		if useErr != nil {
			return "", "", useErr
		}

		if usable {
			return currentVersion, ref, nil
		}
	}

	if len(refs) > 0 {
		return "", "", a.branchAncestryError(refs[0])
	}

	return "", "", nil
}

func (a *releaseAnalyzer) versionHistoryRefs(ctx context.Context) ([]string, error) {
	r := a.releaser
	refs := make([]string, 0)

	preferredRef, err := r.history.GetLatestVersionRef(ctx)
	if err != nil {
		if !errors.Is(err, provider.ErrNoVersionRef) {
			return nil, fmt.Errorf("get latest version ref: %w", err)
		}
	} else {
		refs = append(refs, preferredRef)
	}

	tags, err := r.history.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	refs = append(refs, tags...)

	return a.orderedVersionRefs(refs, ""), nil
}

func (a *releaseAnalyzer) currentVersionFromReachableRef(ctx context.Context, ref string) (string, bool, error) {
	currentVersion, ok := a.currentVersionFromRef(ref)
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

func (a *releaseAnalyzer) currentVersionFromRef(ref string) (string, bool) {
	r := a.releaser

	ref = strings.TrimSpace(ref)
	if ref == "" || isPreviewTag(ref, r.strategy.prefix) {
		return "", false
	}

	currentVersion, err := r.strategy.strategy.Current(ref)
	if err != nil {
		return "", false
	}

	return currentVersion, true
}

func (a *releaseAnalyzer) refReachableFromBranch(ctx context.Context, ref string) (bool, error) {
	r := a.releaser

	_, err := r.history.GetCommitsSince(ctx, ref, r.cfg.Branch)
	if err != nil {
		if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("validate version ref %q: %w", ref, err)
	}

	return true, nil
}

func (a *releaseAnalyzer) orderedVersionRefs(refs []string, excludeRef string) []string {
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

		if _, ok := a.currentVersionFromRef(ref); !ok {
			continue
		}

		orderedRefs = append(orderedRefs, ref)
		seen[ref] = struct{}{}
	}

	sort.SliceStable(orderedRefs, func(i, j int) bool {
		return a.versionRefLess(orderedRefs[j], orderedRefs[i])
	})

	return orderedRefs
}

func (a *releaseAnalyzer) versionRefLess(leftRef, rightRef string) bool {
	r := a.releaser

	leftVersion, ok := a.currentVersionFromRef(leftRef)
	if !ok {
		return false
	}

	rightVersion, ok := a.currentVersionFromRef(rightRef)
	if !ok {
		return false
	}

	if r.cfg.Versioning == config.VersioningCalVer {
		return calVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef)
	}

	return semVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef)
}

func (a *releaseAnalyzer) branchAncestryError(ref string) error {
	r := a.releaser

	return fmt.Errorf(
		"previous release ref %q is not reachable from release branch %q; "+
			"verify the latest tag/release and branch ancestry: %w",
		ref,
		r.cfg.Branch,
		&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: r.cfg.Branch},
	)
}

func (a *releaseAnalyzer) commitsSince(ctx context.Context, ref string) ([]provider.CommitEntry, error) {
	r := a.releaser

	entries, err := r.history.GetCommitsSince(ctx, ref, r.cfg.Branch)
	if err == nil {
		return entries, nil
	}

	if errors.Is(err, provider.ErrCommitBoundaryNotFound) {
		return nil, a.branchAncestryError(ref)
	}

	return nil, fmt.Errorf("get commits from branch %q: %w", r.cfg.Branch, err)
}

func (a *releaseAnalyzer) setResultVersions(
	result *Result,
	baseVersion string,
	entries []provider.CommitEntry,
	preview bool,
	previewHashLength int,
) error {
	r := a.releaser

	result.BaseVersion = baseVersion
	result.BaseTag = r.strategy.prefix + baseVersion
	result.NextVersion = baseVersion

	if !preview {
		result.NextTag = result.BaseTag

		return nil
	}

	if len(entries) == 0 {
		return fmt.Errorf("%w: no commit hash available", ErrInvalidPreviewHashLength)
	}

	hash, err := shortHash(entries[0].Hash, previewHashLength)
	if err != nil {
		return err
	}

	result.NextVersion = baseVersion + "+" + hash
	result.NextTag = r.strategy.prefix + result.NextVersion

	return nil
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

func calVerVersionRefLess(leftVersion, rightVersion, leftRef, rightRef string) bool {
	leftParts, err := parseCalVerVersion(leftVersion)
	if err != nil {
		return leftRef < rightRef
	}

	rightParts, err := parseCalVerVersion(rightVersion)
	if err != nil {
		return leftRef < rightRef
	}

	if leftParts[0] != rightParts[0] {
		return leftParts[0] < rightParts[0]
	}

	if leftParts[1] != rightParts[1] {
		return leftParts[1] < rightParts[1]
	}

	if leftParts[2] != rightParts[2] {
		return leftParts[2] < rightParts[2]
	}

	return leftRef < rightRef
}

func parseCalVerVersion(rawVersion string) ([3]int, error) {
	parts := strings.SplitN(strings.TrimSpace(rawVersion), ".", 3) //nolint:mnd // calver has 3 segments
	if len(parts) != 3 {                                           //nolint:mnd // calver has 3 segments
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
