package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
// It is shared by providers that expose a release concept (GitHub, GitLab).
// Providers without one resolve the latest version ref from tags directly.
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

// commitsSinceFetch returns the commits reachable from the configured branch
// but not from ref, newest-first, with changed-file paths hydrated when the
// caller asked for them. ref == "" means the entire branch history (no
// boundary). It must return an error wrapping ErrRefNotFound when ref does not
// exist or is unreachable from the branch, so the batch records it as a missing
// ref instead of failing.
type commitsSinceFetch func(ctx context.Context, ref string) ([]CommitEntry, error)

// fetchCommitHistoryByRef resolves each ref's "commits since" range with one
// graph-aware fetch per ref, run in parallel and bounded by
// maxConcurrentRequests. Computing the range provider-side (rather than walking
// the whole branch once and slicing it) is what keeps non-linear histories
// correct: a flat, date-ordered commit list cannot reproduce "reachable from
// the branch but not from ref". Refs that fetch reports as ErrRefNotFound are
// recorded in MissingRefs rather than failing the batch. Duplicate refs are
// collapsed by normalizeCommitHistoryRefs.
func fetchCommitHistoryByRef(
	ctx context.Context,
	refs []string,
	maxConcurrentRequests int,
	fetch commitsSinceFetch,
) (CommitHistory, error) {
	normalizedRefs := normalizeCommitHistoryRefs(refs)
	history := CommitHistory{EntriesByRef: make(map[string][]CommitEntry, len(normalizedRefs))}

	if len(normalizedRefs) == 0 {
		return history, nil
	}

	type fetched struct {
		entries  []CommitEntry
		notFound bool
	}

	results := make([]fetched, len(normalizedRefs))

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentRequests)

	for idx, ref := range normalizedRefs {
		eg.Go(func() error {
			entries, err := fetch(egCtx, ref)
			if err != nil {
				if errors.Is(err, ErrRefNotFound) {
					results[idx] = fetched{notFound: true}

					return nil
				}

				return err
			}

			results[idx] = fetched{entries: entries}

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return CommitHistory{}, fmt.Errorf("fetch commits since refs: %w", err)
	}

	for idx, ref := range normalizedRefs {
		if results[idx].notFound {
			history.MissingRefs = append(history.MissingRefs, ref)

			continue
		}

		history.EntriesByRef[ref] = results[idx].entries
	}

	return history, nil
}

// commitPathsFetch resolves the changed-file paths of one commit.
type commitPathsFetch func(ctx context.Context, sha string) ([]string, error)

// hydrateCommitHistoryPaths fetches each commit's paths once, then assigns
// independent slices to entries shared across overlapping ref ranges.
func hydrateCommitHistoryPaths(
	ctx context.Context,
	history CommitHistory,
	maxConcurrentRequests int,
	commitPaths commitPathsFetch,
) error {
	pathIndexes := make(map[string]int)
	hashes := make([]string, 0)

	for _, entries := range history.EntriesByRef {
		for _, entry := range entries {
			if _, exists := pathIndexes[entry.Hash]; exists {
				continue
			}

			pathIndexes[entry.Hash] = len(hashes)
			hashes = append(hashes, entry.Hash)
		}
	}

	paths := make([][]string, len(hashes))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentRequests)

	for idx, hash := range hashes {
		eg.Go(func() error {
			fetchedPaths, err := commitPaths(egCtx, hash)
			if err != nil {
				return err
			}

			paths[idx] = fetchedPaths

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return fmt.Errorf("fetch commit paths: %w", err)
	}

	for ref, entries := range history.EntriesByRef {
		for idx := range entries {
			entries[idx].Paths = slices.Clone(paths[pathIndexes[entries[idx].Hash]])
		}

		history.EntriesByRef[ref] = entries
	}

	return nil
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
