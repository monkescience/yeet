package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Bounds both parallel boundary-ref resolution and per-commit changed-path
// fetches so a single batch cannot trigger upstream rate limits.
const maxConcurrentProviderRequests = 5

func normalizeCommitHistoryRefs(refs []string) []string {
	normalizedRefs := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if _, exists := seen[ref]; exists {
			continue
		}

		seen[ref] = struct{}{}
		normalizedRefs = append(normalizedRefs, ref)
	}

	return normalizedRefs
}

func cloneCommitEntries(entries []CommitEntry) []CommitEntry {
	cloned := make([]CommitEntry, len(entries))
	copy(cloned, entries)

	for idx := range cloned {
		if len(cloned[idx].Paths) == 0 {
			continue
		}

		cloned[idx].Paths = append([]string(nil), cloned[idx].Paths...)
	}

	return cloned
}

// Refs are resolved in parallel and grouped by their resolved SHA so duplicate
// refs (different names pointing at the same commit) share one boundary entry.
// Refs that the resolver reports as ErrRefNotFound (e.g. a tag deleted between
// ListTags and this call) are dropped from the boundary map. They will surface
// as MissingRefs once commitHistoryFromBoundaryPositions sees no position for
// them, so a single unknown ref does not fail the whole batch. The second
// return value reports whether the caller asked for an unbounded ("") scan,
// which disables the early-termination heuristic in the commit scan loop.
func resolveBoundaryRefs(
	ctx context.Context,
	refs []string,
	resolve func(ctx context.Context, ref string) (string, error),
) (map[string][]string, bool, error) {
	type resolvedRef struct {
		ref      string
		sha      string
		notFound bool
	}

	resolved := make([]resolvedRef, len(refs))
	hasUnboundedRef := false

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentProviderRequests)

	for idx, ref := range refs {
		if ref == "" {
			hasUnboundedRef = true

			continue
		}

		eg.Go(func() error {
			sha, err := resolve(egCtx, ref)
			if err != nil {
				if errors.Is(err, ErrRefNotFound) {
					resolved[idx] = resolvedRef{ref: ref, notFound: true}

					return nil
				}

				return fmt.Errorf("resolve ref %q: %w", ref, err)
			}

			resolved[idx] = resolvedRef{ref: ref, sha: sha}

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return nil, false, fmt.Errorf("resolve refs: %w", err)
	}

	boundaryRefsBySHA := make(map[string][]string)

	for _, r := range resolved {
		if r.ref == "" || r.notFound {
			continue
		}

		boundaryRefsBySHA[r.sha] = append(boundaryRefsBySHA[r.sha], r.ref)
	}

	return boundaryRefsBySHA, hasUnboundedRef, nil
}

// Drops boundary commits from the tail of entries that no ref's slice
// references. The scan loop appends boundary commits so older refs see them,
// but when no unbounded ref needs the tail and one or more requested refs
// never appeared in the walk, the trailing boundaries are dead weight:
// keeping them around would also force pointless per-commit detail fetches
// during path hydration.
func trimEntriesToReferencedRange(
	entries []CommitEntry,
	positions map[string]int,
	hasUnboundedRef bool,
) []CommitEntry {
	if hasUnboundedRef {
		return entries
	}

	maxPos := 0

	for _, pos := range positions {
		if pos > maxPos {
			maxPos = pos
		}
	}

	if maxPos >= len(entries) {
		return entries
	}

	return entries[:maxPos]
}

func commitHistoryFromBoundaryPositions(
	refs []string,
	entries []CommitEntry,
	positions map[string]int,
) CommitHistory {
	history := CommitHistory{
		EntriesByRef: make(map[string][]CommitEntry, len(refs)),
	}

	for _, ref := range refs {
		if ref == "" {
			history.EntriesByRef[ref] = cloneCommitEntries(entries)

			continue
		}

		position, exists := positions[ref]
		if !exists {
			history.MissingRefs = append(history.MissingRefs, ref)

			continue
		}

		history.EntriesByRef[ref] = cloneCommitEntries(entries[:position])
	}

	return history
}
