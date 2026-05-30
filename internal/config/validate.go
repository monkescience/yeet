package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/monkescience/yeet/internal/version"
)

//nolint:funlen // Top-level validation deliberately enumerates every field check.
func (c *Config) Validate(ctx context.Context) error {
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

	err := validateBumpTypes(c.BumpTypes)
	if err != nil {
		return err
	}

	err = validateRepositoryConfig(c.Provider, c.Repository)
	if err != nil {
		return err
	}

	if c.Changelog.File == "" {
		return fmt.Errorf("%w: changelog.file must not be empty", ErrInvalidConfig)
	}

	if len(c.Changelog.Include) == 0 {
		return fmt.Errorf("%w: changelog.include must not be empty", ErrInvalidConfig)
	}

	err = validateCalVerConfig("calver.format", c.CalVer)
	if err != nil {
		return err
	}

	for _, versionFile := range c.VersionFiles {
		err = validateVersionFile("version_files", versionFile)
		if err != nil {
			return err
		}
	}

	err = validateReleaseConfig(c.Release)
	if err != nil {
		return err
	}

	err = validateReleaseChannelBranches(c.Branch, c.Release.Channels)
	if err != nil {
		return err
	}

	_, err = c.ResolvedTargets(ctx)
	if err != nil {
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

	err := validateReleaseChannels(release.Channels)
	if err != nil {
		return err
	}

	return nil
}

func validateReleaseChannelBranches(stableBranch string, channels map[string]ReleaseChannelConfig) error {
	stableBranch = strings.TrimSpace(stableBranch)
	for name, channel := range channels {
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

	for name, channel := range channels {
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
