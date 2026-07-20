// Package history serves release commit history from the local git checkout.
// The checkout must be complete (non-shallow) and its HEAD must exactly match
// the remote head of the configured release branch. An unusable checkout
// fails the run with an actionable error instead of silently analyzing the
// wrong commits. The provider stays authoritative for tags, releases, and
// everything mutable. The local repository is only read, never fetched,
// checked out, or mutated.
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
	"github.com/monkescience/yeet/internal/provider"
)

// ErrCheckoutUnusable marks a local checkout that must not serve release
// history: missing, shallow, on the wrong branch, or out of sync with the
// remote release branch.
var ErrCheckoutUnusable = errors.New("local checkout cannot serve release history")

var errRemoteTagMetadata = errors.New("remote tag metadata invalid")

// Remote is the provider-backed slice of history capabilities the local
// source needs: authoritative version refs with commit targets and the branch
// head used to validate the checkout.
type Remote interface {
	GetLatestVersionRef(ctx context.Context) (string, error)
	ListTagRefs(ctx context.Context) ([]provider.TagRef, error)
	GetBranchHead(ctx context.Context, branch string) (string, error)
}

// Source resolves commit ranges from the local checkout. Eligibility is
// validated once per Source: the repository must be complete (non-shallow)
// and HEAD must be exactly the remote head of the configured branch.
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

// New returns a history source for the configured release branch that looks
// for a git repository at dir (searching parent directories like git does).
// Construction never fails: the checkout is validated on first use.
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

// GetLatestVersionRef always asks the remote provider. Remote tags and
// releases are the source of truth.
func (s *Source) GetLatestVersionRef(ctx context.Context) (string, error) {
	ref, err := s.remote.GetLatestVersionRef(ctx)
	if err != nil {
		return "", fmt.Errorf("remote latest version ref: %w", err)
	}

	return ref, nil
}

// ListTags loads names and commit targets from the remote provider. The same
// snapshot validates local release boundaries later in the run.
func (s *Source) ListTags(ctx context.Context) ([]string, error) {
	tags, _, err := s.loadRemoteTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("remote tags: %w", err)
	}

	return tags, nil
}

// GetCommitsSinceRefs serves exact per-ref commit ranges from the local
// commit graph. It fails with ErrCheckoutUnusable when the checkout cannot
// represent the release branch faithfully.
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

// eligibleLocal validates the checkout once per Source and memoizes success.
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

// openEligibleLocal runs the once-per-run checkout validation. Every failure
// names the problem and the fix, because there is no fallback path.
func (s *Source) openEligibleLocal(ctx context.Context) (*localHistory, error) {
	repo, err := git.PlainOpenWithOptions(s.dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"%w: no git repository found; run yeet inside a full checkout of branch %q (%v)",
			ErrCheckoutUnusable, s.branch, err,
		)
	}

	shallows, err := repo.Storer.Shallow()
	if err != nil {
		return nil, fmt.Errorf("%w: shallow state unreadable (%v)", ErrCheckoutUnusable, err)
	}

	if len(shallows) > 0 {
		return nil, fmt.Errorf(
			"%w: checkout is shallow; fetch the full history "+
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
			"%w: checkout is on branch %q; check out release branch %q",
			ErrCheckoutUnusable, head.Name().Short(), s.branch,
		)
	}

	remoteHead, err := s.remote.GetBranchHead(ctx, s.branch)
	if err != nil {
		return nil, fmt.Errorf("validate local head against remote branch %q: %w", s.branch, err)
	}

	if !strings.EqualFold(head.Hash().String(), strings.TrimSpace(remoteHead)) {
		return nil, fmt.Errorf(
			"%w: local HEAD %s does not match the remote head %s of branch %q; "+
				"pull the latest commits before releasing",
			ErrCheckoutUnusable, head.Hash(), strings.TrimSpace(remoteHead), s.branch,
		)
	}

	return newLocalHistory(repo, head.Hash()), nil
}
