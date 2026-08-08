package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Release struct {
	TagName string
	Name    string
	Body    string
	URL     string
}

type PullRequest struct {
	Number            int
	Title             string
	Body              string
	URL               string
	Branch            string
	MergeCommitSHA    string
	NeedsPendingLabel bool
}

const ReleaseLabelYeet = "yeet"

// Label colors are stored without a leading "#" so callers can prepend it when
// the provider's API requires the prefix (GitLab) or omit it (GitHub).
const (
	releaseLabelPendingColor       = "FFD866"
	releaseLabelTaggedColor        = "A9DC76"
	releaseLabelYeetColor          = "FF6188"
	releaseLabelPendingDescription = "release PR is pending tagging"
	releaseLabelTaggedDescription  = "release PR already tagged"
	releaseLabelYeetDescription    = "release PR managed by yeet"
)

type ReleasePROptions struct {
	Title         string
	Body          string
	BaseBranch    string
	ReleaseBranch string
	Reviewers     []string
	Labels        ReleasePRLabels
}

type ReleasePRLabels struct {
	Pending string
	Tagged  string
	Yeet    bool
	Extra   []string
}

// ReleasePRPhase is which lifecycle label a release pull request should carry:
// pending while it is open, tagged once its releases are published.
type ReleasePRPhase int

const (
	ReleasePRPhasePending ReleasePRPhase = iota
	ReleasePRPhaseTagged
)

type ReleaseOptions struct {
	TagName    string
	Ref        string
	Name       string
	Body       string
	Prerelease bool
}

type MergeMethod string

const (
	MergeMethodAuto   MergeMethod = "auto"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
	MergeMethodMerge  MergeMethod = "merge"
)

type MergeReleasePROptions struct {
	BypassMergeChecks bool
	// Method is best effort, because the three forges expose unrelated
	// capability models. GitHub validates it against repository settings and
	// rejects a disabled method. GitLab has no per request strategy override, so
	// rebase and merge only assert that the project is already configured that
	// way. Azure DevOps exposes no capability API and validates nothing, which
	// leaves auto and squash indistinguishable there.
	Method MergeMethod
}

// FileUpdate holds new content and whether the file exists on the base branch.
type FileUpdate struct {
	Content string
	Exists  bool
}

// TagRef identifies a remote tag and the commit it resolves to. CommitSHA is
// the peeled commit hash for annotated tags.
type TagRef struct {
	Name      string
	CommitSHA string
}

//nolint:interfacebloat // intentional aggregate. granular interfaces live consumer-side in package release.
type Provider interface {
	ListTagRefs(ctx context.Context) ([]TagRef, error)
	// GetBranchHead returns the commit SHA the branch currently points at,
	// wrapping ErrRefNotFound when the branch does not exist. Release commit
	// ranges are computed from the local checkout (internal/history), which
	// uses this to validate that the checkout matches the remote branch.
	GetBranchHead(ctx context.Context, branch string) (string, error)

	GetReleaseByTag(ctx context.Context, tag string) (*Release, error)
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)

	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	FindOpenPendingReleasePRs(ctx context.Context, baseBranch, pendingLabel string) ([]*PullRequest, error)
	FindMergedReleasePR(ctx context.Context, baseBranch, pendingLabel string) (*PullRequest, error)
	// MergeReleasePR merges the release PR and returns the commit it produced on
	// the base branch. It blocks for up to two minutes while a forge finalizes a
	// merge it has already accepted, and is idempotent on a request that is
	// already merged, which it answers with the existing merge commit.
	//
	// It returns ErrUntrustedReleasePR for a request that is not on the release
	// branch of the configured repository, ErrMergeBlocked (as a
	// *MergeBlockedError) when the forge refuses the merge,
	// ErrMergeMethodUnsupported for a method no forge accepts, and
	// ErrMergeNotFinalized when an accepted merge does not land inside the wait.
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error)
	// SetReleasePRLabels puts a release pull request in a phase. It states the
	// phase rather than a desired end state: additions and removals are computed
	// within the managed set, which is Pending, Tagged, Yeet and Extra, and every
	// other label on the pull request is left where it is. Extra and the yeet
	// marker are applied at creation or adoption and never removed. See ADR 0006.
	//
	// The lifecycle label of the phase is attached before anything else and
	// fail-fast, so an interrupted run still leaves a pull request the next run
	// can find. See ADR 0007.
	//
	// Extra labels must already exist on forges that expose label definitions. An
	// unknown extra label fails the run before the pull request is created. Azure
	// DevOps creates labels on attach and has no definition API, so an unknown
	// extra label is created rather than rejected.
	SetReleasePRLabels(ctx context.Context, number int, labels ReleasePRLabels, phase ReleasePRPhase) error
	// PreflightReleasePRTagging checks known prerequisites without mutating pull
	// requests, releases, branches, or label definitions. GitHub and GitLab check
	// that taggedLabel exists. Azure DevOps cannot inspect label definitions and
	// returns nil. A later mutation can still fail or race this check.
	PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error
	// MaxPRBodyLength returns zero when the provider has no known limit.
	MaxPRBodyLength() int

	CreateBranch(ctx context.Context, name, base string) error
	GetFile(ctx context.Context, branch, path string) (string, error)
	// UpdateFiles force-updates a branch from base with one commit containing all file changes.
	UpdateFiles(ctx context.Context, branch, base string, files map[string]FileUpdate, message string) error

	RepoURL() string
	// PathPrefix returns the path prefix for commit URLs (empty for GitHub, "/-" for GitLab).
	PathPrefix() string
	CompareURL(fromRef, toRef string) string
}

