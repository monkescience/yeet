// Package history reads release commits and base-branch files from a complete
// local checkout whose HEAD matches the remote release branch. It never
// mutates the checkout, and the provider remains authoritative for remote data.
package history

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/monkescience/yeet/internal/forge"
)

// ErrCheckoutUnusable marks a checkout that cannot reliably serve release history.
var ErrCheckoutUnusable = errors.New("local checkout cannot serve release history")

var errRemoteTagMetadata = errors.New("remote tag metadata invalid")

// CommitEntry is one commit in a release range.
type CommitEntry struct {
	Hash    string
	Message string
	Paths   []string
}

// CommitHistory is the result of resolving one or more release ranges.
type CommitHistory struct {
	// EntriesByRef contains each reachable commit range in newest-first order.
	EntriesByRef map[string][]CommitEntry
	// MissingRefs contains refs that do not exist or are unreachable from the branch.
	MissingRefs []string
}

// Remote provides authoritative tag targets and the branch head used to
// validate the local checkout.
type Remote interface {
	ListTagRefs(ctx context.Context) ([]forge.TagRef, error)
	GetBranchHead(ctx context.Context, branch string) (string, error)
}

// Source resolves commit ranges and file content from a validated local checkout.
//
// Source is not safe for concurrent use. The release analyzer issues history
// calls sequentially.
type Source struct {
	remote Remote
	branch string
	dir    string

	local *localHistory

	remoteTags       []string
	remoteTagCommits map[string]string
}

// Open returns a history source backed by an eligible repository at dir or one
// of its parents.
func Open(ctx context.Context, remote Remote, branch, dir string) (*Source, error) {
	s := &Source{remote: remote, branch: branch, dir: dir}

	local, err := s.openEligibleLocal(ctx)
	if err != nil {
		return nil, err
	}

	_, err = local.branchGraph(ctx)
	if err != nil {
		return nil, fmt.Errorf("validate local commit graph: %w", err)
	}

	s.local = local

	return s, nil
}

// ListTags returns remote tag names and caches their commit targets for later
// boundary validation.
func (s *Source) ListTags(ctx context.Context) ([]string, error) {
	tags, _, err := s.loadRemoteTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("remote tags: %w", err)
	}

	return tags, nil
}

// GetFile reads a blob from the validated local HEAD commit. Working-tree
// changes are intentionally ignored so release inputs match the remote branch.
func (s *Source) GetFile(ctx context.Context, path string) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("read local file %q: %w", path, err)
	}

	commit, err := s.local.repo.CommitObject(s.local.head)
	if err != nil {
		return "", fmt.Errorf("read local head commit: %w", err)
	}

	file, err := commit.File(path)
	if errors.Is(err, object.ErrFileNotFound) {
		return "", forge.ErrFileNotFound
	}

	if err != nil {
		return "", fmt.Errorf("find local file %q: %w", path, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("read local file %q: %w", path, err)
	}

	return content, nil
}

// GetCommitsSinceRefs returns exact per-ref ranges from the local commit graph.
// knownTags carries boundaries the caller already knows, which is how a run
// scans from a tag it published itself: the forge tag listing is eventually
// consistent, so it cannot be asked about that tag yet.
func (s *Source) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	includePaths bool,
	knownTags []forge.TagRef,
) (CommitHistory, error) {
	err := ctx.Err()
	if err != nil {
		return CommitHistory{}, fmt.Errorf("read local history: %w", err)
	}

	boundaries, err := s.remoteBoundaries(ctx, refs, knownTags)
	if err != nil {
		return CommitHistory{}, err
	}

	history, err := s.local.commitsSinceRefs(ctx, refs, boundaries, includePaths)
	if err != nil {
		return CommitHistory{}, err
	}

	slog.DebugContext(ctx, "local git history served commit ranges",
		slog.String("branch", s.branch),
		slog.Int("refs", len(refs)),
	)

	return history, nil
}

