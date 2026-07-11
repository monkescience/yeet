package config

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
)

func (c *Config) ResolvedTargets(ctx context.Context) (map[string]ResolvedTarget, error) {
	resolved, err := c.resolveTargets()
	if err != nil {
		return nil, err
	}

	for _, t := range resolved {
		slog.DebugContext(ctx, "config: resolved target",
			slog.String("id", t.ID),
			slog.String("type", string(t.Type)),
			slog.String("path", t.Path),
			slog.String("versioning", string(t.Versioning)),
		)
	}

	return resolved, nil
}

func (c *Config) resolveTargets() (map[string]ResolvedTarget, error) {
	if len(c.Targets) == 0 {
		return nil, fmt.Errorf("%w: targets must not be empty", ErrInvalidConfig)
	}

	resolved := make(map[string]ResolvedTarget, len(c.Targets))

	for id, target := range c.Targets {
		resolvedTarget, err := c.resolveTarget(id, target)
		if err != nil {
			return nil, err
		}

		if _, exists := resolved[resolvedTarget.ID]; exists {
			return nil, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
		}

		resolved[resolvedTarget.ID] = resolvedTarget
	}

	if err := validateResolvedTargets(resolved); err != nil {
		return nil, err
	}

	if err := validateTargetVersionFileOwnership(c.Targets); err != nil {
		return nil, err
	}

	return resolved, nil
}

//nolint:funlen,gocognit // Target resolution intentionally centralizes validation and defaulting.
func (c *Config) resolveTarget(id string, target Target) (ResolvedTarget, error) {
	targetID := strings.TrimSpace(id)
	if targetID == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
	}

	targetType := TargetType(strings.TrimSpace(string(target.Type)))
	if targetType != TargetTypePath && targetType != TargetTypeDerived {
		return ResolvedTarget{}, fmt.Errorf(
			"%w: targets.%s.type must be %q or %q, got %q",
			ErrInvalidConfig,
			targetID,
			TargetTypePath,
			TargetTypeDerived,
			target.Type,
		)
	}

	resolved := ResolvedTarget{
		ID:                         targetID,
		Type:                       targetType,
		TagPrefix:                  strings.TrimSpace(target.TagPrefix),
		Versioning:                 firstVersioning(target.Versioning, c.Versioning),
		PreMajorBreakingBumpsMinor: resolveBool(target.PreMajorBreakingBumpsMinor, c.PreMajorBreakingBumpsMinor),
		PreMajorFeaturesBumpPatch:  resolveBool(target.PreMajorFeaturesBumpPatch, c.PreMajorFeaturesBumpPatch),
		VersionFiles:               resolveVersionFiles(target.VersionFiles, c.VersionFiles),
		Changelog:                  mergeChangelogConfig(c.Changelog, target.Changelog),
		CalVer:                     mergeCalVerConfig(c.CalVer, target.CalVer),
		ExcludePaths:               make([]string, 0, len(target.ExcludePaths)),
		Includes:                   normalizeTargetIDs(target.Includes),
	}

	preMajorErr := validatePreMajorCalVer(targetID, resolved.Versioning, target)
	if preMajorErr != nil {
		return ResolvedTarget{}, preMajorErr
	}

	if resolved.Versioning != VersioningSemver && resolved.Versioning != VersioningCalVer {
		return ResolvedTarget{}, fmt.Errorf(
			"%w: targets.%s.versioning must be %q or %q, got %q",
			ErrInvalidConfig,
			targetID,
			VersioningSemver,
			VersioningCalVer,
			resolved.Versioning,
		)
	}

	if err := validateCalVerConfig("targets."+targetID+".calver.format", resolved.CalVer); err != nil {
		return ResolvedTarget{}, err
	}

	if resolved.TagPrefix == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.tag_prefix must not be empty", ErrInvalidConfig, targetID)
	}

	if resolved.Changelog.File == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.changelog.file must not be empty", ErrInvalidConfig, targetID)
	}

	if len(resolved.Changelog.Include) == 0 {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.changelog.include must not be empty", ErrInvalidConfig, targetID)
	}

	err := validateReferencesConfig("targets."+targetID+".changelog.references", resolved.Changelog.References)
	if err != nil {
		return ResolvedTarget{}, err
	}

	for _, versionFile := range resolved.VersionFiles {
		err := validateVersionFile("targets."+targetID+".version_files", versionFile)
		if err != nil {
			return ResolvedTarget{}, err
		}
	}

	if targetType == TargetTypePath || strings.TrimSpace(target.Path) != "" {
		normalizedPath, err := normalizeRepoPath(target.Path)
		if err != nil {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.path %v", ErrInvalidConfig, targetID, err)
		}

		resolved.Path = normalizedPath
	}

	for _, excludePath := range target.ExcludePaths {
		normalizedExcludePath, err := normalizeRepoPath(excludePath)
		if err != nil {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.exclude_paths contains %v", ErrInvalidConfig, targetID, err)
		}

		resolved.ExcludePaths = append(resolved.ExcludePaths, normalizedExcludePath)
	}

	if resolved.Path != "." {
		for _, excludePath := range resolved.ExcludePaths {
			if !RepoPathContains(resolved.Path, excludePath) {
				return ResolvedTarget{}, fmt.Errorf(
					"%w: targets.%s.exclude_paths entry %q must be inside %q",
					ErrInvalidConfig,
					targetID,
					excludePath,
					resolved.Path,
				)
			}
		}
	}

	if targetType == TargetTypePath {
		if resolved.Path == "" {
			return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.path must not be empty", ErrInvalidConfig, targetID)
		}

		if len(resolved.Includes) > 0 {
			return ResolvedTarget{}, fmt.Errorf(
				"%w: targets.%s.includes is only valid for derived targets",
				ErrInvalidConfig,
				targetID,
			)
		}
	}

	if targetType == TargetTypeDerived && len(resolved.Includes) == 0 {
		return ResolvedTarget{}, fmt.Errorf("%w: targets.%s.includes must not be empty", ErrInvalidConfig, targetID)
	}

	return resolved, nil
}

