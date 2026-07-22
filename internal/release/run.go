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
	ErrUnconfiguredReleaseBranch = errors.New("branch is not configured for releases")
	ErrUnknownReleaseChannel     = errors.New("unknown release channel")
)

type Options struct {
	DryRun               bool
	Provider             string
	ProviderSet          bool
	RepositoryRemote     string
	RepositoryRemoteSet  bool
	RepositoryHost       string
	RepositoryHostSet    bool
	RepositoryOwner      string
	RepositoryOwnerSet   bool
	RepositoryRepo       string
	RepositoryRepoSet    bool
	RepositoryProject    string
	RepositoryProjectSet bool
	AutoMerge            bool
	AutoMergeSet         bool
	AutoMergeForce       bool
	AutoMergeForceSet    bool
	AutoMergeMethod      string
	AutoMergeMethodSet   bool
	Channel              string
	ChannelSet           bool
	Targets              []string
}

type ConfigError struct {
	path string
	err  error
}

func (e *ConfigError) Error() string {
	return e.err.Error()
}

func (e *ConfigError) Unwrap() error {
	return e.err
}

func (e *ConfigError) Path() string {
	return e.path
}

type ExecutionError struct {
	err error
}

func (e *ExecutionError) Error() string {
	return e.err.Error()
}

func (e *ExecutionError) Unwrap() error {
	return e.err
}

func Run(ctx context.Context, configPath string, options Options) (*Result, error) {
	cfg, err := Prepare(ctx, configPath, options)
	if err != nil {
		return nil, err
	}

	repository, err := provider.ResolveRepository(ctx, cfg, provider.GitRemoteURL)
	if err != nil {
		return nil, fmt.Errorf("repository resolution failed: %w", err)
	}

	p, err := provider.Create(repository)
	if err != nil {
		return nil, fmt.Errorf("provider setup failed: %w", err)
	}

	historySource := history.New(p, cfg.Branch, ".")

	r, err := New(ctx, cfg, p, historySource)
	if err != nil {
		return nil, &ConfigError{path: configPath, err: err}
	}

	if err := r.ValidateTargets(options.Targets); err != nil {
		return nil, &ExecutionError{err: err}
	}

	if err := historySource.Validate(ctx); err != nil {
		return nil, &ExecutionError{err: err}
	}

	result, err := r.ReleaseTargets(ctx, options.DryRun, options.Targets)
	if err != nil {
		return nil, &ExecutionError{err: err}
	}

	return result, nil
}

func Prepare(ctx context.Context, configPath string, options Options) (*config.Config, error) {
	cfg, resolvedConfigPath, err := config.LoadResolved(ctx, configPath)
	if err != nil {
		return nil, &ConfigError{path: resolvedConfigPath, err: err}
	}

	logRun(ctx, resolvedConfigPath, options)

	if err := ApplyOptions(cfg, options); err != nil {
		return nil, fmt.Errorf("invalid release options: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid release options: %w", err)
	}

	currentBranch, branchErr := currentGitBranch(ctx)
	if branchErr != nil {
		if !options.DryRun && (errors.Is(branchErr, ErrCINonBranchRef) || len(cfg.Release.Channels) > 0) {
			return nil, fmt.Errorf("resolve current branch: %w", branchErr)
		}

		slog.DebugContext(ctx, "could not determine current branch (using configured default branch)",
			slog.Any("error", branchErr),
		)
	}

	if err := ResolveMode(cfg, currentBranch, options); err != nil {
		return nil, fmt.Errorf("invalid release options: %w", err)
	}

	return cfg, nil
}

func IsMergeBlocked(err error) bool {
	return errors.Is(err, provider.ErrMergeBlocked)
}

func ResolveMode(cfg *config.Config, currentBranch string, options Options) error {
	currentBranch = strings.TrimSpace(currentBranch)

	if options.ChannelSet {
		return ResolveExplicitChannel(cfg, currentBranch, options)
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
		ErrUnconfiguredReleaseBranch,
		currentBranch,
	)
}

func ResolveExplicitChannel(cfg *config.Config, currentBranch string, options Options) error {
	channelName := strings.TrimSpace(options.Channel)

	channel, exists := cfg.Release.Channels[channelName]
	if !exists {
		return fmt.Errorf("%w: %q", ErrUnknownReleaseChannel, channelName)
	}

	channelBranch := strings.TrimSpace(channel.Branch)
	if !options.DryRun && strings.TrimSpace(currentBranch) != channelBranch {
		return fmt.Errorf(
			"%w: channel %q must run on branch %q, got %q",
			ErrUnconfiguredReleaseBranch,
			channelName,
			channelBranch,
			currentBranch,
		)
	}

	cfg.Branch = channelBranch
	cfg.ActiveChannel = channelName

	return nil
}

