// Package history serves release commit history from the local git checkout
// when it exactly matches the remote release branch, and transparently falls
// back to the remote provider otherwise. The provider stays authoritative for
// tags, releases, and everything mutable. The local repository is only read,
// never fetched, checked out, or mutated.
package history

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/monkescience/yeet/internal/provider"
)

// Remote is the provider-backed slice of history capabilities the local
// adapter needs: authoritative refs plus the fallback path for commit ranges.
type Remote interface {
	GetLatestVersionRef(ctx context.Context) (string, error)
	ListTags(ctx context.Context) ([]string, error)
	GetBranchHead(ctx context.Context, branch string) (string, error)
	GetCommitsSinceRefs(
		ctx context.Context,
		refs []string,
		branch string,
		includePaths bool,
	) (provider.CommitHistory, error)
}

// Source resolves commit ranges from the local checkout when it is eligible
// and delegates everything else to the remote provider. Eligibility is
// evaluated once per Source and requires a complete (non-shallow) repository
// whose HEAD is exactly the remote head of the configured branch.
//
// Source is not safe for concurrent use. The release analyzer issues history
// calls sequentially.
type Source struct {
	remote Remote
	branch string
	dir    string

	evaluated bool
	local     *localHistory
}

// New returns a history source for the configured release branch that looks
// for a git repository at dir (searching parent directories like git does).
// Construction never fails: an unusable checkout is detected lazily and every
// call is served by the remote provider instead.
func New(remote Remote, branch, dir string) *Source {
	return &Source{remote: remote, branch: branch, dir: dir}
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

// ListTags always asks the remote provider. Remote tags are the source of
// truth.
func (s *Source) ListTags(ctx context.Context) ([]string, error) {
	tags, err := s.remote.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("remote tags: %w", err)
	}

	return tags, nil
}

// GetCommitsSinceRefs serves exact per-ref commit ranges from the local
// commit graph when the checkout is eligible, and delegates the whole call to
// the remote provider otherwise. A local failure other than context
// cancellation disables the local path for the rest of the run and falls back.
func (s *Source) GetCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (provider.CommitHistory, error) {
	local, eligible := s.eligibleLocal(ctx, branch)
	if !eligible {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return provider.CommitHistory{}, fmt.Errorf("local history eligibility: %w", ctxErr)
		}

		return s.remoteCommitsSinceRefs(ctx, refs, branch, includePaths)
	}

	history, localErr := local.commitsSinceRefs(ctx, refs, includePaths)
	if localErr == nil {
		slog.DebugContext(ctx, "local git history served commit ranges",
			slog.String("branch", branch),
			slog.Int("refs", len(refs)),
		)

		return history, nil
	}

	if ctx.Err() != nil {
		return provider.CommitHistory{}, fmt.Errorf("local history: %w", localErr)
	}

	s.local = nil

	slog.DebugContext(ctx, "local git history unavailable; using provider",
		slog.String("reason", "local_read_failed"),
		slog.Any("error", localErr),
	)

	return s.remoteCommitsSinceRefs(ctx, refs, branch, includePaths)
}

func (s *Source) remoteCommitsSinceRefs(
	ctx context.Context,
	refs []string,
	branch string,
	includePaths bool,
) (provider.CommitHistory, error) {
	history, err := s.remote.GetCommitsSinceRefs(ctx, refs, branch, includePaths)
	if err != nil {
		return provider.CommitHistory{}, fmt.Errorf("remote commit history: %w", err)
	}

	return history, nil
}

// eligibleLocal returns the local history when the checkout may serve branch.
// The one-per-run evaluation is memoized, except when it was cut short by
// context cancellation, so a caller with a live context can still evaluate.
func (s *Source) eligibleLocal(ctx context.Context, branch string) (*localHistory, bool) {
	if branch != s.branch {
		return nil, false
	}

	if !s.evaluated {
		s.evaluated = true

		local, reason := s.openEligibleLocal(ctx)
		if local == nil {
			if ctx.Err() != nil {
				s.evaluated = false
			}

			slog.DebugContext(ctx, "local git history unavailable; using provider",
				slog.String("reason", reason),
				slog.String("branch", s.branch),
			)
		}

		s.local = local
	}

	return s.local, s.local != nil
}

// openEligibleLocal runs the once-per-run eligibility checks and returns a
// nil localHistory with a stable reason when the checkout must not be used.
func (s *Source) openEligibleLocal(ctx context.Context) (*localHistory, string) {
	repo, opened := openLocalRepository(s.dir)
	if !opened {
		return nil, "no_repository"
	}

	shallow, err := repositoryShallow(repo)
	if err != nil {
		return nil, "shallow_state_unreadable"
	}

	if shallow {
		return nil, "shallow_checkout"
	}

	head, resolved := repositoryHead(repo)
	if !resolved {
		return nil, "no_head"
	}

	// A checkout of another branch must never be analyzed, even when its tip
	// happens to be known to the remote. Detached heads (the common CI shape)
	// are validated purely by the hash comparison below.
	if head.Name().IsBranch() && head.Name().Short() != s.branch {
		return nil, "different_branch_checked_out"
	}

	remoteHead, validated := s.remoteBranchHead(ctx)
	if !validated {
		return nil, "remote_head_unavailable"
	}

	if !strings.EqualFold(head.Hash().String(), remoteHead) {
		return nil, "head_mismatch"
	}

	return newLocalHistory(repo, head.Hash()), ""
}

func (s *Source) remoteBranchHead(ctx context.Context) (string, bool) {
	head, err := s.remote.GetBranchHead(ctx, s.branch)
	if err != nil {
		slog.DebugContext(ctx, "resolve remote branch head failed",
			slog.String("branch", s.branch),
			slog.Any("error", err),
		)

		return "", false
	}

	return strings.TrimSpace(head), true
}

func openLocalRepository(dir string) (*git.Repository, bool) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, false
	}

	return repo, true
}

func repositoryShallow(repo *git.Repository) (bool, error) {
	shallows, err := repo.Storer.Shallow()
	if err != nil {
		return false, fmt.Errorf("read shallow state: %w", err)
	}

	return len(shallows) > 0, nil
}

func repositoryHead(repo *git.Repository) (*plumbing.Reference, bool) {
	head, err := repo.Head()
	if err != nil {
		return nil, false
	}

	return head, true
}
