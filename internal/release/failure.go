package release

import (
	"errors"
	"os"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
)

// FailureKind identifies the operational category of a release failure.
type FailureKind string

const (
	FailureUnexpected     FailureKind = "unexpected"
	FailureConfigMissing  FailureKind = "config_missing"
	FailureConfigInvalid  FailureKind = "config_invalid"
	FailureAuthentication FailureKind = "authentication"
	FailureRepository     FailureKind = "repository"
	FailureHostTrust      FailureKind = "host_trust"
	FailureCheckout       FailureKind = "checkout"
	FailureReleaseBranch  FailureKind = "release_branch"
	FailureReleaseState   FailureKind = "release_state"
	FailureMergeBlocked   FailureKind = "merge_blocked"
	FailureMergeTimeout   FailureKind = "merge_timeout"
	FailureReviewer       FailureKind = "reviewer"
	FailureLabels         FailureKind = "labels"
)

// MergeReason identifies why a forge refused to merge a release change.
type MergeReason string

const (
	MergeReasonConflicts MergeReason = "conflicts"
	MergeReasonDraft     MergeReason = "draft"
	MergeReasonClosed    MergeReason = "closed"
	MergeReasonPolicy    MergeReason = "policy"
	MergeReasonMethod    MergeReason = "method"
	MergeReasonProvider  MergeReason = "provider"
	MergeReasonUnknown   MergeReason = "unknown"
)

// Failure is the complete error interface returned by Run.
type Failure struct { //nolint:errname // the selected release interface is intentionally named Failure
	kind        FailureKind
	configPath  string
	mergeReason MergeReason
	cause       error
}

func (f *Failure) Error() string {
	return f.cause.Error()
}

func (f *Failure) Unwrap() error {
	return f.cause
}

func (f *Failure) Kind() FailureKind {
	return f.kind
}

func (f *Failure) ConfigPath() string {
	return f.configPath
}

func (f *Failure) MergeReason() MergeReason {
	return f.mergeReason
}

func classifyFailure(configPath string, err error) *Failure {
	failure := &Failure{
		kind:       classifyFailureKind(err),
		configPath: configPath,
		cause:      err,
	}

	if failure.kind == FailureMergeBlocked {
		failure.mergeReason = classifyMergeReason(err)
	}

	return failure
}

func classifyFailureKind(err error) FailureKind {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return FailureConfigMissing
	case errors.Is(err, config.ErrInvalidConfig):
		return FailureConfigInvalid
	case errors.Is(err, provider.ErrMissingToken):
		return FailureAuthentication
	case errors.Is(err, provider.ErrInvalidHost), errors.Is(err, provider.ErrUntrustedHost):
		return FailureHostTrust
	case errors.Is(err, history.ErrCheckoutUnusable):
		return FailureCheckout
	case errors.Is(err, errUnconfiguredReleaseBranch),
		errors.Is(err, errUnknownReleaseChannel),
		errors.Is(err, errCINonBranchRef):
		return FailureReleaseBranch
	case errors.Is(err, ErrMultiplePendingReleasePRs):
		return FailureReleaseState
	case errors.Is(err, forge.ErrMergeNotFinalized):
		return FailureMergeTimeout
	case errors.Is(err, forge.ErrMergeBlocked),
		errors.Is(err, forge.ErrMergeMethodUnsupported),
		errors.Is(err, forge.ErrUntrustedReleasePR):
		return FailureMergeBlocked
	case errors.Is(err, forge.ErrReviewerNotFound),
		errors.Is(err, forge.ErrReviewerAmbiguous),
		errors.Is(err, forge.ErrReviewerNotApplied):
		return FailureReviewer
	case errors.Is(err, forge.ErrReleasePRLabelMissing),
		errors.Is(err, forge.ErrReleasePRLabelMismatch),
		errors.Is(err, forge.ErrReleasePRLabelsRejected):
		return FailureLabels
	case isRepositoryFailure(err):
		return FailureRepository
	default:
		return FailureUnexpected
	}
}

func isRepositoryFailure(err error) bool {
	return errors.Is(err, provider.ErrUnsupportedProvider) ||
		errors.Is(err, provider.ErrUnknownRemote) ||
		errors.Is(err, provider.ErrUnsupportedHost) ||
		errors.Is(err, provider.ErrGitRemoteNotFound) ||
		errors.Is(err, provider.ErrGitRemoteHasNoURL) ||
		errors.Is(err, provider.ErrGitRemoteURLBlank) ||
		errors.Is(err, provider.ErrGitHubRepoRequired) ||
		errors.Is(err, provider.ErrGitHubOwnerInvalid) ||
		errors.Is(err, provider.ErrGitLabProjectNeeded) ||
		errors.Is(err, provider.ErrAzureDevOpsCoordsNeeded) ||
		errors.Is(err, provider.ErrRepositoryConflict)
}

func classifyMergeReason(err error) MergeReason {
	if errors.Is(err, forge.ErrMergeMethodUnsupported) {
		return MergeReasonMethod
	}

	if errors.Is(err, forge.ErrUntrustedReleasePR) {
		return MergeReasonProvider
	}

	var blocked *forge.MergeBlockedError
	if !errors.As(err, &blocked) {
		return MergeReasonUnknown
	}

	switch blocked.Reason {
	case forge.MergeBlockedReasonConflicts:
		return MergeReasonConflicts
	case forge.MergeBlockedReasonDraft:
		return MergeReasonDraft
	case forge.MergeBlockedReasonClosed:
		return MergeReasonClosed
	case forge.MergeBlockedReasonPolicy:
		return MergeReasonPolicy
	case forge.MergeBlockedReasonMethod:
		return MergeReasonMethod
	case forge.MergeBlockedReasonFailure:
		return MergeReasonProvider
	case forge.MergeBlockedReasonUnknown:
		return MergeReasonUnknown
	default:
		return MergeReasonUnknown
	}
}
