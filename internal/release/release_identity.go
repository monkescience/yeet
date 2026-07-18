package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/provider"
)

const (
	releaseBranchPrefix         = "yeet/release-"
	releaseManifestMarkerPrefix = "<!-- yeet-release-manifest"
	releaseManifestMarkerSuffix = "-->"
)

type releaseManifest struct {
	BaseBranch string                 `json:"base_branch"`
	Channel    string                 `json:"channel,omitempty"`
	Prerelease bool                   `json:"prerelease,omitempty"`
	Targets    []releaseManifestEntry `json:"targets"`
}

type releaseManifestEntry struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Tag           string `json:"tag"`
	ChangelogFile string `json:"changelog_file"`
}

var ErrInvalidReleaseManifest = errors.New("invalid release manifest")

var releaseManifestMarkerOpenRE = regexp.MustCompile(`<!--\s*yeet-release-manifest\b\s*`)

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

func releaseManifestForPlans(baseBranch string, plans []TargetPlan) releaseManifest {
	manifest := releaseManifest{
		BaseBranch: baseBranch,
		Targets:    make([]releaseManifestEntry, 0, len(plans)),
	}

	for _, plan := range plans {
		manifest.Targets = append(manifest.Targets, releaseManifestEntry{
			ID:            plan.ID,
			Type:          string(plan.Type),
			Tag:           plan.NextTag,
			ChangelogFile: plan.ChangelogFile,
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

func releaseManifestFromPullRequest(pullRequest *provider.PullRequest) (releaseManifest, error) {
	manifest, ok, err := releaseManifestFromBody(pullRequest.Body)
	if !ok && err == nil {
		return releaseManifest{}, fmt.Errorf(
			"%w: missing manifest marker in pull request #%d",
			ErrInvalidReleaseManifest,
			pullRequest.Number,
		)
	}

	return manifest, err
}

func releaseManifestFromBody(body string) (releaseManifest, bool, error) {
	markers := releaseManifestMarkerOpenRE.FindAllStringIndex(body, -1)
	if len(markers) == 0 {
		return releaseManifest{}, false, nil
	}

	if len(markers) > 1 {
		// Changelog content is untrusted, so duplicate markers must fail closed.
		return releaseManifest{}, true, fmt.Errorf("%w: multiple manifest markers in body", ErrInvalidReleaseManifest)
	}

	manifestStart := markers[0][1]

	end := strings.Index(body[manifestStart:], releaseManifestMarkerSuffix)
	if end == -1 {
		return releaseManifest{}, true, ErrInvalidReleaseManifest
	}

	manifestBody := strings.TrimSpace(body[manifestStart : manifestStart+end])
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

func (c *releaseCore) validateReleaseManifest(
	pullRequest *provider.PullRequest,
	manifest releaseManifest,
) error {
	if pullRequest == nil {
		return fmt.Errorf("%w: missing pull request", ErrInvalidReleaseManifest)
	}

	expectedBranch := stableReleaseBranch(c.cfg.Branch)
	if strings.TrimSpace(pullRequest.Branch) != expectedBranch {
		return fmt.Errorf(
			"%w: pull request branch %q does not match %q",
			ErrInvalidReleaseManifest,
			pullRequest.Branch,
			expectedBranch,
		)
	}

	if strings.TrimSpace(manifest.BaseBranch) != strings.TrimSpace(c.cfg.Branch) {
		return fmt.Errorf(
			"%w: base branch %q does not match %q",
			ErrInvalidReleaseManifest,
			manifest.BaseBranch,
			c.cfg.Branch,
		)
	}

	if strings.TrimSpace(manifest.Channel) != strings.TrimSpace(c.cfg.ActiveChannel) {
		return fmt.Errorf(
			"%w: channel %q does not match %q",
			ErrInvalidReleaseManifest,
			manifest.Channel,
			c.cfg.ActiveChannel,
		)
	}

	if manifest.Prerelease != c.isPrerelease() {
		return fmt.Errorf(
			"%w: prerelease value does not match active release mode",
			ErrInvalidReleaseManifest,
		)
	}

	seenTargets := make(map[string]struct{}, len(manifest.Targets))
	for _, entry := range manifest.Targets {
		if err := c.validateReleaseManifestEntry(entry, seenTargets); err != nil {
			return err
		}
	}

	return nil
}

func (c *releaseCore) validateReleaseManifestEntry(
	entry releaseManifestEntry,
	seenTargets map[string]struct{},
) error {
	targetID := strings.TrimSpace(entry.ID)
	if _, exists := seenTargets[targetID]; exists {
		return fmt.Errorf("%w: duplicate target %q", ErrInvalidReleaseManifest, targetID)
	}

	target, exists := c.targets[targetID]
	if !exists {
		return fmt.Errorf("%w: unknown target %q", ErrInvalidReleaseManifest, targetID)
	}

	seenTargets[targetID] = struct{}{}

	if strings.TrimSpace(entry.Type) != string(target.Type) {
		return fmt.Errorf("%w: target %q type does not match configuration", ErrInvalidReleaseManifest, targetID)
	}

	if strings.TrimSpace(entry.ChangelogFile) != strings.TrimSpace(target.Changelog.File) {
		return fmt.Errorf(
			"%w: target %q changelog file does not match configuration",
			ErrInvalidReleaseManifest,
			targetID,
		)
	}

	if !strings.HasPrefix(entry.Tag, target.TagPrefix) {
		return fmt.Errorf("%w: target %q tag has an invalid prefix", ErrInvalidReleaseManifest, targetID)
	}

	strategy := versionStrategyForResolvedTarget(target)
	if _, err := strategy.strategy.Current(entry.Tag); err != nil {
		return fmt.Errorf("%w: target %q tag is invalid: %v", ErrInvalidReleaseManifest, targetID, err)
	}

	return nil
}
