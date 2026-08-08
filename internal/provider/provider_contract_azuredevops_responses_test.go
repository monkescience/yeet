package provider_test

const (
	azureDevOpsContractForkRepo            = "attacker-repo"
	azureDevOpsContractBaseSHA             = "base-sha"
	azureDevOpsContractOtherSHA            = "other-sha"
	azureDevOpsContractReleaseTipSHA       = "release-tip"
	azureDevOpsContractStaleTagSHA         = "stale-tag-sha"
	azureDevOpsContractTagObjectID         = "tag-object-123"
	azureDevOpsContractPreviousTagObjectID = "tag-object-122"
	azureDevOpsContractNewTagObjectID      = "new-tag-object"
	azureDevOpsContractZeroSHA             = "0000000000000000000000000000000000000000"
	azureDevOpsContractLabelID             = "00000000-0000-0000-0000-000000000042"
	azureDevOpsContractAmbiguousFirstID    = "33333333-3333-3333-3333-333333333333"
	azureDevOpsContractAmbiguousSecondID   = "44444444-4444-4444-4444-444444444444"
)

func azureDevOpsListResponse(values ...map[string]any) map[string]any {
	if values == nil {
		values = []map[string]any{}
	}

	return map[string]any{"count": len(values), "value": values}
}

func azureDevOpsRefResponse(name, objectID string) map[string]any {
	return map[string]any{"name": name, "objectId": objectID}
}

func azureDevOpsBranchHeadRefsResponse() map[string]any {
	return azureDevOpsListResponse(
		azureDevOpsRefResponse("refs/heads/"+providerContractBaseBranch+"2", azureDevOpsContractOtherSHA),
		azureDevOpsRefResponse("refs/heads/"+providerContractBaseBranch, providerContractHeadSHA),
	)
}

func azureDevOpsTagRefsResponse() map[string]any {
	return azureDevOpsListResponse(
		map[string]any{
			"name":           "refs/tags/" + providerContractTag,
			"objectId":       azureDevOpsContractTagObjectID,
			"peeledObjectId": providerContractTagCommitSHA,
		},
		map[string]any{
			"name":           "refs/tags/" + providerContractPreviousTag,
			"objectId":       azureDevOpsContractPreviousTagObjectID,
			"peeledObjectId": providerContractPreviousTagCommitSHA,
		},
	)
}

func azureDevOpsReleaseBranchRefUpdateResponse(oldObjectID string) map[string]any {
	return azureDevOpsListResponse(map[string]any{
		"name":         "refs/heads/" + providerContractReleaseBranch,
		"oldObjectId":  oldObjectID,
		"newObjectId":  azureDevOpsContractBaseSHA,
		"success":      true,
		"updateStatus": "succeeded",
	})
}

func azureDevOpsAnnotatedTagResponse(objectID string) map[string]any {
	return map[string]any{
		"name":         providerContractTag,
		"objectId":     objectID,
		"message":      providerContractReleaseNotes,
		"url":          providerContractReleaseURL,
		"taggedObject": map[string]any{"objectId": providerContractHeadSHA},
	}
}

func azureDevOpsIdentityResponse(id, displayName string) map[string]any {
	return map[string]any{"id": id, "providerDisplayName": displayName}
}

func azureDevOpsReleasePRResponse() map[string]any {
	return map[string]any{
		"pullRequestId": providerContractPRNumber,
		"title":         providerContractReleaseTitle,
		"description":   providerContractReleaseBody,
		"url":           "https://example.com/_apis/git/repositories/r/pullRequests/42",
		"sourceRefName": "refs/heads/" + providerContractReleaseBranch,
		"targetRefName": "refs/heads/" + providerContractBaseBranch,
	}
}

func azureDevOpsUpdatedPRResponse() map[string]any {
	return map[string]any{
		"pullRequestId": providerContractPRNumber,
		"title":         providerContractReleaseTitle,
		"description":   providerContractUpdatedReleaseBody,
	}
}

func azureDevOpsLabelsResponse(labels ...string) []map[string]any {
	values := make([]map[string]any, 0, len(labels))
	for _, label := range labels {
		values = append(values, map[string]any{"name": label, "active": true})
	}

	return values
}

func azureDevOpsListedPRResponse(id int, branch string, labels ...string) map[string]any {
	return map[string]any{
		"pullRequestId": id,
		"repository":    map[string]any{"name": azureDevOpsContractRepo},
		"title":         providerContractReleaseTitle,
		"description":   providerContractReleaseBody,
		"url":           providerContractReleasePRURL,
		"sourceRefName": "refs/heads/" + branch,
		"targetRefName": "refs/heads/" + providerContractBaseBranch,
		"status":        "active",
		"labels":        azureDevOpsLabelsResponse(labels...),
	}
}

func azureDevOpsOpenPRsResponse() map[string]any {
	pending := azureDevOpsListedPRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		providerContractPendingLabel,
	)

	forged := azureDevOpsListedPRResponse(
		providerContractForgedPRNumber,
		providerContractPendingBranch,
		testReleaseLabelPending,
	)
	forged["title"] = providerContractForgedTitle
	forged["description"] = providerContractUntrustedBody
	forged["url"] = "https://example.com/pulls/66"
	forged["forkSource"] = map[string]any{
		"repository": map[string]any{"name": azureDevOpsContractForkRepo},
	}

	lookalike := azureDevOpsListedPRResponse(
		providerContractLookalikePRNumber,
		providerContractLookalikeBranch,
		testReleaseLabelPending,
	)
	lookalike["title"] = providerContractLookalikeTitle
	lookalike["description"] = providerContractUntrustedBody
	lookalike["url"] = "https://example.com/pulls/67"

	return azureDevOpsListResponse(pending, forged, lookalike)
}

func azureDevOpsLabelledOpenPRsResponse(labels ...string) map[string]any {
	return azureDevOpsListResponse(azureDevOpsListedPRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		labels...,
	))
}

func azureDevOpsMergedPRsResponse() map[string]any {
	pr := azureDevOpsListedPRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		providerContractPendingLabel,
	)
	delete(pr, "description")
	pr["status"] = "completed"

	return azureDevOpsListResponse(pr)
}

func azureDevOpsMergedPRResponse() map[string]any {
	pr := azureDevOpsListedPRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		testReleaseLabelPending,
	)
	delete(pr, "url")
	pr["status"] = "completed"
	pr["lastMergeCommit"] = map[string]any{"commitId": providerContractMergeSHA}

	return pr
}

func azureDevOpsMergeStatePRResponse(mergeStatus string) map[string]any {
	return map[string]any{
		"pullRequestId": providerContractPRNumber,
		"repository":    map[string]any{"name": azureDevOpsContractRepo},
		"status":        "active",
		"mergeStatus":   mergeStatus,
		"isDraft":       false,
		"sourceRefName": "refs/heads/" + providerContractPendingBranch,
		"targetRefName": "refs/heads/" + providerContractBaseBranch,
	}
}

func azureDevOpsMergeablePRResponse() map[string]any {
	pr := azureDevOpsMergeStatePRResponse("succeeded")
	pr["lastMergeSourceCommit"] = map[string]any{"commitId": providerContractHeadSHA}

	return pr
}

func azureDevOpsCompletedPRResponse() map[string]any {
	return map[string]any{
		"pullRequestId":   providerContractPRNumber,
		"status":          "completed",
		"mergeStatus":     "succeeded",
		"lastMergeCommit": map[string]any{"commitId": providerContractMergeSHA},
	}
}
