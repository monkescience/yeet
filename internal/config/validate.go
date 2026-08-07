package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/version"
)

func (c *Config) Validate() error {
	if c.Versioning != VersioningSemver && c.Versioning != VersioningCalVer {
		return fmt.Errorf("%w: versioning must be %q or %q, got %q",
			ErrInvalidConfig, VersioningSemver, VersioningCalVer, c.Versioning)
	}

	if c.Branch == "" {
		return fmt.Errorf("%w: branch must not be empty", ErrInvalidConfig)
	}

	if c.Provider != ProviderAuto &&
		c.Provider != ProviderGitHub &&
		c.Provider != ProviderGitLab &&
		c.Provider != ProviderAzureDevOps {
		return fmt.Errorf("%w: provider must be %q, %q, %q, or %q, got %q",
			ErrInvalidConfig, ProviderAuto, ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, c.Provider)
	}

	if err := validateBumpTypes(c.BumpTypes); err != nil {
		return err
	}

	if err := validateRepositoryConfig(c.Provider, c.Repository); err != nil {
		return err
	}

	if c.Changelog.File == "" {
		return fmt.Errorf("%w: changelog.file must not be empty", ErrInvalidConfig)
	}

	if len(c.Changelog.Include) == 0 {
		return fmt.Errorf("%w: changelog.include must not be empty", ErrInvalidConfig)
	}

	if err := validateReferencesConfig("changelog.references", c.Changelog.References); err != nil {
		return err
	}

	if err := validateCalVerConfig("calver.format", c.CalVer); err != nil {
		return err
	}

	for _, versionFile := range c.VersionFiles {
		if err := validateVersionFile("version_files", versionFile); err != nil {
			return err
		}
	}

	if err := validateReleaseConfig(c.Release); err != nil {
		return err
	}

	if err := validateReleaseChannelBranches(c.Branch, c.Release.Channels); err != nil {
		return err
	}

	if _, err := c.resolveTargets(); err != nil {
		return err
	}

	return nil
}

func validatePreMajorCalVer(targetID string, versioning VersioningStrategy, target Target) error {
	if versioning != VersioningCalVer {
		return nil
	}

	if target.PreMajorBreakingBumpsMinor != nil {
		return fmt.Errorf(
			"%w: targets.%s.pre_major_breaking_bumps_minor has no effect with calver versioning",
			ErrInvalidConfig,
			targetID,
		)
	}

	if target.PreMajorFeaturesBumpPatch != nil {
		return fmt.Errorf(
			"%w: targets.%s.pre_major_features_bump_patch has no effect with calver versioning",
			ErrInvalidConfig,
			targetID,
		)
	}

	return nil
}

func validateCalVerConfig(path string, calver CalVerConfig) error {
	err := version.ValidateCalVerFormat(calver.Format)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidConfig, path, err)
	}

	return nil
}

func validateVersionFile(configPath string, versionFile VersionFile) error {
	if strings.TrimSpace(versionFile.Path) == "" {
		return fmt.Errorf("%w: %s must not contain empty paths", ErrInvalidConfig, configPath)
	}

	switch versionFile.Format {
	case "", VersionFileFormatMarkers:
		if strings.TrimSpace(versionFile.JSONPointer) != "" {
			return fmt.Errorf(
				"%w: %s json_pointer requires format %q",
				ErrInvalidConfig,
				configPath,
				VersionFileFormatJSON,
			)
		}
	case VersionFileFormatJSON:
		if strings.TrimSpace(versionFile.JSONPointer) == "" {
			return fmt.Errorf(
				"%w: %s json_pointer is required for format %q",
				ErrInvalidConfig,
				configPath,
				VersionFileFormatJSON,
			)
		}

		err := validateJSONPointerSyntax(versionFile.JSONPointer)
		if err != nil {
			return fmt.Errorf("%w: %s json_pointer: %v", ErrInvalidConfig, configPath, err)
		}
	default:
		return fmt.Errorf(
			"%w: %s format must be %q or %q, got %q",
			ErrInvalidConfig,
			configPath,
			VersionFileFormatMarkers,
			VersionFileFormatJSON,
			versionFile.Format,
		)
	}

	return nil
}

func validateJSONPointerSyntax(pointer string) error {
	if pointer == "" || pointer[0] != '/' {
		return errJSONPointerMustStartWithSlash
	}

	for i := 0; i < len(pointer); i++ {
		if pointer[i] != '~' {
			continue
		}

		if i+1 >= len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1') {
			return errJSONPointerInvalidEscape
		}

		i++
	}

	return nil
}