func normalizeTargetIDs(ids []string) []string {
	normalizedIDs := make([]string, 0, len(ids))

	for _, id := range ids {
		normalizedIDs = append(normalizedIDs, strings.TrimSpace(id))
	}

	return normalizedIDs
}

func firstVersioning(values ...VersioningStrategy) VersioningStrategy {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return VersioningSemver
}

func resolveBool(override *bool, defaultValue bool) bool {
	if override != nil {
		return *override
	}

	return defaultValue
}

func resolveVersionFiles(overridePaths, defaultPaths []VersionFile) []VersionFile {
	if len(overridePaths) > 0 {
		return slices.Clone(overridePaths)
	}

	return slices.Clone(defaultPaths)
}

func mergeChangelogConfig(defaultConfig, overrideConfig ChangelogConfig) ChangelogConfig {
	merged := defaultConfig

	if overrideConfig.File != "" {
		merged.File = overrideConfig.File
	}

	if len(overrideConfig.Include) > 0 {
		merged.Include = slices.Clone(overrideConfig.Include)
	}

	if len(overrideConfig.Sections) > 0 {
		merged.Sections = make(map[string]string, len(defaultConfig.Sections)+len(overrideConfig.Sections))
		maps.Copy(merged.Sections, defaultConfig.Sections)
		maps.Copy(merged.Sections, overrideConfig.Sections)
	}

	merged.References = mergeReferencesConfig(defaultConfig.References, overrideConfig.References)

	return merged
}

func mergeReferencesConfig(defaultConfig, overrideConfig ReferencesConfig) ReferencesConfig {
	merged := defaultConfig

	if len(overrideConfig.Patterns) > 0 {
		merged.Patterns = slices.Clone(overrideConfig.Patterns)
	}

	if len(overrideConfig.Footers) > 0 {
		merged.Footers = make(map[string]string, len(defaultConfig.Footers)+len(overrideConfig.Footers))
		maps.Copy(merged.Footers, defaultConfig.Footers)
		maps.Copy(merged.Footers, overrideConfig.Footers)
	}

	return merged
}

func mergeCalVerConfig(defaultConfig, overrideConfig CalVerConfig) CalVerConfig {
	merged := defaultConfig

	if overrideConfig.Format != "" {
		merged.Format = overrideConfig.Format
	}

	return merged
}

