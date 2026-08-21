package provider

import (
	"context"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type openDependencies struct {
	getRemoteURL gitRemoteURLGetter
	create       func(*repositoryDescriptor, providerSettings) (forge.Provider, error)
}

func Open(
	ctx context.Context,
	cfg *config.Config,
	releaseBranch string,
) (forge.Provider, config.ProviderType, error) {
	return openResolved(ctx, cfg, providerSettings{releaseBranch: releaseBranch}, openDependencies{
		getRemoteURL: gitRemoteURL,
		create:       createConfigured,
	})
}

func open(
	ctx context.Context,
	cfg *config.Config,
	settings providerSettings,
	dependencies openDependencies,
) (forge.Provider, error) {
	provider, _, err := openResolved(ctx, cfg, settings, dependencies)

	return provider, err
}

func openResolved(
	ctx context.Context,
	cfg *config.Config,
	settings providerSettings,
	dependencies openDependencies,
) (forge.Provider, config.ProviderType, error) {
	settings.network = &cfg.Network
	settings.mergePolling = &cfg.Release.MergePolling

	repository, err := resolveRepository(ctx, cfg, dependencies.getRemoteURL)
	if err != nil {
		return nil, "", err
	}

	provider, err := dependencies.create(repository, settings)
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
