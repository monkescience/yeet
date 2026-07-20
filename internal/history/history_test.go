package history_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
)

const fixtureBranch = "main"

type remoteStub struct {
	branchHead    string
	branchHeadErr error
	history       provider.CommitHistory
	historyErr    error
	latestRef     string
	tags          []string

	branchHeadCalls int
	commitsCalls    [][]string
}

func (r *remoteStub) GetLatestVersionRef(_ context.Context) (string, error) {
	return r.latestRef, nil
}

func (r *remoteStub) ListTags(_ context.Context) ([]string, error) {
	return r.tags, nil
}

func (r *remoteStub) GetBranchHead(_ context.Context, _ string) (string, error) {
	r.branchHeadCalls++

	return r.branchHead, r.branchHeadErr
}

func (r *remoteStub) GetCommitsSinceRefs(
	_ context.Context,
	refs []string,
	_ string,
	_ bool,
) (provider.CommitHistory, error) {
	r.commitsCalls = append(r.commitsCalls, slices.Clone(refs))

	return r.history, r.historyErr
}

type repoFixture struct {
	t     *testing.T
	dir   string
	repo  *git.Repository
	wt    *git.Worktree
	clock time.Time
}

func newRepoFixture(t *testing.T) *repoFixture {
	t.Helper()

	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	testastic.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD,
		plumbing.NewBranchReferenceName(fixtureBranch),
	))
	testastic.NoError(t, err)

	wt, err := repo.Worktree()
	testastic.NoError(t, err)

	return &repoFixture{
		t:     t,
		dir:   dir,
		repo:  repo,
		wt:    wt,
		clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func (f *repoFixture) writeFile(path, content string) {
	f.t.Helper()

	fullPath := filepath.Join(f.dir, path)

	err := os.MkdirAll(filepath.Dir(fullPath), 0o750)
	testastic.NoError(f.t, err)

	err = os.WriteFile(fullPath, []byte(content), 0o600)
	testastic.NoError(f.t, err)
}

func (f *repoFixture) removeFile(path string) {
	f.t.Helper()

	err := os.Remove(filepath.Join(f.dir, path))
	testastic.NoError(f.t, err)
}

// commit stages every working-tree change and commits it one minute after
// the previous commit. Extra parent hashes turn the commit into a merge.
func (f *repoFixture) commit(message string, files map[string]string, extraParents ...plumbing.Hash) plumbing.Hash {
	f.t.Helper()

	return f.commitAt(message, files, f.clock.Add(time.Minute), extraParents...)
}

func (f *repoFixture) commitAt(
	message string,
	files map[string]string,
	when time.Time,
	extraParents ...plumbing.Hash,
) plumbing.Hash {
	f.t.Helper()

	for path, content := range files {
		f.writeFile(path, content)
	}

	err := f.wt.AddWithOptions(&git.AddOptions{All: true})
	testastic.NoError(f.t, err)

	f.clock = when
	signature := &object.Signature{Name: "yeet test", Email: "yeet@example.test", When: when}

	options := &git.CommitOptions{
		Author:            signature,
		Committer:         signature,
		AllowEmptyCommits: true,
	}

	if len(extraParents) > 0 {
		head, err := f.repo.Head()
		testastic.NoError(f.t, err)

		options.Parents = append([]plumbing.Hash{head.Hash()}, extraParents...)
	}

	hash, err := f.wt.Commit(message, options)
	testastic.NoError(f.t, err)

	return hash
}

func (f *repoFixture) tag(name string, hash plumbing.Hash) {
	f.t.Helper()

	_, err := f.repo.CreateTag(name, hash, nil)
	testastic.NoError(f.t, err)
}

func (f *repoFixture) annotatedTag(name string, hash plumbing.Hash) {
	f.t.Helper()

	_, err := f.repo.CreateTag(name, hash, &git.CreateTagOptions{
		Tagger:  &object.Signature{Name: "yeet test", Email: "yeet@example.test", When: f.clock},
		Message: "release " + name,
	})
	testastic.NoError(f.t, err)
}

func (f *repoFixture) checkoutNewBranch(name string, at plumbing.Hash) {
	f.t.Helper()

	err := f.wt.Checkout(&git.CheckoutOptions{
		Hash:   at,
		Branch: plumbing.NewBranchReferenceName(name),
		Create: true,
	})
	testastic.NoError(f.t, err)
}

func (f *repoFixture) checkoutBranch(name string) {
	f.t.Helper()

	err := f.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
	testastic.NoError(f.t, err)
}

func (f *repoFixture) checkoutDetached(at plumbing.Hash) {
	f.t.Helper()

	err := f.wt.Checkout(&git.CheckoutOptions{Hash: at})
	testastic.NoError(f.t, err)
}

func (f *repoFixture) head() plumbing.Hash {
	f.t.Helper()

	head, err := f.repo.Head()
	testastic.NoError(f.t, err)

	return head.Hash()
}

// source returns a Source whose remote stub reports the fixture head as the
// remote branch head, making the checkout eligible for local history.
func (f *repoFixture) source() (*history.Source, *remoteStub) {
	f.t.Helper()

	remote := &remoteStub{branchHead: f.head().String()}

	return history.New(remote, fixtureBranch, f.dir), remote
}

func entryHashes(entries []provider.CommitEntry) []string {
	hashes := make([]string, 0, len(entries))
	for _, entry := range entries {
		hashes = append(hashes, entry.Hash)
	}

	return hashes
}

func sortedPaths(entry provider.CommitEntry) []string {
	paths := slices.Clone(entry.Paths)
	slices.Sort(paths)

	return paths
}

func entryByHash(t *testing.T, entries []provider.CommitEntry, hash plumbing.Hash) provider.CommitEntry {
	t.Helper()

	for _, entry := range entries {
		if entry.Hash == hash.String() {
			return entry
		}
	}

	t.Fatalf("commit %s not found in entries", hash)

	return provider.CommitEntry{}
}

func TestSourceLocalRanges(t *testing.T) {
	t.Parallel()

	t.Run("linear range since latest tag", func(t *testing.T) {
		t.Parallel()

		// given: a linear history with a tag on the first commit
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})
		c3 := fx.commit("feat: three", map[string]string{"b.txt": "three"})

		source, remote := fx.source()

		// when: the range since the tag is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: exactly the commits after the tag are returned newest-first, locally
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c3.String(), c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
		testastic.Equal(t, "feat: three", result.EntriesByRef["v1.0.0"][0].Message)
		testastic.Empty(t, result.MissingRefs)
		testastic.Len(t, remote.commitsCalls, 0)
	})

	t.Run("empty ref returns complete history", func(t *testing.T) {
		t.Parallel()

		// given: a repository with three commits
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})
		c3 := fx.commit("feat: three", map[string]string{"b.txt": "three"})

		source, _ := fx.source()

		// when: the unbounded range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, false)

		// then: the whole branch history is returned newest-first
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c3.String(), c2.String(), c1.String()}, entryHashes(result.EntriesByRef[""]))
	})

	t.Run("merged hotfix tag excludes its ancestors only", func(t *testing.T) {
		t.Parallel()

		// given: a hotfix branch tagged v1.0.1 and merged back into main
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: base", map[string]string{"a.txt": "one"})
		fx.checkoutNewBranch("hotfix", c1)
		r1 := fx.commit("fix: hotfix", map[string]string{"hotfix.txt": "fix"})
		fx.tag("v1.0.1", r1)
		fx.checkoutBranch(fixtureBranch)
		c2 := fx.commit("feat: main work", map[string]string{"b.txt": "two"})
		m1 := fx.commit("chore: merge hotfix", map[string]string{"hotfix.txt": "fix"}, r1)
		c3 := fx.commit("feat: after merge", map[string]string{"c.txt": "three"})

		source, _ := fx.source()

		// when: the range since the hotfix tag is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.1"}, fixtureBranch, false)

		// then: only commits unreachable from the tag are included
		testastic.NoError(t, err)
		testastic.SliceEqual(
			t,
			[]string{c3.String(), m1.String(), c2.String()},
			entryHashes(result.EntriesByRef["v1.0.1"]),
		)
	})

	t.Run("tag on unmerged side branch is reported missing", func(t *testing.T) {
		t.Parallel()

		// given: a tag that only exists on an unmerged release branch
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: base", map[string]string{"a.txt": "one"})
		fx.checkoutNewBranch("release", c1)
		s1 := fx.commit("fix: release only", map[string]string{"release.txt": "fix"})
		fx.tag("v2.0.0", s1)
		fx.checkoutBranch(fixtureBranch)
		c2 := fx.commit("feat: main work", map[string]string{"b.txt": "two"})

		source, _ := fx.source()

		// when: the unreachable tag is requested next to a reachable base
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v2.0.0", ""}, fixtureBranch, false)

		// then: the tag is missing and the other range is still served
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{"v2.0.0"}, result.MissingRefs)
		testastic.MapNotHasKey(t, result.EntriesByRef, "v2.0.0")
		testastic.SliceEqual(t, []string{c2.String(), c1.String()}, entryHashes(result.EntriesByRef[""]))
	})

	t.Run("annotated and lightweight tags resolve to commits", func(t *testing.T) {
		t.Parallel()

		// given: an annotated tag and a lightweight tag on different commits
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.annotatedTag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})
		fx.tag("v1.0.1", c2)
		c3 := fx.commit("feat: three", map[string]string{"b.txt": "three"})

		source, _ := fx.source()

		// when: ranges for both tags are requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", "v1.0.1"}, fixtureBranch, false)

		// then: both tags peel to their commit and bound exact ranges
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c3.String(), c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
		testastic.SliceEqual(t, []string{c3.String()}, entryHashes(result.EntriesByRef["v1.0.1"]))
	})

	t.Run("skewed committer timestamps do not change membership", func(t *testing.T) {
		t.Parallel()

		// given: a boundary commit with a committer time far in the future
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})
		c2 := fx.commitAt("fix: skewed", map[string]string{"a.txt": "two"}, fx.clock.Add(2*time.Hour))
		fx.tag("v1.0.0", c2)
		c3 := fx.commitAt("feat: three", map[string]string{"b.txt": "three"}, fx.clock.Add(-90*time.Minute))

		source, _ := fx.source()

		// when: the range since the skewed tag is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: membership follows graph reachability, not timestamps
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c3.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
	})

	t.Run("duplicate and padded refs collapse to one range", func(t *testing.T) {
		t.Parallel()

		// given: a tagged history
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})

		source, _ := fx.source()

		// when: the same ref is requested twice with padding
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", " v1.0.0 "}, fixtureBranch, false)

		// then: one deduplicated range is returned
		testastic.NoError(t, err)
		testastic.Len(t, result.EntriesByRef, 1)
		testastic.SliceEqual(t, []string{c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
	})

	t.Run("detached head at branch tip uses local history", func(t *testing.T) {
		t.Parallel()

		// given: a CI-style detached checkout at the branch tip
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})
		fx.checkoutDetached(c2)

		remote := &remoteStub{branchHead: c2.String()}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: local history serves it without provider comparisons
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
		testastic.Len(t, remote.commitsCalls, 0)
		testastic.Equal(t, 1, remote.branchHeadCalls)
	})

	t.Run("eligibility is evaluated once per run", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		fx.commit("fix: two", map[string]string{"a.txt": "two"})

		source, remote := fx.source()

		// when: two history calls run in the same release run
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)
		testastic.NoError(t, err)
		_, err = source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, false)

		// then: the remote head is validated exactly once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, remote.branchHeadCalls)
	})
}