//nolint:funlen // Cross-target validation is easier to review in one place.
func validateResolvedTargets(targets map[string]ResolvedTarget) error {
	if len(targets) == 0 {
		return fmt.Errorf("%w: targets must not be empty", ErrInvalidConfig)
	}

	tagPrefixes := make(map[string]string, len(targets))

	for id, target := range targets {
		if otherID, exists := tagPrefixes[target.TagPrefix]; exists {
			return fmt.Errorf(
				"%w: targets.%s.tag_prefix %q duplicates targets.%s.tag_prefix",
				ErrInvalidConfig,
				id,
				target.TagPrefix,
				otherID,
			)
		}

		tagPrefixes[target.TagPrefix] = id
	}

	for id, target := range targets {
		if target.Type != TargetTypeDerived {
			continue
		}

		for _, includeID := range target.Includes {
			normalizedIncludeID := strings.TrimSpace(includeID)

			includedTarget, exists := targets[normalizedIncludeID]
			if !exists {
				return fmt.Errorf(
					"%w: targets.%s.includes entry %q does not refer to a defined target",
					ErrInvalidConfig,
					id,
					normalizedIncludeID,
				)
			}

			if includedTarget.Type != TargetTypePath {
				return fmt.Errorf(
					"%w: targets.%s.includes entry %q must refer to a path target in v1",
					ErrInvalidConfig,
					id,
					normalizedIncludeID,
				)
			}
		}
	}

	directTargets := make([]ResolvedTarget, 0, len(targets))
	for _, target := range targets {
		if target.Path == "" {
			continue
		}

		directTargets = append(directTargets, target)
	}

	for leftIdx := range directTargets {
		leftTarget := directTargets[leftIdx]

		for rightIdx := leftIdx + 1; rightIdx < len(directTargets); rightIdx++ {
			rightTarget := directTargets[rightIdx]

			if !directTargetsOverlap(leftTarget, rightTarget) {
				continue
			}

			return fmt.Errorf(
				"%w: direct path ownership overlaps between targets.%s and targets.%s",
				ErrInvalidConfig,
				leftTarget.ID,
				rightTarget.ID,
			)
		}
	}

	return nil
}

func validateTargetVersionFileOwnership(targets map[string]Target) error {
	targetIDs := make([]string, 0, len(targets))
	for id := range targets {
		targetIDs = append(targetIDs, id)
	}

	slices.Sort(targetIDs)

	versionFileOwners := make(map[string]string)

	for _, id := range targetIDs {
		target := targets[id]
		targetID := strings.TrimSpace(id)

		for _, versionFile := range target.VersionFiles {
			normalizedVersionFilePath := strings.TrimSpace(versionFile.Path)

			otherID, exists := versionFileOwners[normalizedVersionFilePath]
			if exists && otherID != targetID {
				return fmt.Errorf(
					"%w: targets.%s.version_files entry %q duplicates targets.%s.version_files entry",
					ErrInvalidConfig,
					targetID,
					normalizedVersionFilePath,
					otherID,
				)
			}

			versionFileOwners[normalizedVersionFilePath] = targetID
		}
	}

	return nil
}

func directTargetsOverlap(leftTarget, rightTarget ResolvedTarget) bool {
	if leftTarget.Path == "" || rightTarget.Path == "" {
		return false
	}

	samplePath := overlappingSamplePath(leftTarget.Path, rightTarget.Path)
	if samplePath == "" {
		return false
	}

	return targetOwnsPath(leftTarget, samplePath) && targetOwnsPath(rightTarget, samplePath)
}

func overlappingSamplePath(leftPath, rightPath string) string {
	if RepoPathContains(leftPath, rightPath) {
		return rightPath
	}

	if RepoPathContains(rightPath, leftPath) {
		return leftPath
	}

	return ""
}

func targetOwnsPath(target ResolvedTarget, candidate string) bool {
	if !RepoPathContains(target.Path, candidate) {
		return false
	}

	for _, excludePath := range target.ExcludePaths {
		if RepoPathContains(excludePath, candidate) {
			return false
		}
	}

	return true
}
