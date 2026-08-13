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
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
)

const fixtureBranch = "main"

type remoteStub struct {
	branchHead    string
	branchHeadErr error
	tagRefs       []forge.TagRef

	branchHeadCalls int
	tagRefCalls     int
}

func (r *remoteStub) ListTagRefs(_ context.Context) ([]forge.TagRef, error) {
	r.tagRefCalls++

	return slices.Clone(r.tagRefs), nil
}

func (r *remoteStub) GetBranchHead(_ context.Context, _ string) (string, error) {
	r.branchHeadCalls++

	return r.branchHead, r.branchHeadErr
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

func (f *repoFixture) source() (*history.Source, *remoteStub) {
	f.t.Helper()

	remote := &remoteStub{branchHead: f.head().String()}
	tags, err := f.repo.Tags()
	testastic.NoError(f.t, err)

	err = tags.ForEach(func(ref *plumbing.Reference) error {
		hash, resolveErr := f.repo.ResolveRevision(plumbing.Revision(ref.Name()))
		if resolveErr != nil {
			return resolveErr
		}

		remote.tagRefs = append(remote.tagRefs, forge.TagRef{
			Name:      ref.Name().Short(),
			CommitSHA: hash.String(),
		})

		return nil
	})
	testastic.NoError(f.t, err)

	return history.New(remote, fixtureBranch, f.dir), remote
}

func entryHashes(entries []history.CommitEntry) []string {
	hashes := make([]string, 0, len(entries))
	for _, entry := range entries {
		hashes = append(hashes, entry.Hash)
	}

	return hashes
}

func sortedPaths(entry history.CommitEntry) []string {
	paths := slices.Clone(entry.Paths)
	slices.Sort(paths)

	return paths
}

func entryByHash(t *testing.T, entries []history.CommitEntry, hash plumbing.Hash) history.CommitEntry {
	t.Helper()

	for _, entry := range entries {
		if entry.Hash == hash.String() {
			return entry
		}
	}

	t.Fatalf("commit %s not found in entries", hash)

	return history.CommitEntry{}
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

		source, _ := fx.source()

		// when: the range since the tag is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: exactly the commits after the tag are returned newest-first
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c3.String(), c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
		testastic.Equal(t, "feat: three", result.EntriesByRef["v1.0.0"][0].Message)
		testastic.Empty(t, result.MissingRefs)
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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, false, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.1"}, fixtureBranch, false, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v2.0.0", ""}, fixtureBranch, false, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", "v1.0.1"}, fixtureBranch, false, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", " v1.0.0 "}, fixtureBranch, false, nil)

		// then: one deduplicated range is returned
		testastic.NoError(t, err)
		testastic.Len(t, result.EntriesByRef, 1)
		testastic.SliceEqual(t, []string{c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
	})

	t.Run("detached head at branch tip is eligible", func(t *testing.T) {
		t.Parallel()

		// given: a CI-style detached checkout at the branch tip
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})
		fx.checkoutDetached(c2)

		remote := &remoteStub{
			branchHead: c2.String(),
			tagRefs:    []forge.TagRef{{Name: "v1.0.0", CommitSHA: c1.String()}},
		}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: local history serves it
		testastic.NoError(t, err)
		testastic.SliceEqual(t, []string{c2.String()}, entryHashes(result.EntriesByRef["v1.0.0"]))
		testastic.Equal(t, 1, remote.branchHeadCalls)
	})

	t.Run("eligibility is validated once per run", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		fx.commit("fix: two", map[string]string{"a.txt": "two"})

		source, remote := fx.source()

		// when: two history calls run in the same release run
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)
		testastic.NoError(t, err)
		_, err = source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, false, nil)

		// then: remote head and tag state are each loaded exactly once
		testastic.NoError(t, err)
		testastic.Equal(t, 1, remote.branchHeadCalls)
		testastic.Equal(t, 1, remote.tagRefCalls)
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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, true, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, fixtureBranch, true, nil)

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
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0", ""}, fixtureBranch, true, nil)
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

