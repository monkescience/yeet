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
	"github.com/monkescience/yeet/internal/provider"
)

// ErrCheckoutUnusable marks a checkout that cannot reliably serve release history.
var ErrCheckoutUnusable = errors.New("local checkout cannot serve release history")

var errRemoteTagMetadata = errors.New("remote tag metadata invalid")

// Remote provides authoritative tag targets and the branch head used to
// validate the local checkout.
type Remote interface {
	ListTagRefs(ctx context.Context) ([]provider.TagRef, error)
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

// New returns a history source that searches for a repository at dir or its
// parents. The checkout is validated on first use.
func New(remote Remote, branch, dir string) *Source {
	return &Source{remote: remote, branch: branch, dir: dir}
}

// Validate checks checkout eligibility and builds the complete reachable
// commit graph before the release workflow can mutate provider state.
func (s *Source) Validate(ctx context.Context) error {
	local, err := s.eligibleLocal(ctx, s.branch)
	if err != nil {
		return err
	}

	if _, err := local.branchGraph(ctx); err != nil {
		return fmt.Errorf("validate local commit graph: %w", err)
	}

	return nil
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

// InvalidateTags drops the cached remote tag snapshot so the next lookup
// observes tags published after the snapshot was taken.
func (s *Source) InvalidateTags() {
	s.remoteTags = nil
	s.remoteTagCommits = nil
}

// GetFile reads a blob from the validated local HEAD commit. Working-tree
// changes are intentionally ignored so release inputs match the remote branch.
func (s *Source) GetFile(ctx context.Context, branch, path string) (string, error) {
	local, err := s.eligibleLocal(ctx, branch)
	if err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("read local file %q: %w", path, err)
	}

	commit, err := local.repo.CommitObject(local.head)
	if err != nil {
		return "", fmt.Errorf("read local head commit: %w", err)
	}

	file, err := commit.File(path)
	if errors.Is(err, object.ErrFileNotFound) {
		return "", provider.ErrFileNotFound
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
func (s *Source) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (provider.CommitHistory, error) {
	local, err := s.eligibleLocal(ctx, branch)
	if err != nil {
		return provider.CommitHistory{}, err
	}

	boundaries, err := s.remoteBoundaries(ctx, refs)
	if err != nil {
		return provider.CommitHistory{}, err
	}

	history, err := local.commitsSinceRefs(ctx, refs, boundaries, includePaths)
	if err != nil {
		return provider.CommitHistory{}, err
	}

	slog.DebugContext(ctx, "local git history served commit ranges",
		slog.String("branch", branch),
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

func (s *Source) remoteBoundaries(ctx context.Context, refs []string) (map[string]plumbing.Hash, error) {
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

func (s *Source) eligibleLocal(ctx context.Context, branch string) (*localHistory, error) {
	if branch != s.branch {
		return nil, fmt.Errorf(
			"%w: history requested for branch %q but the source is configured for %q",
			ErrCheckoutUnusable, branch, s.branch,
		)
	}

	if s.local != nil {
		return s.local, nil
	}

	local, err := s.openEligibleLocal(ctx)
	if err != nil {
		return nil, err
	}

	s.local = local

	return local, nil
}

// openEligibleLocal returns actionable errors because no history fallback exists.
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
