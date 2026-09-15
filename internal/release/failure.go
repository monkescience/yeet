package release

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/version"
)

type FailureKind string

const (
	FailureUnexpected           FailureKind = "unexpected"
	FailureConfigMissing        FailureKind = "config_missing"
	FailureConfigInvalid        FailureKind = "config_invalid"
	FailureAuthentication       FailureKind = "authentication"
	FailureRepository           FailureKind = "repository"
	FailureHostTrust            FailureKind = "host_trust"
	FailureCheckout             FailureKind = "checkout"
	FailureReleaseBranch        FailureKind = "release_branch"
	FailureReleaseState         FailureKind = "release_state"
	FailureMergeBlocked         FailureKind = "merge_blocked"
	FailureMergeTimeout         FailureKind = "merge_timeout"
	FailureAutoMergeUnsupported FailureKind = "auto_merge_unsupported"
	FailureReviewer             FailureKind = "reviewer"
	FailureLabels               FailureKind = "labels"
	FailureFileConflict         FailureKind = "file_conflict"
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
	unit        string
	phase       string
	failures    []*Failure
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

func (f *Failure) Unit() string { return f.unit }

func (f *Failure) Phase() string { return f.phase }

func (f *Failure) Failures() []*Failure { return f.failures }

type unitError struct {
	unit  string
	phase string
	cause error
}

func (e *unitError) Error() string {
	return fmt.Sprintf("release unit %q %s: %s", e.unit, e.phase, e.cause)
}

func (e *unitError) Unwrap() error { return e.cause }

func classifyFailure(configPath string, err error) *Failure {
	failure := &Failure{
		kind:       classifyFailureKind(err),
		configPath: configPath,
		cause:      err,
	}

	if failure.kind == FailureMergeBlocked {
		failure.mergeReason = classifyMergeReason(err)
	}

	failure.failures = classifyIndependentFailures(configPath, err, "", "")
	if len(failure.failures) == 0 {
		failure.failures = []*Failure{failure}
	}

	return failure
}

func classifyIndependentFailures(configPath string, err error, unit, phase string) []*Failure {
	if nested, ok := err.(*unitError); ok { //nolint:errorlint // inspect each node before traversing all joined causes
		return classifyIndependentFailures(configPath, nested.cause, nested.unit, nested.phase)
	}

	if isSemanticFailure(err) {
		return classifyIndependentFailure(configPath, err, unit, phase)
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []*Failure
		for _, cause := range joined.Unwrap() {
			failures = append(failures, classifyIndependentFailures(configPath, cause, unit, phase)...)
		}

		return failures
	}

	cause := errors.Unwrap(err)
	if cause != nil && isIndependentAggregate(cause) {
		return classifyIndependentFailures(configPath, cause, unit, phase)
	}

	return classifyIndependentFailure(configPath, err, unit, phase)
}

func classifyIndependentFailure(configPath string, err error, unit, phase string) []*Failure {
	failure := &Failure{
		kind:       classifyFailureKind(err),
		configPath: configPath,
		cause:      err,
		unit:       unit,
		phase:      phase,
	}

	if failure.kind == FailureMergeBlocked {
		failure.mergeReason = classifyMergeReason(err)
	}

	return []*Failure{failure}
}

func isSemanticFailure(err error) bool {
	switch err.(type) { //nolint:errorlint // preserve this wrapper before traversing its causes
	case *config.ValidationError,
		*history.CheckoutError,
		*provider.SetupError,
		*provider.MergeNotFinalizedError,
		*SelectionError,
		*VersionFileError,
		*CommitOverrideError,
		*FileConflictError,
		*PendingReleaseError,
		*version.ReleaseAsError,
		*ManifestError:
		return true
	default:
		return false
	}
}

func isIndependentAggregate(err error) bool {
	if _, ok := err.(*unitError); ok { //nolint:errorlint // inspect this node before recursively checking causes
		return true
	}

	if _, ok := err.(interface{ Unwrap() []error }); ok {
		return true
	}

	cause := errors.Unwrap(err)

	return cause != nil && isIndependentAggregate(cause)
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
	case errors.Is(err, errConflictingFileUpdate):
		return FailureFileConflict
	case errors.Is(err, forge.ErrMergeNotFinalized):
		return FailureMergeTimeout
	case errors.Is(err, forge.ErrAutoMergeUnsupported):
		return FailureAutoMergeUnsupported
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
	return errors.Is(err, git.ErrRepositoryNotExists) ||
		errors.Is(err, provider.ErrUnsupportedProvider) ||
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