func TestSourceUnusableCheckout(t *testing.T) {
	t.Parallel()

	t.Run("missing repository fails with checkout error", func(t *testing.T) {
		t.Parallel()

		// given: a directory without a git repository
		remote := &remoteStub{}
		source := history.New(remote, fixtureBranch, t.TempDir())

		// when: a range is requested
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the run fails with the checkout sentinel
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, history.ErrCheckoutUnusable))
	})

	t.Run("shallow checkout fails naming the fetch-depth fix", func(t *testing.T) {
		t.Parallel()

		// given: a repository marked shallow
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})

		err := fx.repo.Storer.SetShallow([]plumbing.Hash{c1})
		testastic.NoError(t, err)

		source, remote := fx.source()

		// when: a range is requested
		_, err = source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the run fails before any remote validation
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, history.ErrCheckoutUnusable))
		testastic.Equal(
			t,
			"local checkout cannot serve release history: checkout is shallow. Fetch the full history "+
				"(fetch-depth: 0 on GitHub Actions, GIT_DEPTH \"0\" on GitLab CI, fetchDepth: 0 on Azure "+
				"Pipelines)",
			err.Error(),
		)
		testastic.Equal(t, 0, remote.branchHeadCalls)
	})

	t.Run("stale local head fails with pull hint", func(t *testing.T) {
		t.Parallel()

		// given: a remote branch head that differs from the local checkout
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		remote := &remoteStub{branchHead: "1111111111111111111111111111111111111111"}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the run fails asking for a current checkout
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, history.ErrCheckoutUnusable))
		testastic.Equal(
			t,
			"local checkout cannot serve release history: local HEAD "+
				"8ef653648bf61273f097a29668e6d5ed4134a2cc does not match the remote head "+
				"1111111111111111111111111111111111111111 of branch \"main\". Pull the latest commits "+
				"before releasing",
			err.Error(),
		)
	})

	t.Run("checkout of another branch fails even at the same commit", func(t *testing.T) {
		t.Parallel()

		// given: a feature branch checked out at the release branch tip
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.checkoutNewBranch("feature", c1)

		remote := &remoteStub{branchHead: c1.String()}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the run fails naming both branches
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, history.ErrCheckoutUnusable))
		testastic.Equal(
			t,
			"local checkout cannot serve release history: checkout is on branch \"feature\". Check out "+
				"release branch \"main\"",
			err.Error(),
		)
	})

	t.Run("remote head lookup failure propagates", func(t *testing.T) {
		t.Parallel()

		// given: a provider that cannot resolve the branch head
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		remote := &remoteStub{branchHeadErr: errors.New("boom")}
		source := history.New(remote, fixtureBranch, fx.dir)

		// when: a range is requested
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the remote error surfaces with validation context
		testastic.Error(t, err)
		testastic.Equal(t, "validate local head against remote branch \"main\": boom", err.Error())
	})

	t.Run("remote tag target works without a local tag", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout that lacks a tag present on the provider
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()
		remote.tagRefs = append(remote.tagRefs, forge.TagRef{
			Name:      "v9.9.9",
			CommitSHA: fx.head().String(),
		})

		// when: the provider tag is used as the release boundary
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v9.9.9"}, fixtureBranch, false, nil)

		// then: the provider target bounds history without a local tag ref
		testastic.NoError(t, err)
		testastic.Len(t, result.EntriesByRef["v9.9.9"], 0)
	})

	t.Run("remote tag target overrides a stale local tag", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout whose local tag differs from the provider target
		fx := newRepoFixture(t)
		c1 := fx.commit("chore: release", map[string]string{"a.txt": "one"})
		fx.tag("v1.0.0", c1)
		c2 := fx.commit("feat: two", map[string]string{"a.txt": "two"})

		source, remote := fx.source()
		remote.tagRefs = []forge.TagRef{{Name: "v1.0.0", CommitSHA: c2.String()}}

		// when: the tag is used as a release boundary
		result, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: the provider target is authoritative
		testastic.NoError(t, err)
		testastic.Len(t, result.EntriesByRef["v1.0.0"], 0)
	})

	t.Run("invalid remote tag target fails", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout and malformed provider tag metadata
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()
		remote.tagRefs = []forge.TagRef{{Name: "v1.0.0", CommitSHA: "not-a-commit"}}

		// when: the malformed target is used as a release boundary
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{"v1.0.0"}, fixtureBranch, false, nil)

		// then: invalid provider metadata is rejected
		testastic.Error(t, err)
		testastic.Equal(t, "remote tag metadata invalid: tag \"v1.0.0\" has invalid commit hash", err.Error())
	})

	t.Run("missing commit object fails", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout with a pruned parent commit object
		fx := newRepoFixture(t)
		c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
		fx.commit("fix: two", map[string]string{"a.txt": "two"})

		objectPath := filepath.Join(fx.dir, ".git", "objects", c1.String()[:2], c1.String()[2:])
		err := os.Remove(objectPath)
		testastic.NoError(t, err)

		source, _ := fx.source()

		// when: the checkout is validated before release execution
		err = source.Validate(t.Context())

		// then: preflight fails while building the complete graph
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"validate local commit graph: read local commit 8ef653648bf61273f097a29668e6d5ed4134a2cc: "+
				"object not found",
			err.Error(),
		)
	})

	t.Run("branch other than the configured one fails", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout configured for main
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, remote := fx.source()

		// when: history for a different branch is requested
		_, err := source.GetCommitsSinceRefs(t.Context(), []string{""}, "other", false, nil)

		// then: the run fails without touching the remote
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, history.ErrCheckoutUnusable))
		testastic.Equal(t, 0, remote.branchHeadCalls)
	})

	t.Run("context cancellation surfaces", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout and an already-cancelled context
		fx := newRepoFixture(t)
		fx.commit("feat: one", map[string]string{"a.txt": "one"})

		source, _ := fx.source()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		// when: a range is requested with the cancelled context
		_, err := source.GetCommitsSinceRefs(ctx, []string{""}, fixtureBranch, false, nil)

		// then: the cancellation surfaces
		testastic.Error(t, err)
		testastic.True(t, errors.Is(err, context.Canceled))
	})
}

