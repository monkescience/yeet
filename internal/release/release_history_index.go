package release

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

type commitCacheKey struct {
	ref          string
	branch       string
	includePaths bool
}

type monorepoHistoryIndex struct {
	targets map[string]targetHistory
}

// sharedHistoryTopRefsLimit caps how many of each target's ordered refs the
// shared scan resolves up front. Three covers the realistic "latest tag was a
// hotfix on a release branch" fall-back depth. Rarer cases fall through to the
// per-target lookup, which walks the full ref list.
const sharedHistoryTopRefsLimit = 3

type targetHistory struct {
	currentVersion string
	ref            string
	entries        []provider.CommitEntry
}

//nolint:funlen // Shared history assembly keeps target/ref/error handling together.
func (a *releaseAnalyzer) buildSharedHistoryIndex(ctx context.Context, selection releaseSelection) error {
	targets := a.sharedHistoryTargets(selection)
	if len(targets) <= 1 {
		return nil
	}

	refsByTargetID := make(map[string][]string, len(targets))
	requiredRefs := make(map[string]struct{})

	for _, targetID := range sortedHistoryTargetIDs(targets) {
		target := targets[targetID]

		refs, err := a.versionHistoryRefs(ctx, target)
		if err != nil {
			return err
		}

		// Targets with no version refs need an unbounded scan, which disables
		// the provider's early-termination heuristic. Excluding them keeps the
		// shared scan bounded. They fall through to the per-target slow path.
		if len(refs) == 0 {
			continue
		}

		if len(refs) > sharedHistoryTopRefsLimit {
			refs = refs[:sharedHistoryTopRefsLimit]
		}

		refsByTargetID[targetID] = refs

		for _, ref := range refs {
			requiredRefs[ref] = struct{}{}
		}
	}

	if len(requiredRefs) == 0 {
		return nil
	}

	refs := make([]string, 0, len(requiredRefs))
	for ref := range requiredRefs {
		refs = append(refs, ref)
	}

	sort.Strings(refs)

	includePaths := needsPathFiltering(targets)

	history, err := a.history.GetCommitsSinceRefs(
		ctx,
		refs,
		a.core.cfg.Branch,
		includePaths,
	)
	if err != nil {
		return fmt.Errorf("get commits from branch %q: %w", a.core.cfg.Branch, err)
	}

	missingRefs := make(map[string]struct{}, len(history.MissingRefs))
	for _, ref := range history.MissingRefs {
		missingRefs[ref] = struct{}{}
	}

	if len(history.MissingRefs) > 0 {
		slog.WarnContext(ctx, "shared history scan: refs unreachable from branch",
			slog.String("branch", a.core.cfg.Branch),
			slog.Any("missing_refs", history.MissingRefs),
		)
	}

	for _, ref := range refs {
		_, missing := missingRefs[ref]
		a.refReachable[ref] = !missing
	}

	for ref, entries := range history.EntriesByRef {
		a.commitCache[commitCacheKey{
			ref:          ref,
			branch:       a.core.cfg.Branch,
			includePaths: includePaths,
		}] = entries
	}

	index := &monorepoHistoryIndex{targets: make(map[string]targetHistory, len(refsByTargetID))}

	for targetID, candidateRefs := range refsByTargetID {
		selected, ok := a.selectTargetHistory(targets[targetID], candidateRefs, history.EntriesByRef, missingRefs)
		if !ok {
			// Top refs were unreachable, let the per-target fallback walk the full ref list.
			continue
		}

		index.targets[targetID] = selected
	}

	a.historyIndex = index

	slog.DebugContext(ctx, "shared history index built",
		slog.String("branch", a.core.cfg.Branch),
		slog.Int("targets_total", len(targets)),
		slog.Int("targets_indexed", len(index.targets)),
		slog.Int("refs_requested", len(refs)),
		slog.Int("refs_missing", len(history.MissingRefs)),
	)

	return nil
}

func (a *releaseAnalyzer) sharedHistoryTargets(selection releaseSelection) map[string]config.ResolvedTarget {
	targets := make(map[string]config.ResolvedTarget, len(selection.pathTargetsToAnalyze))
	maps.Copy(targets, selection.pathTargetsToAnalyze)

	selectedTargetIDs := targetIDSet(selection.selectedTargets)

	for _, targetID := range sortedTargetIDs(a.core.targets, config.TargetTypeDerived) {
		target := a.core.targets[targetID]
		if len(selectedTargetIDs) > 0 && !derivedTargetEligible(target, selectedTargetIDs) {
			continue
		}

		targets[targetID] = target
	}

	return targets
}

func (a *releaseAnalyzer) selectTargetHistory(
	target config.ResolvedTarget,
	refs []string,
	entriesByRef map[string][]provider.CommitEntry,
	missingRefs map[string]struct{},
) (targetHistory, bool) {
	for _, ref := range refs {
		if _, missing := missingRefs[ref]; missing {
			continue
		}

		currentVersion, ok := a.currentVersionFromRef(target, ref)
		if !ok {
			continue
		}

		entries, exists := entriesByRef[ref]
		if !exists {
			continue
		}

		return targetHistory{currentVersion: currentVersion, ref: ref, entries: entries}, true
	}

	return targetHistory{}, false
}

func (a *releaseAnalyzer) sharedTargetHistory(target config.ResolvedTarget) (targetHistory, bool) {
	if a.historyIndex == nil {
		return targetHistory{}, false
	}

	history, exists := a.historyIndex.targets[target.ID]

	return history, exists
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

	history, err := a.history.GetCommitsSinceRefs(ctx, []string{ref}, branch, includePaths)
	if err != nil {
		return nil, fmt.Errorf("get commits from branch %q: %w", branch, err)
	}

	if slices.Contains(history.MissingRefs, ref) {
		return nil, fmt.Errorf(
			"previous release ref %q is not reachable from release branch %q. "+
				"Verify the latest tag or release and branch ancestry: %w",
			ref,
			branch,
			&provider.CommitBoundaryNotFoundError{Ref: ref, Branch: branch},
		)
	}

	entries := history.EntriesByRef[ref]
	a.commitCache[key] = entries

	return entries, nil
}