func validateReferencesConfig(path string, references ReferencesConfig) error {
	for i, pattern := range references.Patterns {
		if strings.TrimSpace(pattern.Pattern) == "" {
			return fmt.Errorf("%w: %s.patterns[%d].pattern must not be empty", ErrInvalidConfig, path, i)
		}

		_, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf(
				"%w: %s.patterns[%d].pattern %q is not a valid regular expression: %v",
				ErrInvalidConfig,
				path,
				i,
				pattern.Pattern,
				err,
			)
		}
	}

	return nil
}

func validateBumpTypes(bt BumpTypesConfig) error {
	seen := make(map[string]string, len(bt.Minor)+len(bt.Patch))

	for _, t := range bt.Minor {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("%w: bump_types.minor must not contain empty strings", ErrInvalidConfig)
		}

		seen[t] = "minor"
	}

	for _, t := range bt.Patch {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("%w: bump_types.patch must not contain empty strings", ErrInvalidConfig)
		}

		if level, exists := seen[t]; exists {
			return fmt.Errorf("%w: bump_types: type %q appears in both %s and patch", ErrInvalidConfig, t, level)
		}
	}

	return nil
}

func validateReleaseConfig(release ReleaseConfig) error {
	if err := validateReleaseLabels(release.Labels); err != nil {
		return err
	}

	if release.AutoMergeMethod != AutoMergeMethodAuto &&
		release.AutoMergeMethod != AutoMergeMethodSquash &&
		release.AutoMergeMethod != AutoMergeMethodRebase &&
		release.AutoMergeMethod != AutoMergeMethodMerge {
		return fmt.Errorf(
			"%w: release.auto_merge_method must be %q, %q, %q, or %q, got %q",
			ErrInvalidConfig,
			AutoMergeMethodAuto,
			AutoMergeMethodSquash,
			AutoMergeMethodRebase,
			AutoMergeMethodMerge,
			release.AutoMergeMethod,
		)
	}

	if release.PRBodyMaxLength < 0 {
		return fmt.Errorf(
			"%w: release.pr_body_max_length must not be negative, got %d",
			ErrInvalidConfig,
			release.PRBodyMaxLength,
		)
	}

	if err := validateReleaseReviewers(release.Reviewers); err != nil {
		return err
	}

	if err := validateReleaseChannels(release.Channels); err != nil {
		return err
	}

	return nil
}

func validateLifecycleLabelName(path, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: %s must not be blank", ErrInvalidConfig, path)
	}

	if name != strings.TrimSpace(name) {
		return fmt.Errorf(
			"%w: %s %q must not have leading or trailing whitespace",
			ErrInvalidConfig,
			path,
			name,
		)
	}

	if strings.Contains(name, ",") {
		return fmt.Errorf("%w: %s %q must not contain a comma", ErrInvalidConfig, path, name)
	}

	if strings.EqualFold(name, "any") || strings.EqualFold(name, "none") {
		return fmt.Errorf(
			"%w: %s %q is a reserved label filter value",
			ErrInvalidConfig,
			path,
			name,
		)
	}

	return nil
}

func validateReleaseLabels(labels ReleaseLabelsConfig) error {
	lifecycle := []struct {
		path string
		name string
	}{
		{path: "release.labels.pending", name: labels.Pending},
		{path: "release.labels.tagged", name: labels.Tagged},
	}

	for _, label := range lifecycle {
		if err := validateLifecycleLabelName(label.path, label.name); err != nil {
			return err
		}
	}

	if strings.EqualFold(labels.Pending, labels.Tagged) {
		return fmt.Errorf(
			"%w: release.labels.pending and release.labels.tagged must differ",
			ErrInvalidConfig,
		)
	}

	if labels.Yeet {
		for _, lifecycle := range lifecycle {
			if strings.EqualFold(lifecycle.name, "yeet") {
				return fmt.Errorf(
					"%w: %s must differ from the managed yeet label",
					ErrInvalidConfig,
					lifecycle.path,
				)
			}
		}
	}

	return validateReleaseExtraLabels(labels)
}