func (s *Source) loadRemoteTags(ctx context.Context) ([]string, map[string]string, error) {
	if s.remoteTagCommits != nil {
		return slices.Clone(s.remoteTags), s.remoteTagCommits, nil
	}

	refs, err := s.remote.ListTagRefs(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list remote tag refs: %w", err)
	}

	tags := make([]string, 0, len(refs))
	commits := make(map[string]string, len(refs))

	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}

		commitHash := strings.TrimSpace(ref.CommitSHA)
		if commitHash == "" {
			return nil, nil, fmt.Errorf("%w: tag %q has no commit hash", errRemoteTagMetadata, name)
		}

		if existing, exists := commits[name]; exists && !strings.EqualFold(existing, commitHash) {
			return nil, nil, fmt.Errorf("%w: tag %q has conflicting commit hashes", errRemoteTagMetadata, name)
		}

		if _, exists := commits[name]; !exists {
			tags = append(tags, name)
		}

		commits[name] = commitHash
	}

	s.remoteTags = tags
	s.remoteTagCommits = commits

	return slices.Clone(tags), commits, nil
}

func (s *Source) remoteBoundaries(
	ctx context.Context,
	refs []string,
	knownTags []forge.TagRef,
) (map[string]plumbing.Hash, error) {
	_, remoteCommits, err := s.loadRemoteTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("load remote tag boundaries: %w", err)
	}

	boundaries := make(map[string]plumbing.Hash, len(refs))

	for _, rawRef := range refs {
		ref := strings.TrimSpace(rawRef)
		if ref == "" {
			continue
		}

		if _, exists := boundaries[ref]; exists {
			continue
		}

		remoteCommit, exists := remoteCommits[ref]
		if !exists {
			remoteCommit, exists = knownTagCommit(knownTags, ref)
		}

		if !exists {
			return nil, fmt.Errorf("%w: tag %q is not present in the remote tag list", ErrCheckoutUnusable, ref)
		}

		boundary, valid := plumbing.FromHex(remoteCommit)
		if !valid {
			return nil, fmt.Errorf("%w: tag %q has invalid commit hash", errRemoteTagMetadata, ref)
		}

		boundaries[ref] = boundary
	}

	return boundaries, nil
}

func knownTagCommit(knownTags []forge.TagRef, ref string) (string, bool) {
	for _, knownTag := range knownTags {
		if strings.TrimSpace(knownTag.Name) != ref {
			continue
		}

		commitHash := strings.TrimSpace(knownTag.CommitSHA)
		if commitHash == "" {
			return "", false
		}

		return commitHash, true
	}

	return "", false
}

func (s *Source) openEligibleLocal(ctx context.Context) (*localHistory, error) {
	repo, err := git.PlainOpenWithOptions(s.dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"%w: no git repository found. Run yeet inside a full checkout of branch %q (%v)",
			ErrCheckoutUnusable, s.branch, err,
		)
	}

	shallows, err := repo.Storer.Shallow()
	if err != nil {
		return nil, fmt.Errorf("%w: shallow state unreadable (%v)", ErrCheckoutUnusable, err)
	}

	if len(shallows) > 0 {
		return nil, fmt.Errorf(
			"%w: checkout is shallow. Fetch the full history "+
				"(fetch-depth: 0 on GitHub Actions, GIT_DEPTH \"0\" on GitLab CI, "+
				"fetchDepth: 0 on Azure Pipelines)",
			ErrCheckoutUnusable,
		)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve HEAD (%v)", ErrCheckoutUnusable, err)
	}

	// A checkout of another branch must never be analyzed, even when its tip
	// happens to be known to the remote. Detached heads (the common CI shape)
	// are validated purely by the hash comparison below.
	if head.Name().IsBranch() && head.Name().Short() != s.branch {
		return nil, fmt.Errorf(
			"%w: checkout is on branch %q. Check out release branch %q",
			ErrCheckoutUnusable, head.Name().Short(), s.branch,
		)
	}

	remoteHead, err := s.remote.GetBranchHead(ctx, s.branch)
	if err != nil {
		return nil, fmt.Errorf("validate local head against remote branch %q: %w", s.branch, err)
	}

	if !strings.EqualFold(head.Hash().String(), strings.TrimSpace(remoteHead)) {
		return nil, fmt.Errorf(
			"%w: local HEAD %s does not match the remote head %s of branch %q. "+
				"Pull the latest commits before releasing",
			ErrCheckoutUnusable, head.Hash(), strings.TrimSpace(remoteHead), s.branch,
		)
	}

	return newLocalHistory(repo, head.Hash()), nil
}
