package history

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/monkescience/yeet/internal/provider"
)

// localHistory computes exact "reachable from head but not from boundary"
// ranges over the validated local checkout. The graph, ancestor sets, and
// per-commit changed paths are cached for the lifetime of one release run.
type localHistory struct {
	repo *git.Repository
	head plumbing.Hash

	graph       *branchGraph
	ancestors   map[plumbing.Hash]map[plumbing.Hash]struct{}
	pathsByHash map[plumbing.Hash][]string
}

type branchGraph struct {
	nodes map[plumbing.Hash]*graphNode
	// order holds every reachable commit newest-first: committer time
	// descending, traversal index ascending as the tie breaker.
	order []plumbing.Hash
}

type graphNode struct {
	parents []plumbing.Hash
	message string
	when    time.Time
	index   int
}

func newLocalHistory(repo *git.Repository, head plumbing.Hash) *localHistory {
	return &localHistory{
		repo:        repo,
		head:        head,
		ancestors:   make(map[plumbing.Hash]map[plumbing.Hash]struct{}),
		pathsByHash: make(map[plumbing.Hash][]string),
	}
}

// commitsSinceRefs mirrors the provider contract: one exact range per unique
// ref, newest-first, with refs that are absent from the branch graph reported
// in MissingRefs. Remote tag targets are validated before this method runs.
func (l *localHistory) commitsSinceRefs(
	ctx context.Context,
	refs []string,
	boundaries map[string]plumbing.Hash,
	includePaths bool,
) (provider.CommitHistory, error) {
	normalizedRefs := normalizeRefs(refs)

	graph, err := l.branchGraph(ctx)
	if err != nil {
		return provider.CommitHistory{}, err
	}

	history := provider.CommitHistory{
		EntriesByRef: make(map[string][]provider.CommitEntry, len(normalizedRefs)),
	}
	rangesByRef := make(map[string][]plumbing.Hash, len(normalizedRefs))

	for _, ref := range normalizedRefs {
		hashes, reachable := l.refRange(graph, ref, boundaries)
		if !reachable {
			history.MissingRefs = append(history.MissingRefs, ref)

			continue
		}

		rangesByRef[ref] = hashes
	}

	if includePaths {
		if err := l.hydratePaths(ctx, rangesByRef); err != nil {
			return provider.CommitHistory{}, err
		}
	}

	for ref, hashes := range rangesByRef {
		history.EntriesByRef[ref] = l.materializeEntries(graph, hashes, includePaths)
	}

	return history, nil
}

// refRange returns the hashes reachable from head but not from ref's
// boundary, newest-first. The empty ref means the complete history. A
// boundary outside the branch graph reports the ref as unreachable.
func (l *localHistory) refRange(
	graph *branchGraph,
	ref string,
	boundaries map[string]plumbing.Hash,
) ([]plumbing.Hash, bool) {
	if ref == "" {
		return graph.order, true
	}

	boundary := boundaries[ref]
	if _, reachable := graph.nodes[boundary]; !reachable {
		return nil, false
	}

	excluded := l.ancestorSet(boundary)
	hashes := make([]plumbing.Hash, 0)

	for _, hash := range graph.order {
		if _, isAncestor := excluded[hash]; !isAncestor {
			hashes = append(hashes, hash)
		}
	}

	return hashes, true
}

func (l *localHistory) materializeEntries(
	graph *branchGraph,
	hashes []plumbing.Hash,
	includePaths bool,
) []provider.CommitEntry {
	entries := make([]provider.CommitEntry, 0, len(hashes))

	for _, hash := range hashes {
		entry := provider.CommitEntry{
			Hash:    hash.String(),
			Message: graph.nodes[hash].message,
		}

		if includePaths {
			entry.Paths = slices.Clone(l.pathsByHash[hash])
		}

		entries = append(entries, entry)
	}

	return entries
}