func ApplyOptions(cfg *config.Config, options Options) error {
	if err := applyRepositoryOptions(cfg, options); err != nil {
		return err
	}

	ApplyBehaviorOptions(cfg, options)

	return nil
}

func applyRepositoryOptions(cfg *config.Config, options Options) error {
	previousProvider := cfg.Provider

	if options.ProviderSet {
		cfg.Provider = config.ProviderType(options.Provider)
	}

	if options.RepositoryRemoteSet {
		cfg.Repository.Remote = options.RepositoryRemote
	}

	hasRepoFieldOverride := options.RepositoryHostSet ||
		options.RepositoryOwnerSet ||
		options.RepositoryRepoSet ||
		options.RepositoryProjectSet

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

	if options.RepositoryHostSet {
		github.Host = options.RepositoryHost
	}

	if options.RepositoryOwnerSet {
		github.Owner = options.RepositoryOwner
	}

	if options.RepositoryRepoSet {
		github.Repo = options.RepositoryRepo
	}

	if options.RepositoryProjectSet {
		github.Project = options.RepositoryProject

		if !options.RepositoryOwnerSet {
			github.Owner = ""
		}

		if !options.RepositoryRepoSet {
			github.Repo = ""
		}
	}

	if !options.RepositoryProjectSet &&
		(options.RepositoryOwnerSet || options.RepositoryRepoSet) &&
		strings.TrimSpace(github.Owner) != "" &&
		strings.TrimSpace(github.Repo) != "" {
		github.Project = ""
	}

	return nil
}

func applyGitLabOverrides(repository *config.RepositoryConfig, options Options) error {
	if options.RepositoryOwnerSet || options.RepositoryRepoSet {
		return fmt.Errorf(
			"%w: --owner/--repo are not valid for provider gitlab. Use --project",
			config.ErrInvalidConfig,
		)
	}

	if repository.GitLab == nil {
		repository.GitLab = &config.GitLabRepositoryConfig{}
	}

	gitlab := repository.GitLab

	if options.RepositoryHostSet {
		gitlab.Host = options.RepositoryHost
	}

	if options.RepositoryProjectSet {
		gitlab.Project = options.RepositoryProject
	}

	return nil
}

func applyAzureDevOpsOverrides(repository *config.RepositoryConfig, options Options) error {
	if options.RepositoryOwnerSet {
		return fmt.Errorf(
			"%w: --owner is not valid for provider azuredevops",
			config.ErrInvalidConfig,
		)
	}

	if repository.AzureDevOps == nil {
		repository.AzureDevOps = &config.AzureDevOpsRepositoryConfig{}
	}

	azure := repository.AzureDevOps

	if options.RepositoryHostSet {
		azure.Host = options.RepositoryHost
	}

	if options.RepositoryRepoSet {
		azure.Repo = options.RepositoryRepo
	}

	if options.RepositoryProjectSet {
		azure.Project = options.RepositoryProject
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

func ApplyBehaviorOptions(cfg *config.Config, options Options) {
	if options.AutoMergeSet {
		cfg.Release.AutoMerge = options.AutoMerge
		if !options.AutoMerge {
			cfg.Release.AutoMergeForce = false
		}
	}

	if options.AutoMergeForceSet {
		cfg.Release.AutoMergeForce = options.AutoMergeForce
	}

	if options.AutoMergeMethodSet {
		cfg.Release.AutoMergeMethod = config.AutoMergeMethod(options.AutoMergeMethod)
	}

	if cfg.Release.AutoMergeForce {
		cfg.Release.AutoMerge = true
	}
}

func logRun(ctx context.Context, configPath string, options Options) {
	slog.DebugContext(ctx, "running release command",
		slog.String("config", configPath),
		slog.Bool("dry_run", options.DryRun),
		slog.Bool("provider_override_set", options.ProviderSet),
		slog.Bool("remote_override_set", options.RepositoryRemoteSet),
		slog.Bool("host_override_set", options.RepositoryHostSet),
		slog.Bool("owner_override_set", options.RepositoryOwnerSet),
		slog.Bool("repo_override_set", options.RepositoryRepoSet),
		slog.Bool("project_override_set", options.RepositoryProjectSet),
		slog.String("channel", options.Channel),
		slog.Bool("channel_set", options.ChannelSet),
		slog.Any("targets", options.Targets),
	)
}
