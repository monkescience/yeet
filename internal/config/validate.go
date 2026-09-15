package config

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/monkescience/yeet/internal/version"
)

func (c *Config) Validate() error {
	if strings.TrimSpace(c.Branch) == "" {
		return Invalidf("branch must not be blank")
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
		err = c.ReleaseLayout().validateFileOwnership(resolvedTargets)
	}

	if err != nil {
		return err
	}

	return nil
}

// TimeLocation resolves Timezone using the same rules enforced by Validate.
func (c *Config) TimeLocation() (*time.Location, error) {
	if strings.TrimSpace(c.Timezone) == "" {
		return nil, Invalidf("timezone must not be blank")
	}

	if strings.TrimSpace(c.Timezone) != c.Timezone {
		return nil, Invalidf("timezone must not contain surrounding whitespace")
	}

	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, Invalidf("timezone %q is not a valid IANA location", c.Timezone)
	}

	return location, nil
}

func validateNetworkConfig(network NetworkConfig) error {
	if network.RequestTimeout <= 0 {
		return Invalidf("network.request_timeout must be greater than zero")
	}

	if network.Retry.MaxAttempts < 1 {
		return Invalidf("network.retry.max_attempts must be at least 1")
	}

	if network.Retry.MinBackoff <= 0 {
		return Invalidf("network.retry.min_backoff must be greater than zero")
	}

	if network.Retry.MaxBackoff <= 0 {
		return Invalidf("network.retry.max_backoff must be greater than zero")
	}

	if network.Retry.MinBackoff > network.Retry.MaxBackoff {
		return Invalidf("network.retry.min_backoff must not exceed network.retry.max_backoff")
	}

	return nil
}

func validatePreMajorCalVer(targetID string, versioning VersioningStrategy, target Target) error {
	if versioning != VersioningCalVer {
		return nil
	}

	if target.PreMajorBreakingBumpsMinor != nil {
		return Invalidf("targets.%s.pre_major_breaking_bumps_minor has no effect with calver versioning", targetID)
	}

	if target.PreMajorFeaturesBumpPatch != nil {
		return Invalidf("targets.%s.pre_major_features_bump_patch has no effect with calver versioning", targetID)
	}

	return nil
}

func validateCalVerConfig(path string, calver CalVerConfig) error {
	err := version.ValidateCalVerFormat(calver.Format)
	if err != nil {
		return InvalidWithCausef(err, "%s: %s", path, validationReason(err))
	}

	return nil
}

func validateVersionFile(configPath string, versionFile VersionFile) error {
	_, err := normalizedVersionFile(configPath, versionFile)

	return err
}

func normalizedVersionFile(configPath string, versionFile VersionFile) (VersionFile, error) {
	if strings.TrimSpace(versionFile.Path) == "" {
		return VersionFile{}, Invalidf("%s must not contain empty paths", configPath)
	}

	normalizedPath, err := NormalizeRepoFilePath(versionFile.Path)
	if err != nil {
		return VersionFile{}, Invalidf("%s entry %q %v", configPath,
			versionFile.Path,
			err,
		)
	}

	versionFile.Path = normalizedPath

	return versionFile, nil
}

func normalizedChangelogFile(configPath string, rawPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return "", Invalidf("%s must not be empty", configPath)
	}

	normalizedPath, err := NormalizeRepoFilePath(rawPath)
	if err != nil {
		return "", Invalidf("%s %v", configPath, err)
	}

	return normalizedPath, nil
}

func validateReferencesConfig(path string, references ReferencesConfig) error {
	for i, pattern := range references.Patterns {
		if strings.TrimSpace(pattern.Pattern) == "" {
			return Invalidf("%s.patterns[%d].pattern must not be empty", path, i)
		}

		_, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return InvalidWithCausef(err,
				"%s.patterns[%d].pattern %q is not a valid regular expression: %s",
				path,
				i,
				pattern.Pattern,
				validationReason(err),
			)
		}
	}

	keys := slices.Sorted(maps.Keys(references.Footers))
	for i, key := range keys {
		for _, previous := range keys[:i] {
			if strings.EqualFold(previous, key) {
				return Invalidf("%s.footers keys %q and %q differ only by case", path, previous, key)
			}
		}
	}

	return nil
}

