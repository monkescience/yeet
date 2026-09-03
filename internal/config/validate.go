package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/version"
)

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Branch) == "" {
		return fmt.Errorf("%w: branch must not be blank", ErrInvalidConfig)
	}

	_, err := c.TimeLocation()
	if err != nil {
		return err
	}

	err = validateProvider(c.Provider)
	if err != nil {
		return err
	}

	err = validateBumpTypes(c.BumpTypes)
	if err != nil {
		return err
	}

	err = validateRepositoryConfig(c.Provider, c.Repository)
	if err != nil {
		return err
	}

	err = validateNetworkConfig(c.Network)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.Changelog.File) != "" {
		_, err = normalizedChangelogFile("changelog.file", c.Changelog.File)
		if err != nil {
			return err
		}
	}

	err = validateReferencesConfig("changelog.references", c.Changelog.References)
	if err != nil {
		return err
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

	return c.validateReleaseAndTargets()
}

func (c *Config) validateReleaseAndTargets() error {
	err := validateReleaseConfig(c.Release)
	if err != nil {
		return err
	}

	err = validateReleaseChannelBranches(c.Branch, c.Release.Channels)
	if err != nil {
		return err
	}

	resolvedTargets, err := c.resolveTargets()
	if err != nil {
		return err
	}

	err = validateReleaseGroups(c.Release, resolvedTargets)
	if err != nil {
		return err
	}

	switch c.Release.PullRequestMode {
	case PullRequestModeCombined:
		err = validateTargetVersionFileOwnership(c.Targets)
	case PullRequestModeIndependent:
		err = validateIndependentReleaseUnitFileOwnership(c.Release, resolvedTargets)
	}

	if err != nil {
		return err
	}

	return nil
}

