package release

import (
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/provider"
)

const (
	releaseBranchPrefix    = "yeet/release-"
	releaseTagMarkerPrefix = "<!-- yeet-release-tag:"
	releaseTagMarkerSuffix = "-->"
)

func releaseRefForPullRequest(pullRequest *provider.PullRequest, defaultRef string) string {
	mergeCommitSHA := strings.TrimSpace(pullRequest.MergeCommitSHA)
	if mergeCommitSHA != "" {
		return mergeCommitSHA
	}

	return strings.TrimSpace(defaultRef)
}

func stableReleaseBranch(targetBranch string) string {
	return releaseBranchPrefix + targetBranch
}

func releasePRTag(result *Result) string {
	if result.BaseTag != "" {
		return result.BaseTag
	}

	return result.NextTag
}

func releaseTagMarker(releaseTag string) string {
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag == "" {
		return ""
	}

	return fmt.Sprintf("%s %s %s", releaseTagMarkerPrefix, releaseTag, releaseTagMarkerSuffix)
}

func releaseTagFromPullRequest(pullRequest *provider.PullRequest) (string, error) {
	if releaseTag, ok := releaseTagFromBody(pullRequest.Body); ok {
		return releaseTag, nil
	}

	releaseTag, err := releaseTagFromBranch(pullRequest.Branch)
	if err != nil {
		return "", err
	}

	if looksLikeReleaseTag(releaseTag) {
		return releaseTag, nil
	}

	return "", fmt.Errorf(
		"%w: missing release tag marker in pull request #%d",
		ErrInvalidReleaseBranch,
		pullRequest.Number,
	)
}

func releaseTagFromBody(body string) (string, bool) {
	start := strings.Index(body, releaseTagMarkerPrefix)
	if start == -1 {
		return "", false
	}

	start += len(releaseTagMarkerPrefix)

	end := strings.Index(body[start:], releaseTagMarkerSuffix)
	if end == -1 {
		return "", false
	}

	releaseTag := strings.TrimSpace(body[start : start+end])
	if releaseTag == "" {
		return "", false
	}

	return releaseTag, true
}

func looksLikeReleaseTag(releaseTag string) bool {
	return strings.Contains(releaseTag, ".") && strings.ContainsAny(releaseTag, "0123456789")
}

func releaseTagFromBranch(branch string) (string, error) {
	if !strings.HasPrefix(branch, releaseBranchPrefix) {
		return "", fmt.Errorf("%w: %s", ErrInvalidReleaseBranch, branch)
	}

	tag := strings.TrimPrefix(branch, releaseBranchPrefix)
	if tag == "" {
		return "", fmt.Errorf("%w: %s", ErrInvalidReleaseBranch, branch)
	}

	return tag, nil
}

func isPreviewTag(tag, prefix string) bool {
	versionPart := strings.TrimPrefix(tag, prefix)
	if versionPart == tag {
		for idx := range len(tag) {
			if tag[idx] >= '0' && tag[idx] <= '9' {
				versionPart = tag[idx:]

				break
			}
		}
	}

	return strings.Contains(versionPart, "+") || strings.Contains(versionPart, "-")
}
