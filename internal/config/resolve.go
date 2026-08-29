package config

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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

	for _, id := range slices.Sorted(maps.Keys(c.Targets)) {
		target := c.Targets[id]

		resolvedTarget, err := c.resolveTarget(id, target)
		if err != nil {
			return nil, err
		}

		if _, exists := resolved[resolvedTarget.ID]; exists {
			return nil, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
		}

		resolved[resolvedTarget.ID] = resolvedTarget
	}

	err := validateResolvedTargets(resolved)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

func (c *Config) resolveTarget(id string, target Target) (ResolvedTarget, error) {
	targetID := strings.TrimSpace(id)
	if targetID == "" {
		return ResolvedTarget{}, fmt.Errorf("%w: target IDs must be unique and non-empty", ErrInvalidConfig)
	}

	targetType, err := resolveTargetType(targetID, target.Type)
	if err != nil {
		return ResolvedTarget{}, err
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
		Includes:                   normalizeTargetIDs(target.Includes),
	}

	err = validateResolvedTargetConfig(targetID, target, &resolved)
	if err != nil {
		return ResolvedTarget{}, err
	}

	resolved.Path, resolved.ExcludePaths, err = resolveTargetPaths(targetID, targetType, target)
	if err != nil {
		return ResolvedTarget{}, err
	}

	err = validateTargetShape(resolved)
	if err != nil {
		return ResolvedTarget{}, err
	}

	return resolved, nil
}

func resolveTargetType(targetID string, value TargetType) (TargetType, error) {
	if value == TargetTypePath || value == TargetTypeDerived {
		return value, nil
	}

	return "", fmt.Errorf(
		"%w: targets.%s.type must be %q or %q, got %q",
		ErrInvalidConfig,
		targetID,
		TargetTypePath,
		TargetTypeDerived,
		value,
	)
}

func validateResolvedTargetConfig(targetID string, target Target, resolved *ResolvedTarget) error {
	err := validatePreMajorCalVer(targetID, resolved.Versioning, target)
	if err != nil {
		return err
	}

	if resolved.TagPrefix == "" {
		return fmt.Errorf("%w: targets.%s.tag_prefix must not be empty", ErrInvalidConfig, targetID)
	}

	err = validateTargetVersioning(targetID, *resolved)
	if err != nil {
		return err
	}

	normalizedChangelogPath, err := normalizedChangelogFile(
		"targets."+targetID+".changelog.file",
		resolved.Changelog.File,
	)
	if err != nil {
		return err
	}

	resolved.Changelog.File = normalizedChangelogPath

	err = validateTargetChangelog(targetID, resolved.Changelog)
	if err != nil {
		return err
	}

	for index, versionFile := range resolved.VersionFiles {
		normalized, normalizeErr := normalizedVersionFile("targets."+targetID+".version_files", versionFile)
		if normalizeErr != nil {
			return normalizeErr
		}

		resolved.VersionFiles[index] = normalized
	}

	return nil
}

func validateTargetVersioning(targetID string, target ResolvedTarget) error {
	if target.Versioning != VersioningSemver && target.Versioning != VersioningCalVer {
		return fmt.Errorf(
			"%w: targets.%s.versioning must be %q or %q, got %q",
			ErrInvalidConfig,
			targetID,
			VersioningSemver,
			VersioningCalVer,
			target.Versioning,
		)
	}

	return validateCalVerConfig("targets."+targetID+".calver.format", target.CalVer)
}

func validateTargetChangelog(targetID string, changelog ChangelogConfig) error {
	if len(changelog.Include) == 0 {
		return fmt.Errorf("%w: targets.%s.changelog.include must not be empty", ErrInvalidConfig, targetID)
	}

	seen := make(map[string]struct{}, len(changelog.Include))
	for _, commitType := range changelog.Include {
		if _, exists := seen[commitType]; exists {
			return fmt.Errorf(
				"%w: targets.%s.changelog.include contains duplicate %q",
				ErrInvalidConfig,
				targetID,
				commitType,
			)
		}

		seen[commitType] = struct{}{}
	}

	configPath := "targets." + targetID + ".changelog"

	err := validateChangelogSectionHeadings(configPath, changelog)
	if err != nil {
		return err
	}

	return validateReferencesConfig("targets."+targetID+".changelog.references", changelog.References)
}

