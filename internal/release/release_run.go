package release

import (
	"fmt"
	"path"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

type releaseRun struct {
	baseBranch    string
	releaseBranch string
	channelName   string
	prerelease    string
	autoMerge     autoMergeSettings
	changelogFile string
}

type autoMergeSettings struct {
	enabled bool
	force   bool
	method  config.AutoMergeMethod
}

func resolveRun(cfg *config.Config, currentBranch string, options Options) (releaseRun, error) {
	run := releaseRun{
		baseBranch: strings.TrimSpace(cfg.Branch),
		autoMerge: autoMergeSettings{
			enabled: cfg.Release.AutoMerge,
			force:   cfg.Release.AutoMergeForce,
			method:  cfg.Release.AutoMergeMethod,
		},
	}

	applyAutoMergeOptions(&run.autoMerge, options)

	err := config.ValidateAutoMergeMethod(run.autoMerge.method)
	if err != nil {
		//nolint:wrapcheck // Config owns this user-facing validation error.
		return releaseRun{}, err
	}

	err = resolveRunChannel(&run, cfg, strings.TrimSpace(currentBranch), options)
	if err != nil {
		return releaseRun{}, err
	}

	tmpl, err := newReleaseBranchTemplate(cfg.Release.BranchTemplate)
	if err != nil {
		return releaseRun{}, err
	}

	run.releaseBranch, err = renderReleaseBranch(tmpl, run.baseBranch, run.channelName, "")
	if err != nil {
		return releaseRun{}, err
	}

	return run, nil
}

func applyAutoMergeOptions(settings *autoMergeSettings, options Options) {
	if options.AutoMerge != nil {
		settings.enabled = *options.AutoMerge
		if !*options.AutoMerge {
			settings.force = false
		}
	}

	if options.AutoMergeForce != nil {
		settings.force = *options.AutoMergeForce
	}

	if options.AutoMergeMethod != nil {
		settings.method = config.AutoMergeMethod(*options.AutoMergeMethod)
	}

	if settings.force {
		settings.enabled = true
	}
}

func resolveRunChannel(
	run *releaseRun,
	cfg *config.Config,
	currentBranch string,
	options Options,
) error {
	if options.Channel != nil {
		return resolveExplicitRunChannel(run, cfg, currentBranch, options)
	}

	if currentBranch == run.baseBranch || currentBranch == "" && len(cfg.Release.Channels) == 0 {
		return nil
	}

	for channelName, channel := range cfg.Release.Channels {
		if currentBranch != strings.TrimSpace(channel.Branch) {
			continue
		}

		return selectRunChannel(run, channelName, channel)
	}

	if options.DryRun {
		return nil
	}

	return fmt.Errorf(
		"%w: %q. Configure it as branch or release.channels.<name>.branch, or run --dry-run",
		errUnconfiguredReleaseBranch,
		currentBranch,
	)
}

func resolveExplicitRunChannel(
	run *releaseRun,
	cfg *config.Config,
	currentBranch string,
	options Options,
) error {
	channelName := strings.TrimSpace(*options.Channel)

	channel, exists := cfg.Release.Channels[channelName]
	if !exists {
		return fmt.Errorf("%w: %q", errUnknownReleaseChannel, channelName)
	}

	channelBranch := strings.TrimSpace(channel.Branch)
	if !options.DryRun && currentBranch != channelBranch {
		return fmt.Errorf(
			"%w: channel %q must run on branch %q, got %q",
			errUnconfiguredReleaseBranch,
			channelName,
			channelBranch,
			currentBranch,
		)
	}

	return selectRunChannel(run, channelName, channel)
}

func selectRunChannel(run *releaseRun, name string, channel config.ReleaseChannelConfig) error {
	run.baseBranch = strings.TrimSpace(channel.Branch)
	run.channelName = strings.TrimSpace(name)
	run.prerelease = strings.TrimSpace(channel.Prerelease)

	if strings.TrimSpace(channel.ChangelogFile) == "" {
		return nil
	}

	changelogFile, err := config.NormalizeRepoFilePath(channel.ChangelogFile)
	if err != nil {
		return fmt.Errorf(
			"%w: release.channels.%s.changelog_file %v",
			config.ErrInvalidConfig,
			run.channelName,
			err,
		)
	}

	run.changelogFile = changelogFile

	return nil
}

func (r releaseRun) isPrerelease() bool {
	return r.prerelease != ""
}

func (r releaseRun) withChannelChangelogs(
	targets map[string]config.ResolvedTarget,
) (map[string]config.ResolvedTarget, error) {
	if r.channelName == "" {
		return targets, nil
	}

	channelTargets := make(map[string]config.ResolvedTarget, len(targets))
	for targetID, target := range targets {
		if !versionStrategyForResolvedTarget(target).strategy.SupportsPrerelease() {
			return nil, fmt.Errorf(
				"%w: prerelease channel %q supports semver targets only. Target %q uses %q",
				config.ErrInvalidConfig,
				r.channelName,
				targetID,
				target.Versioning,
			)
		}

		if r.changelogFile != "" && len(targets) == 1 {
			target.Changelog.File = r.changelogFile
		} else {
			target.Changelog.File = channelChangelogFile(target.Changelog.File, r.channelName)
		}

		channelTargets[targetID] = target
	}

	return channelTargets, nil
}

func channelChangelogFile(changelogFile string, channelName string) string {
	dir, file := path.Split(changelogFile)
	ext := path.Ext(file)

	base := strings.TrimSuffix(file, ext)
	if base == "" {
		return changelogFile
	}

	return dir + base + "." + channelName + ext
}
