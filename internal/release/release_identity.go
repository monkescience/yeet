package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/monkescience/yeet/internal/provider"
)

const (
	releaseBranchPrefix         = "yeet/release-"
	releaseTagMarkerPrefix      = "<!-- yeet-release-tag:"
	releaseTagMarkerSuffix      = "-->"
	releaseManifestMarkerPrefix = "<!-- yeet-release-manifest"
	releaseManifestMarkerSuffix = "-->"
)

type releaseManifest struct {
	BaseBranch string                 `json:"base_branch"`
	Targets    []releaseManifestEntry `json:"targets"`
}

type releaseManifestEntry struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Tag           string `json:"tag"`
	ChangelogFile string `json:"changelog_file"`
}

var ErrInvalidReleaseManifest = errors.New("invalid release manifest")

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

func releaseManifestForResult(result *Result) releaseManifest {
	return releaseManifestForPlans(result.BaseBranch, result.Plans)
}

func releaseManifestForPlans(baseBranch string, plans []TargetPlan) releaseManifest {
	manifest := releaseManifest{
		BaseBranch: baseBranch,
		Targets:    make([]releaseManifestEntry, 0, len(plans)),
	}

	for _, plan := range plans {
		manifest.Targets = append(manifest.Targets, releaseManifestEntry{
			ID:            plan.ID,
			Type:          plan.Type,
			Tag:           plan.NextTag,
			ChangelogFile: plan.Files["changelog_file"],
		})
	}

	return manifest
}

func releaseManifestMarker(manifest releaseManifest) (string, error) {
	if len(manifest.Targets) == 0 {
		return "", nil
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal release manifest: %w", err)
	}

	return fmt.Sprintf("%s\n%s\n%s", releaseManifestMarkerPrefix, string(manifestData), releaseManifestMarkerSuffix), nil
}

func releaseManifestFromPullRequest(
	pullRequest *provider.PullRequest,
	fallbackChangelogFile string,
) (releaseManifest, error) {
	manifest, ok, err := releaseManifestFromBody(pullRequest.Body)
	if ok || err != nil {
		return manifest, err
	}

	releaseTag, err := releaseTagFromPullRequest(pullRequest)
	if err != nil {
		return releaseManifest{}, err
	}

	return releaseManifest{
		Targets: []releaseManifestEntry{{
			ID:            "default",
			Type:          "path",
			Tag:           releaseTag,
			ChangelogFile: fallbackChangelogFile,
		}},
	}, nil
}

func releaseManifestFromBody(body string) (releaseManifest, bool, error) {
	start := strings.Index(body, releaseManifestMarkerPrefix)
	if start == -1 {
		return releaseManifest{}, false, nil
	}

	start += len(releaseManifestMarkerPrefix)

	end := strings.Index(body[start:], releaseManifestMarkerSuffix)
	if end == -1 {
		return releaseManifest{}, true, ErrInvalidReleaseManifest
	}

	manifestBody := strings.TrimSpace(body[start : start+end])
	if manifestBody == "" {
		return releaseManifest{}, true, ErrInvalidReleaseManifest
	}

	var manifest releaseManifest

	err := json.Unmarshal([]byte(manifestBody), &manifest)
	if err != nil {
		return releaseManifest{}, true, fmt.Errorf("parse release manifest: %w", err)
	}

	if len(manifest.Targets) == 0 {
		return releaseManifest{}, true, ErrInvalidReleaseManifest
	}

	return manifest, true, nil
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
