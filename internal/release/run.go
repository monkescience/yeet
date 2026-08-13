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

	p, err := provider.Open(ctx, cfg, releaseBranch)
	if err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("provider setup failed: %w", err)
	}

	core, err := newReleaseCore(ctx, cfg, p)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	if _, err := selectTargets(core, options.Targets); err != nil {
		return nil, resolvedConfigPath, err
	}

	historySource := history.New(p, cfg.Branch, ".")
	if err := historySource.Validate(ctx); err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("validate checkout: %w", err)
	}

	r, err := newReleaser(core, dependencies{
		metadata:  p,
		prs:       p,
		files:     p,
		publisher: p,
	}, historySource)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	result, err := r.releaseTargets(ctx, options.DryRun, options.Targets)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	return result, resolvedConfigPath, nil
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

	if err := applyOptions(cfg, options); err != nil {
		return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	if err := cfg.Validate(); err != nil {
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

	if err := resolveMode(cfg, currentBranch, options); err != nil {
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

func applyOptions(cfg *config.Config, options Options) error {
	if err := applyRepositoryOptions(cfg, options); err != nil {
		return err
	}

	applyBehaviorOptions(cfg, options)

	return nil
}

func applyRepositoryOptions(cfg *config.Config, options Options) error {
	previousProvider := cfg.Provider

	if options.Provider != nil {
		cfg.Provider = config.ProviderType(*options.Provider)
	}

	if options.RepositoryRemote != nil {
		cfg.Repository.Remote = *options.RepositoryRemote
	}

	hasRepoFieldOverride := options.RepositoryHost != nil ||
		options.RepositoryOwner != nil ||
		options.RepositoryRepo != nil ||
		options.RepositoryProject != nil

	if cfg.Provider == config.ProviderAuto {
		if hasRepoFieldOverride {
			return fmt.Errorf(
				"%w: repository field flags require an explicit --provider (auto cannot route them)",
				config.ErrInvalidConfig,
			)
		}

		cfg.Repository.GitHub = nil
		cfg.Repository.GitLab = nil
		cfg.Repository.AzureDevOps = nil

		return nil
	}

	if providerChanged(previousProvider, cfg.Provider) {
		cfg.Repository.GitHub = nil
		cfg.Repository.GitLab = nil
		cfg.Repository.AzureDevOps = nil
	}

	switch cfg.Provider {
	case config.ProviderGitHub:
		return applyGitHubOverrides(&cfg.Repository, options)
	case config.ProviderGitLab:
		return applyGitLabOverrides(&cfg.Repository, options)
	case config.ProviderAzureDevOps:
		return applyAzureDevOpsOverrides(&cfg.Repository, options)
	case config.ProviderAuto:
	}

	return nil
}

func applyGitHubOverrides(repository *config.RepositoryConfig, options Options) error {
	if repository.GitHub == nil {
		repository.GitHub = &config.GitHubRepositoryConfig{}
	}

	github := repository.GitHub

	if options.RepositoryHost != nil {
		github.Host = *options.RepositoryHost
	}

	if options.RepositoryOwner != nil {
		github.Owner = *options.RepositoryOwner
	}

	if options.RepositoryRepo != nil {
		github.Repo = *options.RepositoryRepo
	}

	if options.RepositoryProject != nil {
		github.Project = *options.RepositoryProject

		if options.RepositoryOwner == nil {
			github.Owner = ""
		}

		if options.RepositoryRepo == nil {
			github.Repo = ""
		}
	}

	if options.RepositoryProject == nil &&
		(options.RepositoryOwner != nil || options.RepositoryRepo != nil) &&
		strings.TrimSpace(github.Owner) != "" &&
		strings.TrimSpace(github.Repo) != "" {
		github.Project = ""
	}

	return nil
}

func applyGitLabOverrides(repository *config.RepositoryConfig, options Options) error {
	if options.RepositoryOwner != nil || options.RepositoryRepo != nil {
		return fmt.Errorf(
			"%w: --owner/--repo are not valid for provider gitlab. Use --project",
			config.ErrInvalidConfig,
		)
	}

	if repository.GitLab == nil {
		repository.GitLab = &config.GitLabRepositoryConfig{}
	}

	gitlab := repository.GitLab

	if options.RepositoryHost != nil {
		gitlab.Host = *options.RepositoryHost
	}

	if options.RepositoryProject != nil {
		gitlab.Project = *options.RepositoryProject
	}

	return nil
}

func applyAzureDevOpsOverrides(repository *config.RepositoryConfig, options Options) error {
	if options.RepositoryOwner != nil {
		return fmt.Errorf(
			"%w: --owner is not valid for provider azuredevops",
			config.ErrInvalidConfig,
		)
	}

	if repository.AzureDevOps == nil {
		repository.AzureDevOps = &config.AzureDevOpsRepositoryConfig{}
	}

	azure := repository.AzureDevOps

	if options.RepositoryHost != nil {
		azure.Host = *options.RepositoryHost
	}

	if options.RepositoryRepo != nil {
		azure.Repo = *options.RepositoryRepo
	}

	if options.RepositoryProject != nil {
		azure.Project = *options.RepositoryProject
	}

	return nil
}

func providerChanged(previous, next config.ProviderType) bool {
	previousProvider := normalizedProvider(previous)
	nextProvider := normalizedProvider(next)

	if previousProvider == "" || nextProvider == "" {
		return false
	}

	return previousProvider != nextProvider
}

func normalizedProvider(providerType config.ProviderType) string {
	providerName := strings.TrimSpace(string(providerType))
	if config.ProviderType(providerName) == config.ProviderAuto {
		return ""
	}

	return providerName
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
