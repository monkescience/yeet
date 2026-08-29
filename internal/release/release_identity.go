package release

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

const (
	releaseManifestMarkerPrefix = "<!-- yeet-release-manifest"
	releaseManifestMarkerSuffix = "-->"
)

type releaseManifest struct {
	Unit              string                  `json:"unit,omitempty"`
	BaseBranch        string                  `json:"base_branch"`
	Channel           string                  `json:"channel,omitempty"`
	Prerelease        bool                    `json:"prerelease,omitzero"`
	ConfiguredTargets []releaseManifestTarget `json:"configured_targets,omitempty"`
	Targets           []releaseManifestEntry  `json:"targets"`
}

type releaseManifestTarget struct {
	ID   string `json:"id"`
	Type string `json:"type"`
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

func releaseManifestForPlans(baseBranch string, plans []TargetPlan, unitIDs ...string) releaseManifest {
	manifest := releaseManifest{
		BaseBranch: baseBranch,
		Targets:    make([]releaseManifestEntry, 0, len(plans)),
	}
	if len(unitIDs) > 0 && unitIDs[0] != combinedReleaseUnitID {
		manifest.Unit = strings.TrimSpace(unitIDs[0])
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
	units ...releaseUnit,
) (releaseManifest, error) {
	expectedBranch := c.run.releaseBranch
	expectedUnitID := combinedReleaseUnitID

	if len(units) > 0 {
		expectedBranch = units[0].ReleaseBranch
		expectedUnitID = units[0].ID
	}

	err := c.validateReleaseManifestContext(pullRequest, manifest, expectedBranch, expectedUnitID)
	if err != nil {
		return releaseManifest{}, err
	}

	validatedTargets := make([]releaseManifestEntry, len(manifest.Targets))
	seenTargets := make(map[string]struct{}, len(manifest.Targets))

	for index, entry := range manifest.Targets {
		changelogFile, entryErr := c.validateReleaseManifestEntry(entry, seenTargets)
		if entryErr != nil {
			return releaseManifest{}, entryErr
		}

		entry.ChangelogFile = changelogFile
		validatedTargets[index] = entry
	}

	manifest.Targets = validatedTargets

	if len(units) == 0 {
		return manifest, nil
	}

	expectedUnit := units[0]
	expectedUnit.Plans = c.configuredUnitPlans(expectedUnit.ID)

	err = validateManifestUnitTargets(manifest.ConfiguredTargets, manifest.Targets, expectedUnit)
	if err != nil {
		return releaseManifest{}, err
	}

	return manifest, nil
}

func (c *releaseCore) validateReleaseManifestContext(
	pullRequest *forge.PullRequest,
	manifest releaseManifest,
	expectedBranch, expectedUnitID string,
) error {
	if pullRequest == nil {
		return fmt.Errorf("%w: missing pull request", errInvalidReleaseManifest)
	}

	if strings.TrimSpace(pullRequest.Branch) != expectedBranch {
		return fmt.Errorf(
			"%w: pull request branch %q does not match %q",
			errInvalidReleaseManifest,
			pullRequest.Branch,
			expectedBranch,
		)
	}

	manifestUnitID := strings.TrimSpace(manifest.Unit)
	if manifestUnitID == "" {
		manifestUnitID = combinedReleaseUnitID
	}

	if manifestUnitID != expectedUnitID {
		return fmt.Errorf(
			"%w: unit %q does not match %q",
			errInvalidReleaseManifest,
			manifestUnitID,
			expectedUnitID,
		)
	}

	if strings.TrimSpace(manifest.BaseBranch) != c.run.baseBranch {
		return fmt.Errorf(
			"%w: base branch %q does not match %q",
			errInvalidReleaseManifest,
			manifest.BaseBranch,
			c.run.baseBranch,
		)
	}

	if strings.TrimSpace(manifest.Channel) != c.run.channelName {
		return fmt.Errorf(
			"%w: channel %q does not match %q",
			errInvalidReleaseManifest,
			manifest.Channel,
			c.run.channelName,
		)
	}

	if manifest.Prerelease != c.run.isPrerelease() {
		return fmt.Errorf(
			"%w: prerelease value does not match active release mode",
			errInvalidReleaseManifest,
		)
	}

	return nil
}

func validateManifestUnitTargets(
	configuredTargets []releaseManifestTarget,
	entries []releaseManifestEntry,
	unit releaseUnit,
) error {
	if unit.ID == combinedReleaseUnitID {
		return nil
	}

	allowed := make(map[string]string, len(unit.Plans))
	for _, plan := range unit.Plans {
		allowed[plan.ID] = string(plan.Type)
	}

	configured := make(map[string]string, len(configuredTargets))
	for _, target := range configuredTargets {
		configured[strings.TrimSpace(target.ID)] = strings.TrimSpace(target.Type)
	}

	if len(configured) != len(allowed) {
		return fmt.Errorf(
			"%w: configured targets do not match unit %q",
			errInvalidReleaseManifest,
			unit.ID,
		)
	}

	for targetID, targetType := range allowed {
		if configured[targetID] != targetType {
			return fmt.Errorf(
				"%w: configured target %q does not match unit %q",
				errInvalidReleaseManifest,
				targetID,
				unit.ID,
			)
		}
	}

	for _, entry := range entries {
		if _, exists := allowed[strings.TrimSpace(entry.ID)]; !exists {
			return fmt.Errorf(
				"%w: target %q does not belong to unit %q",
				errInvalidReleaseManifest,
				entry.ID,
				unit.ID,
			)
		}
	}

	return nil
}

func (c *releaseCore) validateReleaseManifestEntry(
	entry releaseManifestEntry,
	seenTargets map[string]struct{},
) (string, error) {
	targetID := strings.TrimSpace(entry.ID)
	if _, exists := seenTargets[targetID]; exists {
		return "", fmt.Errorf("%w: duplicate target %q", errInvalidReleaseManifest, targetID)
	}

	target, exists := c.targets[targetID]
	if !exists {
		return "", fmt.Errorf("%w: unknown target %q", errInvalidReleaseManifest, targetID)
	}

	seenTargets[targetID] = struct{}{}

	if strings.TrimSpace(entry.Type) != string(target.Type) {
		return "", fmt.Errorf("%w: target %q type does not match configuration", errInvalidReleaseManifest, targetID)
	}

	manifestChangelogFile, err := config.NormalizeRepoFilePath(entry.ChangelogFile)
	if err != nil || manifestChangelogFile != target.Changelog.File {
		return "", fmt.Errorf(
			"%w: target %q changelog file does not match configuration",
			errInvalidReleaseManifest,
			targetID,
		)
	}

	if !strings.HasPrefix(entry.Tag, target.TagPrefix) {
		return "", fmt.Errorf("%w: target %q tag has an invalid prefix", errInvalidReleaseManifest, targetID)
	}

	strategy := versionStrategyForResolvedTarget(target)

	_, err = strategy.strategy.Current(entry.Tag)
	if err != nil {
		return "", fmt.Errorf("%w: target %q tag is invalid: %v", errInvalidReleaseManifest, targetID, err)
	}

	return manifestChangelogFile, nil
}
