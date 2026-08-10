package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/forge"
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

var errInvalidReleaseManifest = errors.New("invalid release manifest")

var releaseManifestMarkerOpenRE = regexp.MustCompile(`<!--\s*yeet-release-manifest\b\s*`)

func releaseRefForPullRequest(pullRequest *forge.PullRequest) (string, error) {
	mergeCommitSHA := strings.TrimSpace(pullRequest.MergeCommitSHA)
	if mergeCommitSHA == "" {
		return "", fmt.Errorf("merged release pull request #%d: %w", pullRequest.Number, forge.ErrEmptyCommitSHA)
	}

	return mergeCommitSHA, nil
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

func releaseManifestFromPullRequest(pullRequest *forge.PullRequest) (releaseManifest, error) {
	manifest, ok, err := releaseManifestFromBody(pullRequest.Body)
	if !ok && err == nil {
		return releaseManifest{}, fmt.Errorf(
			"%w: missing manifest marker in pull request #%d",
			errInvalidReleaseManifest,
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
		return releaseManifest{}, true, fmt.Errorf("%w: multiple manifest markers in body", errInvalidReleaseManifest)
	}

	manifestStart := markers[0][1]

	end := strings.Index(body[manifestStart:], releaseManifestMarkerSuffix)
	if end == -1 {
		return releaseManifest{}, true, errInvalidReleaseManifest
	}

	manifestBody := strings.TrimSpace(body[manifestStart : manifestStart+end])
	if manifestBody == "" {
		return releaseManifest{}, true, errInvalidReleaseManifest
	}

	var manifest releaseManifest

	err := json.Unmarshal([]byte(manifestBody), &manifest)
	if err != nil {
		return releaseManifest{}, true, fmt.Errorf("parse release manifest: %w", err)
	}

	if len(manifest.Targets) == 0 {
		return releaseManifest{}, true, errInvalidReleaseManifest
	}

	return manifest, true, nil
}

func (c *releaseCore) validateReleaseManifest(
	pullRequest *forge.PullRequest,
	manifest releaseManifest,
) error {
	if pullRequest == nil {
		return fmt.Errorf("%w: missing pull request", errInvalidReleaseManifest)
	}

	expectedBranch := stableReleaseBranch(c.cfg.Branch)
	if strings.TrimSpace(pullRequest.Branch) != expectedBranch {
		return fmt.Errorf(
			"%w: pull request branch %q does not match %q",
			errInvalidReleaseManifest,
			pullRequest.Branch,
			expectedBranch,
		)
	}

	if strings.TrimSpace(manifest.BaseBranch) != strings.TrimSpace(c.cfg.Branch) {
		return fmt.Errorf(
			"%w: base branch %q does not match %q",
			errInvalidReleaseManifest,
			manifest.BaseBranch,
			c.cfg.Branch,
		)
	}

	if strings.TrimSpace(manifest.Channel) != strings.TrimSpace(c.cfg.ActiveChannel) {
		return fmt.Errorf(
			"%w: channel %q does not match %q",
			errInvalidReleaseManifest,
			manifest.Channel,
			c.cfg.ActiveChannel,
		)
	}

	if manifest.Prerelease != c.isPrerelease() {
		return fmt.Errorf(
			"%w: prerelease value does not match active release mode",
			errInvalidReleaseManifest,
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
		return fmt.Errorf("%w: duplicate target %q", errInvalidReleaseManifest, targetID)
	}

	target, exists := c.targets[targetID]
	if !exists {
		return fmt.Errorf("%w: unknown target %q", errInvalidReleaseManifest, targetID)
	}

	seenTargets[targetID] = struct{}{}

	if strings.TrimSpace(entry.Type) != string(target.Type) {
		return fmt.Errorf("%w: target %q type does not match configuration", errInvalidReleaseManifest, targetID)
	}

	if strings.TrimSpace(entry.ChangelogFile) != strings.TrimSpace(target.Changelog.File) {
		return fmt.Errorf(
			"%w: target %q changelog file does not match configuration",
			errInvalidReleaseManifest,
			targetID,
		)
	}

	if !strings.HasPrefix(entry.Tag, target.TagPrefix) {
		return fmt.Errorf("%w: target %q tag has an invalid prefix", errInvalidReleaseManifest, targetID)
	}

	strategy := versionStrategyForResolvedTarget(target)
	if _, err := strategy.strategy.Current(entry.Tag); err != nil {
		return fmt.Errorf("%w: target %q tag is invalid: %v", errInvalidReleaseManifest, targetID, err)
	}

	return nil
}
