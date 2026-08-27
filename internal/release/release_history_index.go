package release

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

// historyScan holds the outputs of one ordered history pass: the tag list every
// ref order derives from, the boundaries the shared range request resolved, and
// the reachability and range results later per-target lookups reuse.
type historyScan struct {
	tags         []string
	extraTags    []forge.TagRef
	includePaths bool
	index        map[string]targetHistory
	reachable    map[string]bool
	commits      map[string][]history.CommitEntry
}

// sharedHistoryTopRefsLimit caps how many of each target's ordered refs the
// shared scan resolves up front. Three covers the realistic "latest tag was a
// hotfix on a release branch" fall-back depth. Rarer cases fall through to the
// per-target lookup, which walks the full ref list.
const sharedHistoryTopRefsLimit = 3

type targetHistory struct {
	currentVersion string
	ref            string
	entries        []history.CommitEntry
}

//nolint:funlen // Shared history assembly keeps target/ref/error handling together.
func (a *releaseAnalyzer) buildSharedHistoryIndex(
	ctx context.Context,
	scan *historyScan,
	targets map[string]config.ResolvedTarget,
) error {
	if len(targets) <= 1 {
		return nil
	}

	refsByTargetID := make(map[string][]string, len(targets))
	requiredRefs := make(map[string]struct{})

	for _, targetID := range sortedHistoryTargetIDs(targets) {
		target := targets[targetID]

		refs := a.versionHistoryRefs(scan, target)

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

	scanned, err := a.history.GetCommitsSinceRefs(
		ctx,
		refs,
		scan.includePaths,
		scan.extraTags,
	)
	if err != nil {
		return fmt.Errorf("get commits from branch %q: %w", a.core.run.baseBranch, err)
	}

	missingRefs := make(map[string]struct{}, len(scanned.MissingRefs))
	for _, ref := range scanned.MissingRefs {
		missingRefs[ref] = struct{}{}
	}

	if len(scanned.MissingRefs) > 0 {
		slog.WarnContext(ctx, "shared history scan: refs unreachable from branch",
			slog.String("branch", a.core.run.baseBranch),
			slog.Any("missing_refs", scanned.MissingRefs),
		)
	}

	for _, ref := range refs {
		_, missing := missingRefs[ref]
		scan.reachable[ref] = !missing
	}

	maps.Copy(scan.commits, scanned.EntriesByRef)

	index := make(map[string]targetHistory, len(refsByTargetID))

	for targetID, candidateRefs := range refsByTargetID {
		selected, ok := a.selectTargetHistory(targets[targetID], candidateRefs, scanned.EntriesByRef, missingRefs)
		if !ok {
			continue
		}

		index[targetID] = selected
	}

	scan.index = index

	slog.DebugContext(ctx, "shared history index built",
		slog.String("branch", a.core.run.baseBranch),
		slog.Int("targets_total", len(targets)),
		slog.Int("targets_indexed", len(index)),
		slog.Int("refs_requested", len(refs)),
		slog.Int("refs_missing", len(scanned.MissingRefs)),
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
	entriesByRef map[string][]history.CommitEntry,
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

func sharedTargetHistory(scan *historyScan, target config.ResolvedTarget) (targetHistory, bool) {
	if scan.index == nil {
		return targetHistory{}, false
	}

	history, exists := scan.index[target.ID]

	return history, exists
}

func (a *releaseAnalyzer) commitsSince(
	ctx context.Context,
	scan *historyScan,
	ref string,
) ([]history.CommitEntry, error) {
	if cached, exists := scan.commits[ref]; exists {
		return cached, nil
	}

	history, err := a.history.GetCommitsSinceRefs(ctx, []string{ref}, scan.includePaths, scan.extraTags)
	if err != nil {
		return nil, fmt.Errorf("get commits from branch %q: %w", a.core.run.baseBranch, err)
	}

	if slices.Contains(history.MissingRefs, ref) {
		return nil, fmt.Errorf(
			"previous release ref %q is not reachable from release branch %q. "+
				"Verify the latest tag or release and branch ancestry: %w",
			ref,
			a.core.run.baseBranch,
			&forge.CommitBoundaryNotFoundError{Ref: ref, Branch: a.core.run.baseBranch},
		)
	}

	entries := history.EntriesByRef[ref]
	scan.commits[ref] = entries

	return entries, nil
}