type RepoInfo struct {
	Owner string
	Name  string
}

const DefaultGitHubHost = "github.com"

const DefaultGitLabHost = "gitlab.com"

const DefaultAzureDevOpsHost = "dev.azure.com"

const (
	providerNameAuto        = "auto"
	providerNameGitHub      = "github"
	providerNameGitLab      = "gitlab"
	providerNameAzureDevOps = "azuredevops"
)

var (
	ErrUnknownRemote           = errors.New("unable to parse remote URL")
	ErrUnsupportedHost         = errors.New("unsupported remote host")
	ErrNoRelease               = errors.New("no release found")
	ErrReleasePRLabelMismatch  = errors.New("release PR lifecycle label mismatch")
	ErrReleasePRLabelMissing   = errors.New("release PR label does not exist")
	ErrCommitBoundaryNotFound  = errors.New("commit boundary not found")
	ErrNoPR                    = errors.New("no release PR found")
	ErrFileNotFound            = errors.New("file not found")
	ErrEmptyCommitSHA          = errors.New("empty commit SHA")
	ErrEmptyCommitID           = errors.New("empty commit ID")
	ErrRefNotFound             = errors.New("ref not found")
	ErrMergeBlocked            = errors.New("release PR merge blocked")
	ErrMergeNotFinalized       = errors.New("release PR merge did not finalize")
	ErrUntrustedReleasePR      = errors.New("untrusted release PR")
	ErrMergeMethodUnsupported  = errors.New("merge method unsupported")
	ErrReviewerNotFound        = errors.New("reviewer not found")
	ErrReviewerAmbiguous       = errors.New("reviewer is ambiguous")
	ErrReviewerNotApplied      = errors.New("reviewer not applied")
	ErrPaginationLimitExceeded = errors.New("pagination safety limit exceeded")
)

const maxPaginationPages = 100

type CommitBoundaryNotFoundError struct {
	Ref    string
	Branch string
}

func (e *CommitBoundaryNotFoundError) Error() string {
	ref := strings.TrimSpace(e.Ref)
	branch := strings.TrimSpace(e.Branch)

	switch {
	case ref == "" && branch == "":
		return ErrCommitBoundaryNotFound.Error()
	case branch == "":
		return fmt.Sprintf("%s: ref %q", ErrCommitBoundaryNotFound, ref)
	case ref == "":
		return fmt.Sprintf("%s: branch %q", ErrCommitBoundaryNotFound, branch)
	default:
		return fmt.Sprintf("%s: ref %q is not reachable from branch %q", ErrCommitBoundaryNotFound, ref, branch)
	}
}

func (e *CommitBoundaryNotFoundError) Unwrap() error {
	return ErrCommitBoundaryNotFound
}

// MergeBlockedReason distinguishes the conditions ErrMergeBlocked covers, so a
// caller can tell a permanent refusal from one that may clear on its own.
type MergeBlockedReason string

const (
	MergeBlockedReasonConflicts MergeBlockedReason = "conflicts"
	MergeBlockedReasonDraft     MergeBlockedReason = "draft"
	MergeBlockedReasonClosed    MergeBlockedReason = "closed"
	MergeBlockedReasonPolicy    MergeBlockedReason = "policy"
	MergeBlockedReasonMethod    MergeBlockedReason = "method"
	MergeBlockedReasonUnknown   MergeBlockedReason = "unknown"
)

// MergeBlockedError reports why a forge refused to merge a release PR. Detail
// carries the forge's own explanation and is never parsed.
type MergeBlockedError struct {
	Reference string
	Reason    MergeBlockedReason
	Detail    string
}

func (e *MergeBlockedError) Error() string {
	reference := strings.TrimSpace(e.Reference)
	detail := strings.TrimSpace(e.Detail)

	switch {
	case reference == "" && detail == "":
		return ErrMergeBlocked.Error()
	case detail == "":
		return fmt.Sprintf("%s: %s", ErrMergeBlocked, reference)
	case reference == "":
		return fmt.Sprintf("%s: %s", ErrMergeBlocked, detail)
	default:
		return fmt.Sprintf("%s: %s %s", ErrMergeBlocked, reference, detail)
	}
}

func (e *MergeBlockedError) Unwrap() error {
	return ErrMergeBlocked
}
