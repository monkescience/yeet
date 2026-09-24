package forge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Release struct {
	TagName   string
	CommitSHA string
	Name      string
	Body      string
	URL       string
}

type PullRequest struct {
	Number            int
	Reference         string
	Title             string
	Body              string
	URL               string
	Branch            string
	MergeCommitSHA    string
	NeedsPendingLabel bool
}

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
	BaseBranch    string
	ReleaseBranch string
	Method        MergeMethod
}

type FileUpdate struct {
	Content string
	Exists  bool
}

type TagRef struct {
	Name      string
	CommitSHA string
}

//nolint:interfacebloat // intentional aggregate. granular interfaces live consumer-side in package release.
type Provider interface {
	ListTagRefs(ctx context.Context) ([]TagRef, error)
	GetBranchHead(ctx context.Context, branch string) (string, error)
	IsAncestor(ctx context.Context, ancestorSHA, descendantSHA string) (bool, error)
	GetReleaseByTag(ctx context.Context, tag string) (*Release, error)
	CreateRelease(ctx context.Context, opts ReleaseOptions) (*Release, error)
	CreateReleasePR(ctx context.Context, opts ReleasePROptions) (*PullRequest, error)
	UpdateReleasePR(ctx context.Context, number int, opts ReleasePROptions) error
	FindOpenPendingReleasePRs(
		ctx context.Context,
		baseBranch, pendingLabel string,
		expectedBranches ...string,
	) ([]*PullRequest, error)
	FindOpenPendingReleasePRsForBase(
		ctx context.Context,
		baseBranch, pendingLabel string,
	) ([]*PullRequest, error)
	FindMergedReleasePR(
		ctx context.Context,
		baseBranch, pendingLabel string,
		expectedBranches ...string,
	) (*PullRequest, error)
	FindMergedReleasePRs(
		ctx context.Context,
		baseBranch, pendingLabel string,
		expectedBranches ...string,
	) ([]*PullRequest, error)
	EnsureAutoMerge(ctx context.Context, number int, opts MergeReleasePROptions) error
	MergeReleasePR(ctx context.Context, number int, opts MergeReleasePROptions) (string, error)
	SetReleasePRLabels(ctx context.Context, number int, labels ReleasePRLabels, phase ReleasePRPhase) error
	PreflightReleasePRTagging(ctx context.Context, taggedLabel string) error
	MaxPRBodyLength() int
	GetFile(ctx context.Context, branch, path string) (string, error)
	UpdateFiles(ctx context.Context, branch, base string, files map[string]FileUpdate, message string) error
	RepoURL() string
	PathPrefix() string
	CompareURL(fromRef, toRef string) string
}

var (
	ErrNoRelease               = errors.New("no release found")
	ErrReleaseTagMismatch      = errors.New("release tag commit mismatch")
	ErrReleasePRLabelMismatch  = errors.New("release PR lifecycle label mismatch")
	ErrReleasePRLabelMissing   = errors.New("release PR label does not exist")
	ErrReleasePRLabelsRejected = errors.New("release PR labels rejected")
	ErrCommitBoundaryNotFound  = errors.New("commit boundary not found")
	ErrNoPR                    = errors.New("no release PR found")
	ErrFileNotFound            = errors.New("file not found")
	ErrEmptyCommitSHA          = errors.New("empty commit SHA")
	ErrInvalidCommitSHA        = errors.New("ref is not a commit SHA")
	ErrEmptyTagName            = errors.New("empty tag name")
	ErrEmptyCommitID           = errors.New("empty commit ID")
	ErrRefNotFound             = errors.New("ref not found")
	ErrMergeBlocked            = errors.New("release PR merge blocked")
	ErrMergeNotFinalized       = errors.New("release PR merge did not finalize")
	ErrUntrustedReleasePR      = errors.New("untrusted release PR")
	ErrMergeMethodUnsupported  = errors.New("merge method unsupported")
	ErrAutoMergeUnsupported    = errors.New("provider-managed auto-merge unsupported")
	ErrReviewerNotFound        = errors.New("reviewer not found")
	ErrReviewerAmbiguous       = errors.New("reviewer is ambiguous")
	ErrReviewerNotApplied      = errors.New("reviewer not applied")
	ErrPaginationLimitExceeded = errors.New("pagination safety limit exceeded")
)

type CommitBoundaryNotFoundError struct {
	Ref    string
	Branch string
	Target string
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

type MergeBlockedReason string

const (
	MergeBlockedReasonConflicts MergeBlockedReason = "conflicts"
	MergeBlockedReasonDraft     MergeBlockedReason = "draft"
	MergeBlockedReasonClosed    MergeBlockedReason = "closed"
	MergeBlockedReasonPolicy    MergeBlockedReason = "policy"
	MergeBlockedReasonMethod    MergeBlockedReason = "method"
	MergeBlockedReasonFailure   MergeBlockedReason = "failure"
	MergeBlockedReasonUnknown   MergeBlockedReason = "unknown"
)

type MergeBlockedError struct {
	MergeStatus     string
	Reference       string
	Reason          MergeBlockedReason
	Detail          string
	ProviderMessage string
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

type UntrustedReleasePRError struct {
	Reference string
}

func (e *UntrustedReleasePRError) Error() string {
	reference := strings.TrimSpace(e.Reference)
	if reference == "" {
		return ErrUntrustedReleasePR.Error()
	}

	return fmt.Sprintf("%s: %s", ErrUntrustedReleasePR, reference)
}

func (e *UntrustedReleasePRError) Unwrap() error {
	return ErrUntrustedReleasePR
}

type MergeMethodUnsupportedError struct {
	Method MergeMethod
	Branch string
}

func (e *MergeMethodUnsupportedError) Error() string {
	method := strings.TrimSpace(string(e.Method))
	branch := strings.TrimSpace(e.Branch)

	switch {
	case method == "":
		return ErrMergeMethodUnsupported.Error()
	case branch != "":
		return fmt.Sprintf(
			"%s: merge queue for branch %q has unknown merge method %q",
			ErrMergeMethodUnsupported,
			branch,
			method,
		)
	default:
		return fmt.Sprintf("%s: unknown merge method %q", ErrMergeMethodUnsupported, method)
	}
}

func (e *MergeMethodUnsupportedError) Unwrap() error {
	return ErrMergeMethodUnsupported
}

type AutoMergeUnsupportedError struct {
	Provider        string
	Reference       string
	Version         string
	RequiredVersion string
	Problem         string
}

func (e *AutoMergeUnsupportedError) Error() string {
	problem := strings.TrimSpace(e.Problem)
	if problem == "" {
		return ErrAutoMergeUnsupported.Error()
	}

	return ErrAutoMergeUnsupported.Error() + ": " + problem
}

func (e *AutoMergeUnsupportedError) Unwrap() error {
	return ErrAutoMergeUnsupported
}
