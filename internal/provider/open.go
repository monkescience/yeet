package provider

import (
	"context"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type openDependencies struct {
	getRemoteURL gitRemoteURLGetter
	create       func(resolvedRepository, providerSettings) (forge.Provider, error)
}

// RepositoryOverrides carries command-level repository facts without
// mutating the loaded configuration. A nil field means that the command did
// not provide an override, while a non-nil pointer preserves an explicitly
// empty value.
type RepositoryOverrides struct {
	Provider *string
	Remote   *string
	Host     *string
	Owner    *string
	Repo     *string
	Project  *string
}

func Open(
	ctx context.Context,
	cfg *config.Config,
	releaseBranch string,
	overrides RepositoryOverrides,
) (forge.Provider, config.ProviderType, error) {
	return openResolved(ctx, cfg, providerSettings{releaseBranch: releaseBranch}, openDependencies{
		getRemoteURL: gitRemoteURL,
		create:       createConfigured,
	}, overrides)
}

//nolint:unparam // the explicit override record keeps the internal opening seam aligned with Open
func open(
	ctx context.Context,
	cfg *config.Config,
	settings providerSettings,
	dependencies openDependencies,
	overrides RepositoryOverrides,
) (forge.Provider, error) {
	provider, _, err := openResolved(ctx, cfg, settings, dependencies, overrides)

	return provider, err
}

func openResolved(
	ctx context.Context,
	cfg *config.Config,
	settings providerSettings,
	dependencies openDependencies,
	overrides RepositoryOverrides,
) (forge.Provider, config.ProviderType, error) {
	settings.network = &cfg.Network
	settings.mergePolling = &cfg.Release.MergePolling

	repository, err := resolveRepository(ctx, cfg, dependencies.getRemoteURL, overrides)
	if err != nil {
		return nil, "", err
	}

	resolved, err := resolvedRepositoryFromDescriptor(repository)
	if err != nil {
		return nil, "", err
	}

	provider, err := dependencies.create(resolved, settings)
	if err != nil {
		return nil, "", err
	}

	return provider, config.ProviderType(repository.Provider), nil
}

type providerSettings struct {
	releaseBranch string
	network       *config.NetworkConfig
	mergePolling  *config.ReleaseMergePollingConfig
}
