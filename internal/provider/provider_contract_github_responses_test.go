package provider_test

import "encoding/base64"

const (
	gitHubContractRepoFullName    = "o/r"
	gitHubContractForkFullName    = "attacker/r"
	gitHubContractBaseRefSHA      = "base-ref-sha"
	gitHubContractBaseTreeSHA     = "base-tree-sha"
	gitHubContractNewTreeSHA      = "new-tree-sha"
	gitHubContractNewCommitSHA    = "new-commit-sha"
	gitHubContractTagObjectSHA    = "tag-object-sha"
	gitHubContractReviewerRefusal = "Reviews may only be requested from collaborators."
)

func gitHubNotFoundResponse() map[string]any {
	return map[string]any{"message": "Not Found"}
}

func gitHubReviewerRefusalResponse() map[string]any {
	return map[string]any{
		"message":           gitHubContractReviewerRefusal,
		"documentation_url": "https://docs.github.com/rest/pulls/review-requests",
	}
}

func gitHubTagsResponse() []map[string]any {
	return []map[string]any{
		{
			"name":   providerContractTag,
			"commit": map[string]any{"sha": providerContractTagCommitSHA},
		},
		{
			"name":   providerContractPreviousTag,
			"commit": map[string]any{"sha": providerContractPreviousTagCommitSHA},
		},
	}
}

func gitHubTagsPageTwoResponse() []map[string]any {
	return []map[string]any{
		{
			"name":   providerContractOlderTag,
			"commit": map[string]any{"sha": providerContractOlderTagCommitSHA},
		},
		{
			"name":   providerContractOldestTag,
			"commit": map[string]any{"sha": providerContractOldestTagCommitSHA},
		},
	}
}

func gitHubSHAResponse(sha string) map[string]any {
	return map[string]any{"sha": sha}
}

func gitHubRefResponse(ref, sha, objectType string) map[string]any {
	return map[string]any{
		"ref":    ref,
		"object": map[string]any{"sha": sha, "type": objectType},
	}
}

func gitHubReleaseResponse() map[string]any {
	return map[string]any{
		"tag_name": providerContractTag,
		"name":     providerContractTag,
		"body":     providerContractReleaseNotes,
		"html_url": providerContractReleaseURL,
	}
}

func gitHubCreatedReleaseResponse() map[string]any {
	release := gitHubReleaseResponse()
	release["target_commitish"] = providerContractBaseBranch

	return release
}

func gitHubTagObjectResponse() map[string]any {
	return map[string]any{
		"tag":     providerContractTag,
		"sha":     gitHubContractTagObjectSHA,
		"message": providerContractReleaseNotes,
	}
}

func gitHubUserResponse() map[string]any {
	return map[string]any{
		"login": "yeet-bot",
		"name":  "Yeet Bot",
		"email": "noreply@yeet.dev",
	}
}

func gitHubReleasePRResponse(body string) map[string]any {
	return map[string]any{
		"number":   providerContractPRNumber,
		"title":    providerContractReleaseTitle,
		"body":     body,
		"html_url": providerContractReleasePRURL,
		"head":     map[string]any{"ref": providerContractReleaseBranch},
	}
}

func gitHubMergedPRResponse() map[string]any {
	return map[string]any{
		"number":           providerContractPRNumber,
		"title":            providerContractReleaseTitle,
		"body":             providerContractReleaseBody,
		"html_url":         providerContractReleasePRURL,
		"merge_commit_sha": providerContractMergeSHA,
		"head": map[string]any{
			"ref":  providerContractPendingBranch,
			"repo": map[string]any{"full_name": gitHubContractRepoFullName},
		},
	}
}

func gitHubSearchIssuesResponse(items ...map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}

	return map[string]any{
		"total_count":        len(items),
		"incomplete_results": false,
		"items":              items,
	}
}

func gitHubOpenPRResponse(number int, branch string, labels []string) map[string]any {
	names := make([]map[string]any, 0, len(labels))
	for _, label := range labels {
		names = append(names, map[string]any{"name": label})
	}

	return map[string]any{
		"number":   number,
		"title":    providerContractReleaseTitle,
		"body":     providerContractReleaseBody,
		"html_url": providerContractReleasePRURL,
		"head": map[string]any{
			"ref":  branch,
			"repo": map[string]any{"full_name": gitHubContractRepoFullName},
		},
		"labels": names,
	}
}

func gitHubOpenPRsResponse() []map[string]any {
	pending := gitHubOpenPRResponse(
		providerContractPRNumber,
		providerContractPendingBranch,
		[]string{providerContractPendingLabel},
	)

	forged := gitHubOpenPRResponse(
		providerContractForgedPRNumber,
		providerContractPendingBranch,
		[]string{testReleaseLabelPending},
	)
	forged["title"] = providerContractForgedTitle
	forged["body"] = providerContractUntrustedBody
	forged["html_url"] = "https://example.com/pulls/66"
	forged["head"] = map[string]any{
		"ref":  providerContractPendingBranch,
		"repo": map[string]any{"full_name": gitHubContractForkFullName},
	}

	lookalike := gitHubOpenPRResponse(
		providerContractLookalikePRNumber,
		providerContractLookalikeBranch,
		[]string{testReleaseLabelPending},
	)
	lookalike["title"] = providerContractLookalikeTitle
	lookalike["body"] = providerContractUntrustedBody
	lookalike["html_url"] = "https://example.com/pulls/67"

	feature := gitHubOpenPRResponse(providerContractFeaturePRNumber, providerContractFeatureBranch, nil)
	feature["title"] = providerContractFeatureTitle
	feature["body"] = ""
	feature["html_url"] = "https://example.com/pulls/7"
	feature["head"] = map[string]any{"ref": providerContractFeatureBranch}

	return []map[string]any{pending, forged, lookalike, feature}
}

func gitHubFileResponse(path, content string) map[string]any {
	return map[string]any{
		"type":     "file",
		"path":     path,
		"encoding": "base64",
		"content":  base64.StdEncoding.EncodeToString([]byte(content)),
	}
}

func gitHubMergeStatePRResponse(mergeableState string) map[string]any {
	return map[string]any{
		"number":          providerContractPRNumber,
		"state":           "open",
		"merged":          false,
		"draft":           false,
		"mergeable_state": mergeableState,
		"head": map[string]any{
			"sha":  providerContractHeadSHA,
			"ref":  providerContractPendingBranch,
			"repo": map[string]any{"full_name": gitHubContractRepoFullName},
		},
		"base": map[string]any{"ref": providerContractBaseBranch},
	}
}

func gitHubSquashOnlyRepoResponse() map[string]any {
	return map[string]any{
		"allow_squash_merge": true,
		"allow_rebase_merge": false,
		"allow_merge_commit": false,
	}
}

func gitHubMergeResultResponse() map[string]any {
	return map[string]any{"merged": true, "sha": providerContractMergeSHA}
}

func gitHubBaseCommitResponse() map[string]any {
	return map[string]any{
		"sha":  gitHubContractBaseRefSHA,
		"tree": map[string]any{"sha": gitHubContractBaseTreeSHA},
	}
}
