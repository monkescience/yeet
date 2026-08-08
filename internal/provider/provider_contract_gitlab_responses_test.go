package provider_test

const (
	gitLabContractProjectID     = 10
	gitLabContractForkProjectID = 11
	gitLabContractAliceID       = 101
	gitLabContractBobID         = 102
	gitLabContractBaseRefSHA    = "base-ref-sha"
	gitLabContractNewCommitSHA  = "new-commit-sha"
)

func gitLabNotFoundResponse() map[string]any {
	return map[string]any{"message": "404 Not Found"}
}

func gitLabTagsResponse() []map[string]any {
	return []map[string]any{
		{
			"name":   providerContractTag,
			"commit": map[string]any{"id": providerContractTagCommitSHA},
		},
		{
			"name":   providerContractPreviousTag,
			"commit": map[string]any{"id": providerContractPreviousTagCommitSHA},
		},
	}
}

func gitLabBranchResponse(name, sha string) map[string]any {
	return map[string]any{
		"name":   name,
		"commit": map[string]any{"id": sha},
	}
}

func gitLabReleaseResponse() map[string]any {
	return map[string]any{
		"tag_name":    providerContractTag,
		"name":        providerContractTag,
		"description": providerContractReleaseNotes,
		"_links":      map[string]any{"self": providerContractReleaseURL},
	}
}

func gitLabMemberResponse(id int, username string) map[string]any {
	return map[string]any{"id": id, "username": username}
}

func gitLabReleaseMRResponse(description string) map[string]any {
	return map[string]any{
		"iid":           providerContractPRNumber,
		"title":         providerContractReleaseTitle,
		"description":   description,
		"web_url":       providerContractReleasePRURL,
		"source_branch": providerContractReleaseBranch,
	}
}

func gitLabReleaseMRWithReviewersResponse(reviewers ...map[string]any) map[string]any {
	mr := gitLabReleaseMRResponse(providerContractReleaseBody)
	mr["reviewers"] = reviewers

	return mr
}

func gitLabOpenMRResponse(iid int, branch string, sourceProjectID int) map[string]any {
	return map[string]any{
		"iid":               iid,
		"title":             providerContractReleaseTitle,
		"description":       providerContractReleaseBody,
		"web_url":           providerContractReleasePRURL,
		"source_branch":     branch,
		"source_project_id": sourceProjectID,
		"target_project_id": gitLabContractProjectID,
		"state":             "opened",
	}
}

func gitLabOpenMRsResponse() []map[string]any {
	pending := gitLabOpenMRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		gitLabContractProjectID,
	)

	forged := gitLabOpenMRResponse(
		providerContractForgedPRNumber,
		providerContractPendingBranch,
		gitLabContractForkProjectID,
	)
	forged["title"] = providerContractForgedTitle
	forged["description"] = providerContractUntrustedBody
	forged["web_url"] = "https://example.com/pulls/66"

	lookalike := gitLabOpenMRResponse(
		providerContractLookalikePRNumber,
		providerContractLookalikeBranch,
		gitLabContractProjectID,
	)
	lookalike["title"] = providerContractLookalikeTitle
	lookalike["description"] = providerContractUntrustedBody
	lookalike["web_url"] = "https://example.com/pulls/67"

	feature := map[string]any{
		"iid":           providerContractFeaturePRNumber,
		"title":         providerContractFeatureTitle,
		"description":   "",
		"web_url":       "https://example.com/pulls/7",
		"source_branch": providerContractFeatureBranch,
		"state":         "opened",
	}

	return []map[string]any{pending, forged, lookalike, feature}
}

func gitLabLabelledOpenMRResponse(labels []string) []map[string]any {
	if labels == nil {
		labels = []string{}
	}

	mr := gitLabOpenMRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		gitLabContractProjectID,
	)
	mr["labels"] = labels

	return []map[string]any{mr}
}

func gitLabMergedMRsResponse() []map[string]any {
	mr := gitLabOpenMRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		gitLabContractProjectID,
	)
	mr["state"] = "merged"
	mr["merge_commit_sha"] = providerContractMergeSHA

	return []map[string]any{mr}
}

func gitLabMergeStateMRResponse(detailedMergeStatus string) map[string]any {
	return map[string]any{
		"iid":                   providerContractPRNumber,
		"state":                 "opened",
		"draft":                 false,
		"has_conflicts":         false,
		"detailed_merge_status": detailedMergeStatus,
		"sha":                   providerContractHeadSHA,
		"source_branch":         providerContractPendingBranch,
		"target_branch":         providerContractBaseBranch,
		"source_project_id":     gitLabContractProjectID,
		"target_project_id":     gitLabContractProjectID,
	}
}

func gitLabConflictedMRResponse() map[string]any {
	mr := gitLabMergeStateMRResponse("conflict")
	mr["has_conflicts"] = true

	return mr
}

func gitLabMergeCommitProjectResponse() map[string]any {
	return map[string]any{"merge_method": "merge", "squash_option": "default_off"}
}

func gitLabMergeResultResponse() map[string]any {
	return map[string]any{
		"iid":              providerContractPRNumber,
		"state":            "merged",
		"merge_commit_sha": providerContractMergeSHA,
	}
}

func gitLabUpdatedMRResponse() map[string]any {
	return map[string]any{
		"iid":   providerContractPRNumber,
		"title": providerContractReleaseTitle,
	}
}

func gitLabPushResponse() map[string]any {
	return map[string]any{"id": gitLabContractNewCommitSHA}
}
