package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	AutoMergeMode     *string
	AutoMergeMethod   *string
	Channel           *string
	Targets           []string
}

func Run(ctx context.Context, configPath string, options Options) (*Result, error) {
	result, resolvedConfigPath, err := rawRun(ctx, configPath, options)
	if err != nil {
		return result, classifyFailure(resolvedConfigPath, err)
	}

	return result, nil
}

func rawRun(ctx context.Context, configPath string, options Options) (*Result, string, error) {
	cfg, run, resolvedConfigPath, err := prepareWithPath(ctx, configPath, options)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	p, resolvedProvider, err := provider.Open(ctx, cfg, run.releaseBranch, repositoryOverrides(options))
	if err != nil {
		if errors.Is(err, config.ErrInvalidConfig) {
			return nil, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
		}

		return nil, resolvedConfigPath, fmt.Errorf("provider setup failed: %w", err)
	}

	core, err := newReleaseCore(ctx, cfg, p, run)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	selection, err := selectTargets(core, options.Targets)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	historySource, err := history.Open(ctx, p, run.baseBranch, ".")
	if err != nil {
		checkout, ok := errors.AsType[*history.CheckoutError](err)
		if ok && checkout.Problem == history.CheckoutProblemSuperseded {
			return &Result{
				BaseBranch: run.baseBranch,
				Provider:   resolvedProvider,
				Superseded: &SupersededCheckout{
					Branch:     checkout.Branch,
					LocalHead:  checkout.LocalHead,
					RemoteHead: checkout.RemoteHead,
				},
			}, resolvedConfigPath, nil
		}

		return nil, resolvedConfigPath, fmt.Errorf("validate checkout: %w", err)
	}

	r, err := newReleaser(core, historySource, p)
	if err != nil {
		return nil, resolvedConfigPath, err
	}

	result, err := r.releaseTargets(ctx, options.DryRun, selection)
	if err != nil {
		if result != nil {
			result.Provider = resolvedProvider
		}

		return result, resolvedConfigPath, err
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

func prepare(ctx context.Context, options Options) (*config.Config, releaseRun, error) {
	cfg, run, _, err := prepareWithPath(ctx, config.DefaultFile, options)

	return cfg, run, err
}

func prepareWithPath(
	ctx context.Context,
	configPath string,
	options Options,
) (*config.Config, releaseRun, string, error) {
	cfg, resolvedConfigPath, err := config.LoadResolved(ctx, configPath)
	if err != nil {
		return nil, releaseRun{}, resolvedConfigPath, fmt.Errorf("load release config: %w", err)
	}

	logRun(ctx, resolvedConfigPath, options)

	err = cfg.Validate()
	if err != nil {
		return nil, releaseRun{}, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	err = validateReleaseBranchTemplates(cfg)
	if err != nil {
		return nil, releaseRun{}, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	currentBranch, branchErr := currentGitBranch(ctx)
	if branchErr != nil {
		if !options.DryRun && (errors.Is(branchErr, errCINonBranchRef) || len(cfg.Release.Channels) > 0) {
			return nil, releaseRun{}, resolvedConfigPath, fmt.Errorf("resolve current branch: %w", branchErr)
		}

		slog.DebugContext(ctx, "could not determine current branch (using configured default branch)",
			slog.Any("error", branchErr),
		)
	}

	run, err := resolveRun(cfg, currentBranch, options)
	if err != nil {
		return nil, releaseRun{}, resolvedConfigPath, fmt.Errorf("invalid release options: %w", err)
	}

	return cfg, run, resolvedConfigPath, nil
}

func logRun(ctx context.Context, configPath string, options Options) {
	attrs := []any{
		slog.String("config", configPath),
		slog.Bool("dry_run", options.DryRun),
	}

	overrides := make([]string, 0)

	for _, override := range []struct {
		name string
		set  bool
	}{
		{"provider", options.Provider != nil},
		{"remote", options.RepositoryRemote != nil},
		{"host", options.RepositoryHost != nil},
		{"owner", options.RepositoryOwner != nil},
		{"repo", options.RepositoryRepo != nil},
		{"project", options.RepositoryProject != nil},
	} {
		if override.set {
			overrides = append(overrides, override.name)
		}
	}

	if len(overrides) > 0 {
		attrs = append(attrs, slog.Any("overrides", overrides))
	}

	if options.Channel != nil {
		attrs = append(attrs, slog.String("channel", *options.Channel))
	}

	if len(options.Targets) > 0 {
		attrs = append(attrs, slog.Any("targets", options.Targets))
	}

	slog.DebugContext(ctx, "running release command", attrs...)
}