func validateChangelogSectionHeadings(configPath string, changelog ChangelogConfig) error {
	headingsByCommitType := make(map[string]string, len(changelog.Sections)+1)
	maps.Copy(headingsByCommitType, changelog.Sections)

	if _, configured := headingsByCommitType["breaking"]; !configured {
		headingsByCommitType["breaking"] = defaultBreakingChangesHeading
	}

	sectionPath := configPath + ".sections"

	headingOwners, err := validateConfiguredChangelogHeadings(sectionPath, headingsByCommitType)
	if err != nil {
		return err
	}

	return validateIncludedChangelogHeadings(configPath, changelog.Include, headingsByCommitType, headingOwners)
}

func validateConfiguredChangelogHeadings(
	configPath string,
	headingsByCommitType map[string]string,
) (map[string]string, error) {
	commitTypes := slices.Sorted(maps.Keys(headingsByCommitType))

	for _, commitType := range commitTypes {
		problem := changelogHeadingProblem(headingsByCommitType[commitType])
		if problem != "" {
			return nil, fmt.Errorf(
				"%w: %s.%s %s",
				ErrInvalidConfig,
				configPath,
				commitType,
				problem,
			)
		}
	}

	headingOwners := make(map[string]string, len(headingsByCommitType))
	for _, commitType := range commitTypes {
		heading := headingsByCommitType[commitType]

		firstCommitType, exists := headingOwners[heading]
		if exists {
			return nil, duplicateChangelogHeadingError(configPath, heading, firstCommitType, commitType)
		}

		headingOwners[heading] = commitType
	}

	return headingOwners, nil
}

func validateIncludedChangelogHeadings(
	configPath string,
	include []string,
	headingsByCommitType map[string]string,
	headingOwners map[string]string,
) error {
	sectionPath := configPath + ".sections"

	for _, commitType := range include {
		if commitType == "breaking" {
			return fmt.Errorf(
				"%w: %s.include must not contain %q because breaking changes are included automatically",
				ErrInvalidConfig,
				configPath,
				commitType,
			)
		}

		if _, isMapped := headingsByCommitType[commitType]; isMapped {
			continue
		}

		heading := changelogFallbackHeading(commitType)

		problem := changelogHeadingProblem(heading)
		if problem != "" {
			return fmt.Errorf(
				"%w: %s.include entry %q produces a section heading that %s",
				ErrInvalidConfig,
				configPath,
				commitType,
				problem,
			)
		}

		firstCommitType, exists := headingOwners[heading]
		if exists {
			return duplicateChangelogHeadingError(sectionPath, heading, firstCommitType, commitType)
		}

		headingOwners[heading] = commitType
	}

	return nil
}

func duplicateChangelogHeadingError(configPath, heading, firstCommitType, secondCommitType string) error {
	return fmt.Errorf(
		"%w: %s headings must be unique: %q is used by %q and %q",
		ErrInvalidConfig,
		configPath,
		heading,
		firstCommitType,
		secondCommitType,
	)
}

func changelogFallbackHeading(commitType string) string {
	if commitType == "" {
		return ""
	}

	first, size := utf8.DecodeRuneInString(commitType)

	return string(unicode.ToUpper(first)) + commitType[size:]
}

func changelogHeadingProblem(heading string) string {
	switch {
	case strings.TrimSpace(heading) == "":
		return "must not be blank"
	case strings.IndexFunc(heading, isChangelogLineBreak) >= 0:
		return "must be a single line"
	case strings.TrimSpace(heading) != heading:
		return "must not have leading or trailing whitespace"
	case hasMarkdownHeadingMarkers(heading):
		return "must contain heading text without leading or closing Markdown # markers"
	default:
		return ""
	}
}

func isChangelogLineBreak(r rune) bool {
	switch r {
	case '\n', '\v', '\f', '\r', '\u0085', '\u2028', '\u2029':
		return true
	default:
		return false
	}
}