func (l *localHistory) branchGraph(ctx context.Context) (*branchGraph, error) {
	if l.graph != nil {
		return l.graph, nil
	}

	nodes := make(map[plumbing.Hash]*graphNode)
	pending := []plumbing.Hash{l.head}
	index := 0

	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build local commit graph: %w", err)
		}

		hash := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if _, seen := nodes[hash]; seen {
			continue
		}

		commit, err := l.repo.CommitObject(hash)
		if err != nil {
			return nil, fmt.Errorf("read local commit %s: %w", hash, err)
		}

		nodes[hash] = &graphNode{
			parents: commit.ParentHashes,
			message: commit.Message,
			when:    commit.Committer.When,
			index:   index,
		}
		index++

		pending = append(pending, commit.ParentHashes...)
	}

	order := make([]plumbing.Hash, 0, len(nodes))
	for hash := range nodes {
		order = append(order, hash)
	}

	sort.Slice(order, func(i, j int) bool {
		left, right := nodes[order[i]], nodes[order[j]]
		if !left.when.Equal(right.when) {
			return left.when.After(right.when)
		}

		return left.index < right.index
	})

	l.graph = &branchGraph{nodes: nodes, order: order}

	return l.graph, nil
}

// ancestorSet returns every commit reachable from boundary, boundary
// included. Membership in a range is graph reachability, never a positional
// or timestamp slice, so skewed commit dates cannot over-include commits.
func (l *localHistory) ancestorSet(boundary plumbing.Hash) map[plumbing.Hash]struct{} {
	if cached, exists := l.ancestors[boundary]; exists {
		return cached
	}

	set := make(map[plumbing.Hash]struct{})
	pending := []plumbing.Hash{boundary}

	for len(pending) > 0 {
		hash := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if _, seen := set[hash]; seen {
			continue
		}

		set[hash] = struct{}{}

		if node, exists := l.graph.nodes[hash]; exists {
			pending = append(pending, node.parents...)
		}
	}

	l.ancestors[boundary] = set

	return set
}

// tagCommit resolves the exact refs/tags name and peels annotated tags to
// their commit. A tag the checkout lacks is fatal, so the error names the fix.
func (l *localHistory) tagCommit(ref string) (plumbing.Hash, error) {
	hash, err := l.repo.ResolveRevision(plumbing.Revision(plumbing.NewTagReferenceName(ref)))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf(
			"resolve local tag %q (fetch tags, e.g. git fetch --tags): %w", ref, err,
		)
	}

	return *hash, nil
}

// hydratePaths computes changed paths once per unique commit in the union of
// all requested ranges, reusing results cached from earlier calls.
func (l *localHistory) hydratePaths(ctx context.Context, rangesByRef map[string][]plumbing.Hash) error {
	for _, hashes := range rangesByRef {
		for _, hash := range hashes {
			if _, exists := l.pathsByHash[hash]; exists {
				continue
			}

			paths, err := l.commitPaths(ctx, hash)
			if err != nil {
				return err
			}

			l.pathsByHash[hash] = paths
		}
	}

	return nil
}

// commitPaths diffs the commit against its first parent (or the empty tree
// for a root commit) with rename detection, recording both the old and the
// new path of every file change, deduplicated in encounter order.
func (l *localHistory) commitPaths(ctx context.Context, hash plumbing.Hash) ([]string, error) {
	commit, err := l.repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("read local commit %s: %w", hash, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("read tree of local commit %s: %w", hash, err)
	}

	var parentTree *object.Tree

	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("read first parent of local commit %s: %w", hash, err)
		}

		parentTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("read parent tree of local commit %s: %w", hash, err)
		}
	}

	changes, err := object.DiffTreeWithOptions(ctx, parentTree, tree, object.DefaultDiffTreeOptions)
	if err != nil {
		return nil, fmt.Errorf("diff local commit %s: %w", hash, err)
	}

	paths := make([]string, 0, len(changes))
	seen := make(map[string]struct{}, len(changes))

	addPath := func(candidate string) {
		normalized := strings.TrimSpace(candidate)
		if normalized == "" {
			return
		}

		if _, exists := seen[normalized]; exists {
			return
		}

		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}

	for _, change := range changes {
		addPath(change.From.Name)
		addPath(change.To.Name)
	}

	return paths, nil
}

// normalizeRefs trims and deduplicates refs while preserving order, matching
// the provider-side batch normalization.
func normalizeRefs(refs []string) []string {
	normalized := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if _, exists := seen[ref]; exists {
			continue
		}

		seen[ref] = struct{}{}
		normalized = append(normalized, ref)
	}

	return normalized
}