func validateBumpTypes(bt BumpTypesConfig) error {
	seen := make(map[string]string, len(bt.Minor)+len(bt.Patch))

	for _, t := range bt.Minor {
		if strings.TrimSpace(t) == "" {
			return Invalidf("bump_types.minor must not contain empty strings")
		}

		seen[t] = "minor"
	}

	for _, t := range bt.Patch {
		if strings.TrimSpace(t) == "" {
			return Invalidf("bump_types.patch must not contain empty strings")
		}

		if level, exists := seen[t]; exists {
			return Invalidf("bump_types: type %q appears in both %s and patch", t, level)
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

	err = ValidateAutoMergeMode(release.AutoMergeMode)
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

func ValidateAutoMergeMode(mode AutoMergeMode) error {
	switch mode {
	case AutoMergeModeProvider, AutoMergeModeDirect:
	default:
		return Invalidf("release.auto_merge_mode must be \"provider\" or \"direct\", got %q", mode)
	}

	return nil
}

// ValidateAutoMergeMethod reports whether method is supported by release providers.
func ValidateAutoMergeMethod(method AutoMergeMethod) error {
	switch method {
	case AutoMergeMethodAuto, AutoMergeMethodSquash, AutoMergeMethodRebase, AutoMergeMethodMerge:
	default:
		return Invalidf("release.auto_merge_method must be \"auto\", \"squash\", \"rebase\", or \"merge\", got %q", method)
	}

	return nil
}

func validateReleaseMergePolling(polling ReleaseMergePollingConfig) error {
	if polling.InitialInterval <= 0 {
		return Invalidf("release.merge_polling.initial_interval must be greater than zero")
	}

	if polling.MaxInterval <= 0 {
		return Invalidf("release.merge_polling.max_interval must be greater than zero")
	}

	if polling.Timeout <= 0 {
		return Invalidf("release.merge_polling.timeout must be greater than zero")
	}

	if polling.InitialInterval > polling.MaxInterval {
		return Invalidf("release.merge_polling.initial_interval must not exceed release.merge_polling.max_interval")
	}

	if polling.MaxInterval > polling.Timeout {
		return Invalidf("release.merge_polling.max_interval must not exceed release.merge_polling.timeout")
	}

	return nil
}

func validateLifecycleLabelName(path, name string) error {
	if strings.TrimSpace(name) == "" {
		return Invalidf("%s must not be blank", path)
	}

	if strings.Contains(name, ",") {
		return Invalidf("%s %q must not contain a comma", path, name)
	}

	if strings.EqualFold(name, "any") || strings.EqualFold(name, "none") {
		return Invalidf("%s %q is a reserved label filter value", path,
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
		return Invalidf("release.labels.pending and release.labels.tagged must differ")
	}

	if labels.Yeet {
		for _, lifecycle := range lifecycle {
			if strings.EqualFold(lifecycle.name, "yeet") {
				return Invalidf("%s must differ from the managed yeet label", lifecycle.path)
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
			return Invalidf("release.labels.extra must not contain blank labels")
		}

		if strings.Contains(extra, ",") {
			return Invalidf("release.labels.extra entry %q must not contain a comma", extra)
		}

		for _, existing := range seen {
			if strings.EqualFold(extra, existing.name) {
				return Invalidf("release.labels.extra entry %q duplicates %s", extra,
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
			return Invalidf("release.reviewers must not contain empty strings")
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

		return Invalidf("release.channels.%s.branch %q duplicates stable branch", strings.TrimSpace(name),
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
			return Invalidf("release.channels.%s.branch must not be empty", channelName)
		}

		if otherChannel, exists := seenBranches[branch]; exists {
			return Invalidf("release.channels.%s.branch %q duplicates release.channels.%s.branch", channelName,
				branch,
				otherChannel,
			)
		}

		seenBranches[branch] = channelName

		prerelease := strings.TrimSpace(channel.Prerelease)
		if prerelease == "" {
			return Invalidf("release.channels.%s.prerelease must not be empty", channelName)
		}

		err = version.ValidatePrereleaseIdentifier(prerelease)
		if err != nil {
			return InvalidWithCausef(
				err,
				"release.channels.%s.prerelease %q must be a valid semver prerelease identifier: %s",
				channelName,
				prerelease,
				validationReason(err),
			)
		}

		if otherChannel, exists := seenPrereleaseIDs[prerelease]; exists {
			return Invalidf("release.channels.%s.prerelease %q duplicates release.channels.%s.prerelease", channelName,
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
		return "", Invalidf("release.channels keys must not be empty")
	}

	if strings.EqualFold(channelName, "stable") {
		return "", Invalidf("release.channels.%s must not use reserved name stable", channelName)
	}

	return channelName, nil
}

func validateReleaseChannelChangelogFile(channelName, changelogFile string) error {
	if changelogFile == "" {
		return nil
	}

	if strings.TrimSpace(changelogFile) == "" {
		return Invalidf("release.channels.%s.changelog_file must not be blank", channelName)
	}

	_, err := NormalizeRepoFilePath(changelogFile)
	if err != nil {
		return Invalidf("release.channels.%s.changelog_file %v", channelName,
			err,
		)
	}

	return nil
}