func hasMarkdownHeadingMarkers(heading string) bool {
	leadingEnd := 0
	for leadingEnd < len(heading) && heading[leadingEnd] == '#' {
		leadingEnd++
	}

	if leadingEnd > 0 && (leadingEnd == len(heading) || isMarkdownHeadingSpace(heading[leadingEnd])) {
		return true
	}

	closingStart := len(heading)
	for closingStart > 0 && heading[closingStart-1] == '#' {
		closingStart--
	}

	return closingStart < len(heading) &&
		(closingStart == 0 || isMarkdownHeadingSpace(heading[closingStart-1]))
}

func isMarkdownHeadingSpace(value byte) bool {
	return value == ' ' || value == '\t'
}

func resolveTargetPaths(targetID string, targetType TargetType, target Target) (string, []string, error) {
	targetPath := ""

	if targetType == TargetTypePath || strings.TrimSpace(target.Path) != "" {
		normalizedPath, err := normalizeRepoPath(target.Path)
		if err != nil {
			return "", nil, fmt.Errorf("%w: targets.%s.path %v", ErrInvalidConfig, targetID, err)
		}

		targetPath = normalizedPath
	}

	excludePaths := make([]string, 0, len(target.ExcludePaths))
	for _, excludePath := range target.ExcludePaths {
		normalizedExcludePath, err := normalizeRepoPath(excludePath)
		if err != nil {
			return "", nil, fmt.Errorf("%w: targets.%s.exclude_paths contains %v", ErrInvalidConfig, targetID, err)
		}

		excludePaths = append(excludePaths, normalizedExcludePath)
	}

	if targetPath != "." {
		for _, excludePath := range excludePaths {
			if !RepoPathContains(targetPath, excludePath) {
				return "", nil, fmt.Errorf(
					"%w: targets.%s.exclude_paths entry %q must be inside %q",
					ErrInvalidConfig,
					targetID,
					excludePath,
					targetPath,
				)
			}
		}
	}

	return targetPath, excludePaths, nil
}

func validateTargetShape(target ResolvedTarget) error {
	if target.Type == TargetTypePath {
		if target.Path == "" {
			return fmt.Errorf("%w: targets.%s.path must not be empty", ErrInvalidConfig, target.ID)
		}

		if len(target.Includes) > 0 {
			return fmt.Errorf(
				"%w: targets.%s.includes is only valid for derived targets",
				ErrInvalidConfig,
				target.ID,
			)
		}
	}

	if target.Type == TargetTypeDerived && len(target.Includes) == 0 {
		return fmt.Errorf("%w: targets.%s.includes must not be empty", ErrInvalidConfig, target.ID)
	}

	return nil
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

func validateResolvedTargets(targets map[string]ResolvedTarget) error {
	if len(targets) == 0 {
		return fmt.Errorf("%w: targets must not be empty", ErrInvalidConfig)
	}

	err := validateUniqueTagPrefixes(targets)
	if err != nil {
		return err
	}

	err = validateDerivedIncludes(targets)
	if err != nil {
		return err
	}

	return validateDirectPathOwnership(targets)
}

func validateUniqueTagPrefixes(targets map[string]ResolvedTarget) error {
	tagPrefixes := make(map[string]string, len(targets))

	for _, id := range slices.Sorted(maps.Keys(targets)) {
		target := targets[id]
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

	return nil
}

func validateDerivedIncludes(targets map[string]ResolvedTarget) error {
	for _, id := range slices.Sorted(maps.Keys(targets)) {
		target := targets[id]
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

	return nil
}

func validateDirectPathOwnership(targets map[string]ResolvedTarget) error {
	directTargets := make([]ResolvedTarget, 0, len(targets))
	for _, id := range slices.Sorted(maps.Keys(targets)) {
		target := targets[id]
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
			normalizedVersionFile, err := normalizedVersionFile(
				"targets."+targetID+".version_files",
				versionFile,
			)
			if err != nil {
				return err
			}

			normalizedVersionFilePath := normalizedVersionFile.Path

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
