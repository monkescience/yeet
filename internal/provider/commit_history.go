package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"
)

// DefaultMaxConcurrentRequests bounds both parallel boundary-ref resolution and
// per-commit changed-path fetches per provider so a single batch cannot trigger
// upstream rate limits. Override it with WithMaxConcurrentRequests.
const DefaultMaxConcurrentRequests = 8

// Option configures a provider at construction time.
type Option func(*concurrencyConfig)

type concurrencyConfig struct {
	maxConcurrentRequests int
}

func newConcurrencyConfig(opts []Option) concurrencyConfig {
	cfg := concurrencyConfig{maxConcurrentRequests: DefaultMaxConcurrentRequests}
	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// WithMaxConcurrentRequests overrides how many provider API requests a single
// batch issues in parallel. Non-positive limits are ignored.
func WithMaxConcurrentRequests(limit int) Option {
	return func(c *concurrencyConfig) {
		if limit > 0 {
			c.maxConcurrentRequests = limit
		}
	}
}

// latestVersionRefWithReleaseFallback returns the tag of the latest release,
// falling back to the most recent tag when the provider reports ErrNoRelease.
// It is shared by providers that expose a release concept (GitHub, GitLab);
// providers without one resolve the latest version ref from tags directly.
func latestVersionRefWithReleaseFallback(
	ctx context.Context,
	latestReleaseTag func(context.Context) (string, error),
	listTags func(context.Context) ([]string, error),
) (string, error) {
	tag, err := latestReleaseTag(ctx)
	if err == nil {
		return tag, nil
	}

	if !errors.Is(err, ErrNoRelease) {
		return "", err
	}

	tags, err := listTags(ctx)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoVersionRef
	}

	return tags[0], nil
}

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

// commitBoundaryScanner walks a provider's commit list newest-first, recording
// where each boundary ref's commit appears and accumulating the commits in
// order. It is the shared core of every provider's GetCommitsSinceRefs, so the
// three providers differ only in how they fetch and decode a commit.
type commitBoundaryScanner struct {
	boundaryRefsByID map[string][]string
	hasUnboundedRef  bool
	entries          []CommitEntry
	positions        map[string]int
	foundIDs         map[string]struct{}
}

func newCommitBoundaryScanner(boundaryRefsByID map[string][]string, hasUnboundedRef bool) *commitBoundaryScanner {
	return &commitBoundaryScanner{
		boundaryRefsByID: boundaryRefsByID,
		hasUnboundedRef:  hasUnboundedRef,
		entries:          make([]CommitEntry, 0),
		positions:        make(map[string]int, len(boundaryRefsByID)),
		foundIDs:         make(map[string]struct{}, len(boundaryRefsByID)),
	}
}

// observe records one commit by id and message, returning true when the walk
// can stop. It terminates before appending only when every boundary ref has
// been located and no unbounded ("") ref still needs older commits. Otherwise
// the boundary commit is appended too, so older refs see the full slice of
// commits since their own boundary.
func (s *commitBoundaryScanner) observe(id, message string) bool {
	if boundaryRefs, isBoundary := s.boundaryRefsByID[id]; isBoundary {
		for _, ref := range boundaryRefs {
			s.positions[ref] = len(s.entries)
		}

		s.foundIDs[id] = struct{}{}

		if len(s.foundIDs) == len(s.boundaryRefsByID) && !s.hasUnboundedRef {
			return true
		}
	}

	s.entries = append(s.entries, CommitEntry{Hash: id, Message: message})

	return false
}

// earlyTerminated reports whether the scan located every boundary ref without
// needing to walk the full history (used only for debug logging).
func (s *commitBoundaryScanner) earlyTerminated() bool {
	return !s.hasUnboundedRef && len(s.foundIDs) == len(s.boundaryRefsByID)
}

// pathSet accumulates changed-file paths in encounter order, trimming blanks
// and discarding duplicates. It is the shared accumulator for providers whose
// change payloads expose both a current and a previous path per file.
type pathSet struct {
	paths []string
	seen  map[string]struct{}
}

func newPathSet() *pathSet {
	return &pathSet{paths: make([]string, 0), seen: make(map[string]struct{})}
}

func (p *pathSet) add(candidate string) {
	normalized := strings.TrimSpace(candidate)
	if normalized == "" {
		return
	}

	if _, exists := p.seen[normalized]; exists {
		return
	}

	p.seen[normalized] = struct{}{}
	p.paths = append(p.paths, normalized)
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
	maxConcurrentRequests int,
) (map[string][]string, bool, error) {
	type resolvedRef struct {
		ref      string
		sha      string
		notFound bool
	}

	resolved := make([]resolvedRef, len(refs))
	hasUnboundedRef := false

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentRequests)

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
