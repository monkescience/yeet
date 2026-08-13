package provider

import (
	"context"
	"fmt"

	"github.com/monkescience/yeet/internal/forge"
)

// mergeState is a forge pull request reduced to the fields the merge policy
// needs, in one vocabulary. Normalising into it is where forge differences get
// named instead of buried in three adapters.
type mergeState struct {
	// Reference is the forge's own wording for the request, such as
	// "pull request #42" or "merge request !42". It reaches error text verbatim.
	Reference string
	// RawReadiness is the forge's own readiness expression, such as
	// "mergeable_state=blocked". It reaches error text verbatim and the driver
	// never interprets it.
	RawReadiness     string
	MergeCommitSHA   string
	HeadSHA          string
	SourceBranch     string
	BaseBranch       string
	IsOpen           bool
	IsMerged         bool
	IsClosedUnmerged bool
	IsDraft          bool
	HasConflicts     bool
	ReadinessBlocked bool
	SameRepository   bool
	Refusal          *mergeRefusal
}

type mergeRefusal struct {
	reason forge.MergeBlockedReason
	detail string
}

// forgeMerge is the per-forge half of the merge path: normalising the forge's
// pull request, resolving the merge method its capability model accepts, and
// issuing the merge itself.
//
// execute reports pending when the forge accepted the merge but has not applied
// it, which is what sends the driver into the polling loop. A forge whose API
// reveals a refusal immediately returns an error instead.
type forgeMerge interface {
	state(ctx context.Context) (mergeState, error)
	resolveMethod(ctx context.Context, requested forge.MergeMethod) (any, error)
	execute(ctx context.Context, current mergeState, method any) (string, bool, error)
}

type mergeDriver struct {
	forge         forgeMerge
	polling       mergePolling
	releaseBranch string
}

func (d mergeDriver) run(ctx context.Context, opts forge.MergeReleasePROptions) (string, error) {
	current, err := d.forge.state(ctx)
	if err != nil {
		return "", err
	}

	if current.IsMerged {
		if current.MergeCommitSHA != "" {
			return current.MergeCommitSHA, nil
		}

		return d.awaitMergeCommit(ctx, current.Reference)
	}

	if !current.SameRepository ||
		!isExpectedReleaseBranch(current.SourceBranch, current.BaseBranch, d.releaseBranch) {
		return "", fmt.Errorf("%w: %s", forge.ErrUntrustedReleasePR, current.Reference)
	}

	if err := checkMergeReadiness(current, opts.BypassMergeChecks); err != nil {
		return "", err
	}

	method, err := d.forge.resolveMethod(ctx, opts.Method)
	if err != nil {
		return "", err
	}

	mergeSHA, pending, err := d.forge.execute(ctx, current, method)
	if err != nil {
		return "", err
	}

	if !pending {
		return mergeSHA, nil
	}

	return d.awaitMergeCommit(ctx, current.Reference)
}

func (d mergeDriver) awaitMergeCommit(ctx context.Context, reference string) (string, error) {
	return d.polling.awaitMergedCommit(ctx, reference, func(pollCtx context.Context) (string, error) {
		current, err := d.forge.state(pollCtx)
		if err != nil {
			return "", err
		}

		// Readiness is deliberately absent here: a forge that accepted a merge
		// under --auto-merge-force still reports the policy it was told to skip.
		switch {
		case current.Refusal != nil:
			return "", blockedMerge(reference, current.Refusal.reason, current.Refusal.detail)
		case current.IsMerged:
			return current.MergeCommitSHA, nil
		case current.IsClosedUnmerged:
			return "", blockedMerge(reference, forge.MergeBlockedReasonClosed, "was closed")
		case current.HasConflicts:
			return "", blockedMerge(reference, forge.MergeBlockedReasonConflicts, "has conflicts")
		default:
			return "", nil
		}
	})
}

// checkMergeReadiness gates conflicts unconditionally and policy behind the
// bypass, because --auto-merge-force overrides a repository's own rules and
// never overrides a merge the forge cannot perform.
func checkMergeReadiness(current mergeState, bypassMergeChecks bool) error {
	switch {
	case !current.IsOpen:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonClosed, "is closed")
	case current.IsDraft:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonDraft, "is draft")
	case current.HasConflicts:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonConflicts, "has conflicts")
	case !bypassMergeChecks && current.ReadinessBlocked:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonPolicy, current.RawReadiness)
	default:
		return nil
	}
}

func blockedMerge(reference string, reason forge.MergeBlockedReason, detail string) error {
	return &forge.MergeBlockedError{Reference: reference, Reason: reason, Detail: detail}
}

func unsupportedResolvedMethod(method any) error {
	return fmt.Errorf("%w: %T", forge.ErrMergeMethodUnsupported, method)
}