func validateReleaseExtraLabels(labels ReleaseLabelsConfig) error {
	seen := []struct {
		name string
		path string
	}{
		{name: labels.Pending, path: "release.labels.pending"},
		{name: labels.Tagged, path: "release.labels.tagged"},
	}

	if labels.Yeet {
		seen = append(seen, struct {
			name string
			path string
		}{name: "yeet", path: "the managed yeet label"})
	}

	for _, extra := range labels.Extra {
		if strings.TrimSpace(extra) == "" {
			return fmt.Errorf("%w: release.labels.extra must not contain blank labels", ErrInvalidConfig)
		}

		if extra != strings.TrimSpace(extra) {
			return fmt.Errorf(
				"%w: release.labels.extra entry %q must not have leading or trailing whitespace",
				ErrInvalidConfig,
				extra,
			)
		}

		if strings.Contains(extra, ",") {
			return fmt.Errorf(
				"%w: release.labels.extra entry %q must not contain a comma",
				ErrInvalidConfig,
				extra,
			)
		}

		for _, existing := range seen {
			if strings.EqualFold(extra, existing.name) {
				return fmt.Errorf(
					"%w: release.labels.extra entry %q duplicates %s",
					ErrInvalidConfig,
					extra,
					existing.path,
				)
			}
		}

		seen = append(seen, struct {
			name string
			path string
		}{name: extra, path: "release.labels.extra"})
	}

	return nil
}

func validateReleaseReviewers(reviewers []string) error {
	seen := make(map[string]struct{}, len(reviewers))

	for _, reviewer := range reviewers {
		if strings.TrimSpace(reviewer) == "" {
			return fmt.Errorf("%w: release.reviewers must not contain empty strings", ErrInvalidConfig)
		}

		if reviewer != strings.TrimSpace(reviewer) {
			return fmt.Errorf(
				"%w: release.reviewers entry %q must not have leading or trailing whitespace",
				ErrInvalidConfig,
				reviewer,
			)
		}

		if _, exists := seen[reviewer]; exists {
			return fmt.Errorf("%w: release.reviewers contains duplicate %q", ErrInvalidConfig, reviewer)
		}

		seen[reviewer] = struct{}{}
	}

	return nil
}

func validateReleaseChannelBranches(stableBranch string, channels map[string]ReleaseChannelConfig) error {
	stableBranch = strings.TrimSpace(stableBranch)

	for _, name := range slices.Sorted(maps.Keys(channels)) {
		channel := channels[name]

		branch := strings.TrimSpace(channel.Branch)
		if branch == "" || stableBranch == "" || branch != stableBranch {
			continue
		}

		return fmt.Errorf(
			"%w: release.channels.%s.branch %q duplicates stable branch",
			ErrInvalidConfig,
			strings.TrimSpace(name),
			branch,
		)
	}

	return nil
}

func validateReleaseChannels(channels map[string]ReleaseChannelConfig) error {
	seenBranches := make(map[string]string, len(channels))
	seenPrereleaseIDs := make(map[string]string, len(channels))

	for _, name := range slices.Sorted(maps.Keys(channels)) {
		channel := channels[name]

		channelName := strings.TrimSpace(name)
		if channelName == "" {
			return fmt.Errorf("%w: release.channels keys must not be empty", ErrInvalidConfig)
		}

		if strings.EqualFold(channelName, "stable") {
			return fmt.Errorf("%w: release.channels.%s must not use reserved name stable", ErrInvalidConfig, channelName)
		}

		branch := strings.TrimSpace(channel.Branch)
		if branch == "" {
			return fmt.Errorf("%w: release.channels.%s.branch must not be empty", ErrInvalidConfig, channelName)
		}

		if otherChannel, exists := seenBranches[branch]; exists {
			return fmt.Errorf(
				"%w: release.channels.%s.branch %q duplicates release.channels.%s.branch",
				ErrInvalidConfig,
				channelName,
				branch,
				otherChannel,
			)
		}

		seenBranches[branch] = channelName

		prerelease := strings.TrimSpace(channel.Prerelease)
		if prerelease == "" {
			return fmt.Errorf("%w: release.channels.%s.prerelease must not be empty", ErrInvalidConfig, channelName)
		}

		err := validatePrereleaseIdentifier(prerelease)
		if err != nil {
			return fmt.Errorf("%w: release.channels.%s.prerelease: %v", ErrInvalidConfig, channelName, err)
		}

		if otherChannel, exists := seenPrereleaseIDs[prerelease]; exists {
			return fmt.Errorf(
				"%w: release.channels.%s.prerelease %q duplicates release.channels.%s.prerelease",
				ErrInvalidConfig,
				channelName,
				prerelease,
				otherChannel,
			)
		}

		seenPrereleaseIDs[prerelease] = channelName

		if channel.ChangelogFile != "" && strings.TrimSpace(channel.ChangelogFile) == "" {
			return fmt.Errorf("%w: release.channels.%s.changelog_file must not be blank", ErrInvalidConfig, channelName)
		}
	}

	return nil
}

func validatePrereleaseIdentifier(identifier string) error {
	_, err := semver.StrictNewVersion("1.0.0-" + identifier)
	if err != nil {
		return fmt.Errorf("invalid semver prerelease identifier %q: %w", identifier, err)
	}

	return nil
}
