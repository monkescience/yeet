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

func Open(ctx context.Context, cfg *config.Config, releaseBranch string) (forge.Provider, error) {
	return open(ctx, cfg, providerSettings{releaseBranch: releaseBranch}, openDependencies{
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
	settings.network = &cfg.Network

	repository, err := resolveRepository(ctx, cfg, dependencies.getRemoteURL)
	if err != nil {
		return nil, err
	}

	return dependencies.create(repository, settings)
}

type providerSettings struct {
	releaseBranch string
	network       *config.NetworkConfig
}
