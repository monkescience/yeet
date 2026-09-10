package provider

import (
	"strings"

	"github.com/monkescience/yeet/internal/forge"
)

func isTrustedMergeState(current mergeState, baseBranch, releaseBranch string) bool {
	expectedBase := strings.TrimSpace(baseBranch)

	return current.SameRepository &&
		strings.TrimSpace(current.BaseBranch) == expectedBase &&
		isExpectedReleaseBranch(current.SourceBranch, expectedBase, releaseBranch)
}

func checkAutoMergeCandidate(current mergeState) error {
	switch {
	case !current.IsOpen:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonClosed, "is closed")
	case current.IsDraft:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonDraft, "is draft")
	case current.HasConflicts:
		return blockedMerge(current.Reference, forge.MergeBlockedReasonConflicts, "has conflicts")
	default:
		return nil
	}
}
