package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/history"
	"github.com/monkescience/yeet/internal/provider"
)

var (
	errUnconfiguredReleaseBranch = errors.New("branch is not configured for releases")
	errUnknownReleaseChannel     = errors.New("unknown release channel")
)

type Options struct {
	DryRun            bool
	Provider          *string
	RepositoryRemote  *string
	RepositoryHost    *string
	RepositoryOwner   *string
	RepositoryRepo    *string
	RepositoryProject *string
	AutoMerge         *bool
	AutoMergeForce    *bool
	AutoMergeMethod   *string
	Channel           *string
	Targets           []string
}

func Run(ctx context.Context, configPath string, options Options) (*Result, error) {
	result, resolvedConfigPath, err := rawRun(ctx, configPath, options)
	if err != nil {
		return nil, classifyFailure(resolvedConfigPath, err)
	}

	return result, nil
}

func rawRun(ctx context.Context, configPath string, options Options) (*Result, string, error) {
	cfg, resolvedConfigPath, err := prepareWithPath(ctx, configPath, options)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	releaseBranch, err := releaseBranchForConfig(cfg)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	p, resolvedProvider, err := provider.Open(ctx, cfg, releaseBranch, repositoryOverrides(options))
	if err != nil {
		if errors.Is(err, config.ErrInvalidConfig) {
			return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
		}

		return nil, resolvedConfigPath, fmt.Errorf("provider setup failed: %w", err)
	}

	core, err := newReleaseCore(ctx, cfg, p, releaseBranch)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	selection, err := selectTargets(core, options.Targets)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	historySource, err := history.Open(ctx, p, cfg.Branch, ".")
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("validate checkout: %w", err)
	}

	r, err := newReleaser(core, historySource, p, p, p)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	result, err := r.releaseTargets(ctx, options.DryRun, selection)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	result.Provider = resolvedProvider

	return result, resolvedConfigPath, nil
}

func repositoryOverrides(options Options) provider.RepositoryOverrides {
	return provider.RepositoryOverrides{
		Provider: options.Provider,
		Remote:   options.RepositoryRemote,
		Host:     options.RepositoryHost,
		Owner:    options.RepositoryOwner,
		Repo:     options.RepositoryRepo,
		Project:  options.RepositoryProject,
	}
}

func prepare(ctx context.Context, options Options) (*config.Config, error) {
	cfg, _, err := prepareWithPath(ctx, config.DefaultFile, options)

	return cfg, err
}

func prepareWithPath(
	ctx context.Context,
	configPath string,
	options Options,
) (*config.Config, string, error) {
	cfg, resolvedConfigPath, err := config.LoadResolved(ctx, configPath)
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("load release config: %w", err)
	}

	logRun(ctx, resolvedConfigPath, options)

	applyBehaviorOptions(cfg, options)

	err = cfg.Validate()
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	err = validateReleaseBranchTemplates(cfg)
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	currentBranch, branchErr := currentGitBranch(ctx)
	if branchErr != nil {
		if !options.DryRun && (errors.Is(branchErr, errCINonBranchRef) || len(cfg.Release.Channels) > 0) {
			return nil, resolvedConfigPath, fmt.Errorf("resolve current branch: %w", branchErr)
		}

		slog.DebugContext(ctx, "could not determine current branch (using configured default branch)",
			slog.Any("error", branchErr),
		)
	}

	err = resolveMode(cfg, currentBranch, options)
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	return cfg, resolvedConfigPath, nil
}

func resolveMode(cfg *config.Config, currentBranch string, options Options) error {
	currentBranch = strings.TrimSpace(currentBranch)

	if options.Channel != nil {
		return resolveExplicitChannel(cfg, currentBranch, options)
	}

	if currentBranch == cfg.Branch {
		cfg.ActiveChannel = ""

		return nil
	}

	if currentBranch == "" && len(cfg.Release.Channels) == 0 {
		cfg.ActiveChannel = ""

		return nil
	}

	for channelName, channel := range cfg.Release.Channels {
		if currentBranch != strings.TrimSpace(channel.Branch) {
			continue
		}

		cfg.Branch = strings.TrimSpace(channel.Branch)
		cfg.ActiveChannel = strings.TrimSpace(channelName)

		return nil
	}

	if options.DryRun {
		cfg.ActiveChannel = ""

		return nil
	}

	return fmt.Errorf(
		"%w: %q. Configure it as branch or release.channels.<name>.branch, or run --dry-run",
		errUnconfiguredReleaseBranch,
		currentBranch,
	)
}

func resolveExplicitChannel(cfg *config.Config, currentBranch string, options Options) error {
	channelName := strings.TrimSpace(*options.Channel)

	channel, exists := cfg.Release.Channels[channelName]
	if !exists {
		return fmt.Errorf("%w: %q", errUnknownReleaseChannel, channelName)
	}

	channelBranch := strings.TrimSpace(channel.Branch)
	if !options.DryRun && strings.TrimSpace(currentBranch) != channelBranch {
		return fmt.Errorf(
			"%w: channel %q must run on branch %q, got %q",
			errUnconfiguredReleaseBranch,
			channelName,
			channelBranch,
			currentBranch,
		)
	}

	cfg.Branch = channelBranch
	cfg.ActiveChannel = channelName

	return nil
}

func applyBehaviorOptions(cfg *config.Config, options Options) {
	if options.AutoMerge != nil {
		cfg.Release.AutoMerge = *options.AutoMerge
		if !*options.AutoMerge {
			cfg.Release.AutoMergeForce = false
		}
	}

	if options.AutoMergeForce != nil {
		cfg.Release.AutoMergeForce = *options.AutoMergeForce
	}

	if options.AutoMergeMethod != nil {
		cfg.Release.AutoMergeMethod = config.AutoMergeMethod(*options.AutoMergeMethod)
	}

	if cfg.Release.AutoMergeForce {
		cfg.Release.AutoMerge = true
	}
}

func logRun(ctx context.Context, configPath string, options Options) {
	channel := ""
	if options.Channel != nil {
		channel = *options.Channel
	}

	slog.DebugContext(ctx, "running release command",
		slog.String("config", configPath),
		slog.Bool("dry_run", options.DryRun),
		slog.Bool("provider_override_set", options.Provider != nil),
		slog.Bool("remote_override_set", options.RepositoryRemote != nil),
		slog.Bool("host_override_set", options.RepositoryHost != nil),
		slog.Bool("owner_override_set", options.RepositoryOwner != nil),
		slog.Bool("repo_override_set", options.RepositoryRepo != nil),
		slog.Bool("project_override_set", options.RepositoryProject != nil),
		slog.String("channel", channel),
		slog.Bool("channel_set", options.Channel != nil),
		slog.Any("targets", options.Targets),
	)
}