func TestSourcePaths(t *testing.T) {
	t.Parallel()

	t.Run("root add rename and delete record both path sides", func(t *testing.T) {
		t.Parallel()

		// given: a root commit, a rename, and a deletion
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: add files", map[string]string{
			"a.txt":     "stable content for rename detection",
			"dir/b.txt": "second file",
		})

		fx.removeFile("a.txt")
		c2 := fx.commit("refactor: rename a", map[string]string{
			"renamed.txt": "stable content for rename detection",
		})

		fx.removeFile("dir/b.txt")
		c3 := fx.commit("chore: delete b", nil)

		source, _ := fx.source()

		// when: the full history is requested with paths
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, true)

		// then: root paths, both rename sides, and the deleted path are present
		testastic.NoError(t, err)

		entries := result.EntriesByRef[""]
		testastic.SliceEqual(t, []string{"a.txt", "dir/b.txt"}, sortedPaths(entryByHash(t, entries, c1)))
		testastic.SliceEqual(t, []string{"a.txt", "renamed.txt"}, sortedPaths(entryByHash(t, entries, c2)))
		testastic.SliceEqual(t, []string{"dir/b.txt"}, sortedPaths(entryByHash(t, entries, c3)))
	})

	t.Run("merge commit paths follow the first parent diff", func(t *testing.T) {
		t.Parallel()

		// given: a merge commit whose second parent added a file
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: base", map[string]string{"a.txt": "one"})
		fx.checkoutNewBranch("feature", c1)
		f1 := fx.commit("feat: side file", map[string]string{"side.txt": "side"})
		fx.checkoutBranch(fixtureBranch)
		fx.commit("fix: main work", map[string]string{"a.txt": "two"})
		m1 := fx.commit("chore: merge feature", map[string]string{"side.txt": "side"}, f1)

		source, _ := fx.source()

		// when: the full history is requested with paths
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, true)

		// then: the merge reports the paths it introduced over its first parent
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{"side.txt"}, sortedPaths(entryByHash(t, result.EntriesByRef[""], m1)))
	})

	t.Run("overlapping ranges get independent path slices", func(t *testing.T) {
		t.Parallel()

		// given: two ranges sharing the newest commit
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"b.txt": "two"})

		source, _ := fx.source()

		// when: overlapping ranges are requested with paths
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", ""}, fixtureBranch, true)
		testastic.NoError(t, err)

		tagged := entryByHash(t, result.EntriesByRef["v1.0.0"], c2)
		full := entryByHash(t, result.EntriesByRef[""], c2)
		testastic.SliceEqual(t, []string{"b.txt"}, tagged.Paths)
		testastic.SliceEqual(t, []string{"b.txt"}, full.Paths)

		// then: mutating one range's paths does not leak into the other
		tagged.Paths[0] = "mutated"
		testastic.SliceEqual(t, []string{"b.txt"}, full.Paths)
	})
}

