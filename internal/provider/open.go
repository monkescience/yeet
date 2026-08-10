package provider

import (
	"context"

	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

type openDependencies struct {
	getRemoteURL gitRemoteURLGetter
	create       func(*repositoryDescriptor) (forge.Provider, error)
}

func Open(ctx context.Context, cfg *config.Config) (forge.Provider, error) {
	return open(ctx, cfg, openDependencies{
		getRemoteURL: gitRemoteURL,
		create:       create,
	})
}

func open(ctx context.Context, cfg *config.Config, dependencies openDependencies) (forge.Provider, error) {
	repository, err := resolveRepository(ctx, cfg, dependencies.getRemoteURL)
	if err != nil {
		return nil, err
	}

	return dependencies.create(repository)
}