func TestSourceDelegation(t *testing.T) {
	t.Parallel()

	// given: provider-side tag state
	fx := newRepoFixture(t)
	fx.commit("feat: one", map[string]string{"a.txt": "one"})

	source, remote := fx.source()
	remote.tagRefs = []forge.TagRef{
		{Name: "v3.0.0", CommitSHA: "3333333333333333333333333333333333333333"},
		{Name: "v2.0.0", CommitSHA: "2222222222222222222222222222222222222222"},
	}

	// when: tags are requested
	tags, err := source.ListTags(t.Context())

	// then: the provider answers once
	testastic.NoError(t, err)
	testastic.SliceEqual(t, []string{"v3.0.0", "v2.0.0"}, tags)
	testastic.Equal(t, 1, remote.tagRefCalls)
}

func TestSourceKnownTagBoundaries(t *testing.T) {
	t.Parallel()

	// given: a source whose tag snapshot predates a tag the caller already knows
	fx := newRepoFixture(t)
	c1 := fx.commit("feat: one", map[string]string{"a.txt": "one"})
	c2 := fx.commit("fix: two", map[string]string{"a.txt": "two"})

	source, remote := fx.source()
	remote.tagRefs = []forge.TagRef{}

	_, err := source.ListTags(t.Context())
	testastic.NoError(t, err)

	// when: the range since that tag is requested with its boundary supplied
	result, err := source.GetCommitsSinceRefs(
		t.Context(),
		[]string{"v2.0.0"},
		fixtureBranch,
		false,
		[]forge.TagRef{{Name: "v2.0.0", CommitSHA: c1.String()}},
	)

	// then: the boundary resolves without a second remote tag listing
	testastic.NoError(t, err)
	testastic.Empty(t, result.MissingRefs)
	testastic.SliceEqual(t, []string{c2.String()}, entryHashes(result.EntriesByRef["v2.0.0"]))
	testastic.Equal(t, 1, remote.tagRefCalls)
}

func TestSourceGetFile(t *testing.T) {
	t.Parallel()

	t.Run("reads the committed blob instead of the dirty worktree", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout whose working file differs from HEAD
		fx := newRepoFixture(t)
		fx.commit("feat: add config", map[string]string{"config.txt": "committed\n"})
		fx.writeFile("config.txt", "dirty\n")
		source, _ := fx.source()

		// when: the base branch file is read
		content, err := source.GetFile(t.Context(), fixtureBranch, "config.txt")

		// then: release input comes from the validated HEAD commit
		testastic.NoError(t, err)
		testastic.Equal(t, "committed\n", content)
	})

	t.Run("returns the provider file sentinel for a missing blob", func(t *testing.T) {
		t.Parallel()

		// given: an eligible checkout without the requested file
		fx := newRepoFixture(t)
		fx.commit("feat: add config", map[string]string{"config.txt": "committed\n"})
		source, _ := fx.source()

		// when: a missing base branch file is read
		_, err := source.GetFile(t.Context(), fixtureBranch, "missing.txt")

		// then: callers receive the shared missing-file sentinel
		testastic.ErrorIs(t, err, forge.ErrFileNotFound)
	})
}