func TestSourceFallback(t *testing.T) {
	t.Parallel()

	fallbackHistory := provider.CommitHistory{
		EntriesByRef: map[string][]provider.CommitEntry{
			"v1.0.0": {{Hash: "remote", Message: "feat: remote"}},
		},
	}

	t.Run("no repository delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: a directory without a git repository
		remote := &remoteStub{history: fallbackHistory}
		source := history.New(remote, fixtureBranch, t.TempDir())

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: the provider serves the identical request
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Len(t, remote.commitsCalls, 1)
		testastic.SliceEqual(t, []string{"v1.0.0"}, remote.commitsCalls[0])
	})

	t.Run("shallow checkout delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: a repository marked shallow
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})

		err := fx.repo.Storer.SetShallow([]plumbing.Hash{c1})
		testastic.NoError(t, err)

		source, remote := fx.source()
		remote.history = fallbackHistory

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: the provider serves it without a head validation call
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Equal(t, 0, remote.branchHeadCalls)
		testastic.Len(t, remote.commitsCalls, 1)
	})

	t.Run("stale local head delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: a remote branch head ahead of the local checkout
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		remote := &remoteStub{
			branchHead: "1111111111111111111111111111111111111111",
			history:    fallbackHistory,
		}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: the provider serves it
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Len(t, remote.commitsCalls, 1)
	})

	t.Run("checkout of another branch delegates even at the same commit", func(t *testing.T) {
		t.Parallel()

		// given: a feature branch checked out at the release branch tip
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.checkoutNewBranch("feature", c1)

		remote := &remoteStub{branchHead: c1.String(), history: fallbackHistory}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: the provider serves it
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Len(t, remote.commitsCalls, 1)
	})

	t.Run("remote head lookup failure delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: a provider that cannot resolve the branch head
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		remote := &remoteStub{branchHeadErr: errors.New("boom"), history: fallbackHistory}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false)

		// then: the provider fallback still serves the run
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
	})

	t.Run("tag absent from the checkout delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout that lacks the requested tag
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()
		remote.history = fallbackHistory

		// when: an unknown tag is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v9.9.9"}, fixtureBranch, false)

		// then: the provider decides whether the tag exists
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Len(t, remote.commitsCalls, 1)
		testastic.SliceEqual(t, []string{"v9.9.9"}, remote.commitsCalls[0])
	})

	t.Run("missing commit object delegates to the provider", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout with a pruned parent commit object
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.commit("fix: two", map[string]string{"a.txt": "two"})

		objectPath := filepath.Join(fx.dir, ".git", "objects", c1.String()[:2], c1.String()[2:])
		err := os.Remove(objectPath)
		testastic.NoError(t, err)

		source, remote := fx.source()
		remote.history = fallbackHistory

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, false)

		// then: the provider serves it
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Len(t, remote.commitsCalls, 1)
	})

	t.Run("branch other than the configured one delegates", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout configured for main
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()
		remote.history = fallbackHistory

		// when: history for a different branch is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, "other", false)

		// then: the provider serves it without local validation
		testastic.NoError(t, err)
		testastic.DeepEqual(t, fallbackHistory, result)
		testastic.Equal(t, 0, remote.branchHeadCalls)
	})

	t.Run("context cancellation is returned, not hidden by fallback", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout and an already-cancelled context
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		// when: a range is requested with the cancelled context
		_, err := source.GetCommitsSinceRefs(ctx, []string{""}, fixtureBranch, false)

		// then: the cancellation surfaces and no provider fallback runs
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, context.Canceled))
		testastic.Len(t, remote.commitsCalls, 0)
	})
}

func TestSourceDelegation(t *testing.T) {
	t.Parallel()

	t.Run("tags and latest ref always come from the provider", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout and provider-side tag state
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()
		remote.latestRef = "v3.0.0"
		remote.tags = []string{"v3.0.0", "v2.0.0"}

		// when: tags and the latest ref are requested
		latest, err := source.GetLatestVersionRef(t.Context())
		testastic.NoError(t, err)

		tags, err := source.ListTags(t.Context())

		// then: the provider answers both
		testastic.NoError(t, err)
		testastic.Equal(t, "v3.0.0", latest)
		testastic.SliceEqual(t, []string{"v3.0.0", "v2.0.0"}, tags)
	})
}
