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

var ErrCheckoutUnusable = errors.New("local checkout cannot serve release history")

var errRemoteTagMetadata = errors.New("remote tag metadata invalid")

const (
	CheckoutProblemShallow        = "checkout is shallow"
	CheckoutProblemShallowUnknown = "checkout shallow state is unavailable"
	CheckoutProblemNoRepository   = "git repository is unavailable"
	CheckoutProblemNoHead         = "checkout head is unavailable"
	CheckoutProblemOtherBranch    = "checkout is on another branch"
	CheckoutProblemBehindRemote   = "checkout does not match remote branch"
	CheckoutProblemTagUnavailable = "release tag is unavailable in checkout"
)

type CheckoutError struct {
	Problem       string
	Branch        string
	CurrentBranch string
	Ref           string
	LocalHead     string
	RemoteHead    string
	Err           error
}

func (e *CheckoutError) Error() string {
	return fmt.Sprintf("%s: %s", ErrCheckoutUnusable, e.Problem)
}

func (e *CheckoutError) Is(target error) bool {
	return target == ErrCheckoutUnusable
}

func (e *CheckoutError) Unwrap() error {
	return e.Err
}

func checkoutError(problem string, err error) error {
	return &CheckoutError{Problem: problem, Err: err}
}

type CommitEntry struct {
	Hash    string
	Message string
	Paths   []string
}

type CommitHistory struct {
	EntriesByRef map[string][]CommitEntry
	MissingRefs  []string
}

type Remote interface {
	ListTagRefs(ctx context.Context) ([]forge.TagRef, error)
	GetBranchHead(ctx context.Context, branch string) (string, error)
}

type Source struct {
	remote Remote
	branch string
	dir    string

	local *localHistory

	remoteTags       []string
	remoteTagCommits map[string]string
}

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

func (s *Source) ListTags(ctx context.Context) ([]string, error) {
	tags, _, err := s.loadRemoteTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("remote tags: %w", err)
	}

	return tags, nil
}

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
			return nil, &CheckoutError{
				Problem: CheckoutProblemTagUnavailable,
				Ref:     ref,
			}
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
		return nil, &CheckoutError{
			Problem: CheckoutProblemNoRepository,
			Branch:  s.branch,
			Err:     err,
		}
	}

	shallows, err := repo.Storer.Shallow()
	if err != nil {
		return nil, checkoutError(CheckoutProblemShallowUnknown, err)
	}

	if len(shallows) > 0 {
		return nil, checkoutError(CheckoutProblemShallow, nil)
	}

	return s.validateLocalHead(ctx, repo)
}

func (s *Source) validateLocalHead(ctx context.Context, repo *git.Repository) (*localHistory, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, checkoutError(CheckoutProblemNoHead, err)
	}

	if head.Name().IsBranch() && head.Name().Short() != s.branch {
		return nil, &CheckoutError{
			Problem:       CheckoutProblemOtherBranch,
			Branch:        s.branch,
			CurrentBranch: head.Name().Short(),
		}
	}

	remoteHead, err := s.remote.GetBranchHead(ctx, s.branch)
	if err != nil {
		return nil, fmt.Errorf("validate local head against remote branch %q: %w", s.branch, err)
	}

	if !strings.EqualFold(head.Hash().String(), strings.TrimSpace(remoteHead)) {
		return nil, &CheckoutError{
			Problem:    CheckoutProblemBehindRemote,
			Branch:     s.branch,
			LocalHead:  head.Hash().String(),
			RemoteHead: strings.TrimSpace(remoteHead),
		}
	}

	return newLocalHistory(repo, head.Hash()), nil
}
