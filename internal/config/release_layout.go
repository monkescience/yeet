package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type ReleaseLayout struct {
	units         []ConfiguredReleaseUnit
	groupByTarget map[string][]string
}

type ConfiguredReleaseUnit struct {
	ID          string
	BranchValue string
	TargetIDs   []string
}

func (c *Config) ReleaseLayout() ReleaseLayout {
	targetIDs := slices.Sorted(maps.Keys(c.Targets))
	if c.Release.PullRequestMode != PullRequestModeIndependent {
		return ReleaseLayout{units: []ConfiguredReleaseUnit{{
			ID:        "combined",
			TargetIDs: normalizeTargetIDs(targetIDs),
		}}}
	}

	layout := ReleaseLayout{
		units:         make([]ConfiguredReleaseUnit, 0, len(c.Targets)+len(c.Release.Groups)),
		groupByTarget: make(map[string][]string),
	}

	for _, groupName := range slices.Sorted(maps.Keys(c.Release.Groups)) {
		members := normalizeTargetIDs(c.Release.Groups[groupName].Targets)
		for _, targetID := range members {
			layout.groupByTarget[targetID] = members
		}

		orderedMembers := slices.Clone(c.Release.Groups[groupName].Targets)
		slices.Sort(orderedMembers)
		layout.units = append(layout.units, ConfiguredReleaseUnit{
			ID:          "group:" + strings.TrimSpace(groupName),
			BranchValue: releaseUnitBranchValue("group", groupName),
			TargetIDs:   normalizeTargetIDs(orderedMembers),
		})
	}

	for _, targetID := range targetIDs {
		normalized := strings.TrimSpace(targetID)
		if _, grouped := layout.groupByTarget[normalized]; grouped {
			continue
		}

		layout.units = append(layout.units, ConfiguredReleaseUnit{
			ID:          "target:" + normalized,
			BranchValue: releaseUnitBranchValue("target", targetID),
			TargetIDs:   []string{normalized},
		})
	}

	return layout
}

func (l ReleaseLayout) Units() []ConfiguredReleaseUnit {
	units := slices.Clone(l.units)
	for index := range units {
		units[index].TargetIDs = slices.Clone(units[index].TargetIDs)
	}

	return units
}

func (l ReleaseLayout) ExpandSelection(targetIDs []string) []string {
	if len(l.groupByTarget) == 0 {
		return slices.Clone(targetIDs)
	}

	expanded := make([]string, 0, len(targetIDs))
	seen := make(map[string]struct{})

	for _, targetID := range targetIDs {
		normalized := strings.TrimSpace(targetID)

		members := l.groupByTarget[normalized]
		if len(members) == 0 {
			members = []string{normalized}
		}

		for _, member := range members {
			if _, exists := seen[member]; exists {
				continue
			}

			seen[member] = struct{}{}
			expanded = append(expanded, member)
		}
	}

	return expanded
}

func releaseUnitBranchValue(kind, name string) string {
	normalized := strings.TrimSpace(name)

	var slug strings.Builder

	lastWasSeparator := false

	for _, r := range strings.ToLower(normalized) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			slug.WriteRune(r)

			lastWasSeparator = false

			continue
		}

		if slug.Len() > 0 && !lastWasSeparator {
			slug.WriteByte('-')

			lastWasSeparator = true
		}
	}

	readable := strings.Trim(slug.String(), "-")
	if readable == "" {
		readable = "unit"
	}

	const maxReadableLength = 40
	if len(readable) > maxReadableLength {
		readable = strings.TrimRight(readable[:maxReadableLength], "-")
	}

	digest := sha256.Sum256([]byte(kind + "\x00" + normalized))

	return kind + "-" + readable + "-" + hex.EncodeToString(digest[:5])
}

func (l ReleaseLayout) validateFileOwnership(targets map[string]ResolvedTarget) error {
	unitByTarget := make(map[string]string, len(targets))

	for _, unit := range l.units {
		for _, targetID := range unit.TargetIDs {
			unitByTarget[targetID] = unit.ID
		}
	}

	owners := make(map[string]string)

	for _, targetID := range slices.Sorted(maps.Keys(targets)) {
		target := targets[targetID]
		unitID := unitByTarget[targetID]

		paths := make([]string, 0, len(target.VersionFiles)+1)

		paths = append(paths, target.Changelog.File)
		for _, versionFile := range target.VersionFiles {
			paths = append(paths, versionFile.Path)
		}

		for _, path := range paths {
			previous, exists := owners[path]
			if exists && previous != unitID {
				return fmt.Errorf(
					"%w: release units %q and %q both write %q, "+
						"configure separate files or place the targets in one atomic group",
					ErrInvalidConfig,
					previous,
					unitID,
					path,
				)
			}

			owners[path] = unitID
		}
	}

	return nil
}

func validateReleaseGroups(release ReleaseConfig, targets map[string]ResolvedTarget) error {
	err := validateReleaseGroupMode(release)
	if err != nil {
		return err
	}

	membership := make(map[string]string)

	for _, rawName := range slices.Sorted(maps.Keys(release.Groups)) {
		err = validateReleaseGroup(rawName, release.Groups[rawName], targets, membership)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateReleaseGroupMode(release ReleaseConfig) error {
	switch release.PullRequestMode {
	case PullRequestModeCombined:
		if len(release.Groups) > 0 {
			return fmt.Errorf(
				"%w: release.groups is only valid when release.pull_request_mode is %q",
				ErrInvalidConfig,
				PullRequestModeIndependent,
			)
		}
	case PullRequestModeIndependent:
	default:
		return fmt.Errorf(
			"%w: release.pull_request_mode must be %q or %q, got %q",
			ErrInvalidConfig,
			PullRequestModeCombined,
			PullRequestModeIndependent,
			release.PullRequestMode,
		)
	}

	return nil
}

func validateReleaseGroup(
	rawName string,
	group ReleaseGroupConfig,
	targets map[string]ResolvedTarget,
	membership map[string]string,
) error {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return fmt.Errorf("%w: release.groups keys must not be empty", ErrInvalidConfig)
	}

	if len(group.Targets) == 0 {
		return fmt.Errorf("%w: release.groups.%s.targets must not be empty", ErrInvalidConfig, name)
	}

	for _, rawTargetID := range group.Targets {
		targetID := strings.TrimSpace(rawTargetID)
		if targetID == "" {
			return fmt.Errorf(
				"%w: release.groups.%s.targets must not contain empty target IDs",
				ErrInvalidConfig,
				name,
			)
		}

		if _, exists := targets[targetID]; !exists {
			return fmt.Errorf(
				"%w: release.groups.%s.targets contains unknown target %q",
				ErrInvalidConfig,
				name,
				targetID,
			)
		}

		if previous, exists := membership[targetID]; exists {
			return fmt.Errorf(
				"%w: target %q belongs to both release.groups.%s and release.groups.%s",
				ErrInvalidConfig,
				targetID,
				previous,
				name,
			)
		}

		membership[targetID] = name
	}

	return nil
}