func validateIndependentReleaseUnitFileOwnership(
	release ReleaseConfig,
	targets map[string]ResolvedTarget,
) error {
	unitByTarget := make(map[string]string, len(targets))

	for groupName, group := range release.Groups {
		unitID := "group:" + strings.TrimSpace(groupName)
		for _, targetID := range group.Targets {
			unitByTarget[strings.TrimSpace(targetID)] = unitID
		}
	}

	owners := make(map[string]string)

	for _, targetID := range slices.Sorted(maps.Keys(targets)) {
		target := targets[targetID]
		unitID, grouped := unitByTarget[targetID]

		if !grouped {
			unitID = "target:" + targetID
		}

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

// TimeLocation resolves Timezone using the same rules enforced by Validate.
func (c *Config) TimeLocation() (*time.Location, error) {
	if strings.TrimSpace(c.Timezone) == "" {
		return nil, fmt.Errorf("%w: timezone must not be blank", ErrInvalidConfig)
	}

	if strings.TrimSpace(c.Timezone) != c.Timezone {
		return nil, fmt.Errorf("%w: timezone must not contain surrounding whitespace", ErrInvalidConfig)
	}

	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: timezone %q is not a valid IANA location", ErrInvalidConfig, c.Timezone)
	}

	return location, nil
}

func validateNetworkConfig(network NetworkConfig) error {
	if network.RequestTimeout <= 0 {
		return fmt.Errorf("%w: network.request_timeout must be greater than zero", ErrInvalidConfig)
	}

	if network.Retry.MaxAttempts < 1 {
		return fmt.Errorf("%w: network.retry.max_attempts must be at least 1", ErrInvalidConfig)
	}

	if network.Retry.MinBackoff <= 0 {
		return fmt.Errorf("%w: network.retry.min_backoff must be greater than zero", ErrInvalidConfig)
	}

	if network.Retry.MaxBackoff <= 0 {
		return fmt.Errorf("%w: network.retry.max_backoff must be greater than zero", ErrInvalidConfig)
	}

	if network.Retry.MinBackoff > network.Retry.MaxBackoff {
		return fmt.Errorf(
			"%w: network.retry.min_backoff must not exceed network.retry.max_backoff",
			ErrInvalidConfig,
		)
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
	_, err := normalizedVersionFile(configPath, versionFile)

	return err
}

func normalizedVersionFile(configPath string, versionFile VersionFile) (VersionFile, error) {
	if strings.TrimSpace(versionFile.Path) == "" {
		return VersionFile{}, fmt.Errorf("%w: %s must not contain empty paths", ErrInvalidConfig, configPath)
	}

	normalizedPath, err := NormalizeRepoFilePath(versionFile.Path)
	if err != nil {
		return VersionFile{}, fmt.Errorf(
			"%w: %s entry %q %v",
			ErrInvalidConfig,
			configPath,
			versionFile.Path,
			err,
		)
	}

	versionFile.Path = normalizedPath

	return versionFile, nil
}

func normalizedChangelogFile(configPath string, rawPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return "", fmt.Errorf("%w: %s must not be empty", ErrInvalidConfig, configPath)
	}

	normalizedPath, err := NormalizeRepoFilePath(rawPath)
	if err != nil {
		return "", fmt.Errorf("%w: %s %v", ErrInvalidConfig, configPath, err)
	}

	return normalizedPath, nil
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
	err := validateReleaseMergePolling(release.MergePolling)
	if err != nil {
		return err
	}

	err = ValidateAutoMergeMethod(release.AutoMergeMethod)
	if err != nil {
		return err
	}

	err = validateReleaseLabels(release.Labels)
	if err != nil {
		return err
	}

	err = validateReleaseReviewers(release.Reviewers)
	if err != nil {
		return err
	}

	err = validateReleaseChannels(release.Channels)
	if err != nil {
		return err
	}

	return nil
}

// ValidateAutoMergeMethod reports whether method is supported by release providers.
func ValidateAutoMergeMethod(method AutoMergeMethod) error {
	switch method {
	case AutoMergeMethodAuto, AutoMergeMethodSquash, AutoMergeMethodRebase, AutoMergeMethodMerge:
	default:
		return fmt.Errorf(
			"%w: release.auto_merge_method must be \"auto\", \"squash\", \"rebase\", or \"merge\", got %q",
			ErrInvalidConfig,
			method,
		)
	}

	return nil
}

func validateReleaseMergePolling(polling ReleaseMergePollingConfig) error {
	if polling.InitialInterval <= 0 {
		return fmt.Errorf(
			"%w: release.merge_polling.initial_interval must be greater than zero",
			ErrInvalidConfig,
		)
	}

	if polling.MaxInterval <= 0 {
		return fmt.Errorf(
			"%w: release.merge_polling.max_interval must be greater than zero",
			ErrInvalidConfig,
		)
	}

	if polling.Timeout <= 0 {
		return fmt.Errorf("%w: release.merge_polling.timeout must be greater than zero", ErrInvalidConfig)
	}

	if polling.InitialInterval > polling.MaxInterval {
		return fmt.Errorf(
			"%w: release.merge_polling.initial_interval must not exceed release.merge_polling.max_interval",
			ErrInvalidConfig,
		)
	}

	if polling.MaxInterval > polling.Timeout {
		return fmt.Errorf(
			"%w: release.merge_polling.max_interval must not exceed release.merge_polling.timeout",
			ErrInvalidConfig,
		)
	}

	return nil
}

func validateLifecycleLabelName(path, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: %s must not be blank", ErrInvalidConfig, path)
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
		err := validateLifecycleLabelName(label.path, label.name)
		if err != nil {
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
	for _, reviewer := range reviewers {
		if strings.TrimSpace(reviewer) == "" {
			return fmt.Errorf("%w: release.reviewers must not contain empty strings", ErrInvalidConfig)
		}
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

		channelName, err := validateReleaseChannelName(name)
		if err != nil {
			return err
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

		err = version.ValidatePrereleaseIdentifier(prerelease)
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

		err = validateReleaseChannelChangelogFile(channelName, channel.ChangelogFile)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateReleaseChannelName(name string) (string, error) {
	channelName := strings.TrimSpace(name)
	if channelName == "" {
		return "", fmt.Errorf("%w: release.channels keys must not be empty", ErrInvalidConfig)
	}

	if strings.EqualFold(channelName, "stable") {
		return "", fmt.Errorf("%w: release.channels.%s must not use reserved name stable", ErrInvalidConfig, channelName)
	}

	return channelName, nil
}

func validateReleaseChannelChangelogFile(channelName, changelogFile string) error {
	if changelogFile == "" {
		return nil
	}

	if strings.TrimSpace(changelogFile) == "" {
		return fmt.Errorf("%w: release.channels.%s.changelog_file must not be blank", ErrInvalidConfig, channelName)
	}

	_, err := NormalizeRepoFilePath(changelogFile)
	if err != nil {
		return fmt.Errorf(
			"%w: release.channels.%s.changelog_file %v",
			ErrInvalidConfig,
			channelName,
			err,
		)
	}

	return nil
}
