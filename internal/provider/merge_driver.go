package provider

import (
	"context"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/forge"
)

type mergeState struct {
	Reference        string
	RawReadiness     string
	MergeStatus      string
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
	reason  forge.MergeBlockedReason
	detail  string
	status  string
	message string
}

func (r *mergeRefusal) failure(reference string) error {
	return &forge.MergeBlockedError{
		Reference:       reference,
		Reason:          r.reason,
		Detail:          r.detail,
		MergeStatus:     r.status,
		ProviderMessage: r.message,
	}
}

type forgeMerge[M any] interface {
	state(ctx context.Context) (mergeState, error)
	resolveMethod(ctx context.Context, requested forge.MergeMethod) (M, error)
	execute(ctx context.Context, current mergeState, method M) (string, bool, error)
}

type mergeDriver[M any] struct {
	forge         forgeMerge[M]
	polling       mergePolling
	logger        *slog.Logger
	baseBranch    string
	releaseBranch string
}

func (d mergeDriver[M]) run(ctx context.Context, opts forge.MergeReleasePROptions) (string, error) {
	current, err := d.forge.state(ctx)
	if err != nil {
		return "", err
	}

	if !d.isTrusted(current) {
		return "", &forge.UntrustedReleasePRError{Reference: current.Reference}
	}

	if current.IsMerged {
		if current.MergeCommitSHA != "" {
			return current.MergeCommitSHA, nil
		}

		return d.awaitMergeCommit(ctx, current.Reference)
	}

	err = checkMergeReadiness(current)
	if err != nil {
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

func (d mergeDriver[M]) isTrusted(current mergeState) bool {
	return isTrustedMergeState(current, d.baseBranch, d.releaseBranch)
}

func (d mergeDriver[M]) awaitMergeCommit(ctx context.Context, reference string) (string, error) {
	return d.polling.awaitMergedCommit(ctx, d.logger, reference, func(pollCtx context.Context) (string, error) {
		current, err := d.forge.state(pollCtx)
		if err != nil {
			return "", err
		}

		if !d.isTrusted(current) {
			return "", &forge.UntrustedReleasePRError{Reference: current.Reference}
		}

		switch {
		case current.Refusal != nil:
			return "", current.Refusal.failure(reference)
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

func checkMergeReadiness(current mergeState) error {
	err := checkAutoMergeCandidate(current)
	if err != nil {
		return err
	}

	switch {
	case current.ReadinessBlocked:
		return &forge.MergeBlockedError{
			Reference:   current.Reference,
			Reason:      forge.MergeBlockedReasonPolicy,
			Detail:      current.RawReadiness,
			MergeStatus: current.MergeStatus,
		}
	default:
		return nil
	}
}

func blockedMerge(reference string, reason forge.MergeBlockedReason, detail string) error {
	return &forge.MergeBlockedError{Reference: reference, Reason: reason, Detail: detail}
}

func blockedMergeMessage(reference string, reason forge.MergeBlockedReason, detail, message string) error {
	return &forge.MergeBlockedError{
		Reference:       reference,
		Reason:          reason,
		Detail:          detail,
		ProviderMessage: strings.TrimSpace(message),
	}
}
