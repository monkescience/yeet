package provider

import (
	"context"
	"fmt"

	"github.com/monkescience/yeet/internal/config"
)

type gitRemoteURLGetter func(context.Context, string) (string, error)

func resolveRepository(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
	overrides RepositoryOverrides,
) (*repositoryDescriptor, error) {
	repository, err := repositoryDescriptorFromSources(ctx, cfg, getRemoteURL, overrides)
	if err != nil {
		return nil, err
	}

	err = resolveRepositoryProvider(repository)
	if err != nil {
		return nil, err
	}

	applyRepositoryProviderDefaults(repository)
	normalizeRepositoryDescriptor(repository)

	err = validateRepositoryDescriptor(repository)
	if err != nil {
		return nil, err
	}

	err = validateProviderHostTrust(ctx, repository, getRemoteURL)
	if err != nil {
		return nil, err
	}

	return repository, nil
}

func repositoryDescriptorFromSources(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
	overrides RepositoryOverrides,
) (*repositoryDescriptor, error) {
	coordinateErr := validateRepositoryOverrides(cfg, overrides)
	if coordinateErr != nil {
		return nil, coordinateErr
	}

	repository := repositoryFromConfig(cfg)
	applyRepositoryOverrides(repository, cfg.Provider, overrides)

	overrideErr := validateOverriddenRepositoryFields(cfg, repository, overrides)
	if overrideErr != nil {
		return nil, overrideErr
	}

	if repository.Remote == "" {
		repository.Remote = "origin"
	}

	if needsRemoteLookup(repository) {
		remoteURL, err := getRemoteURL(ctx, repository.Remote)
		if err != nil {
			return nil, fmt.Errorf("get git remote %q url: %w", repository.Remote, err)
		}

		detected, err := parseRemote(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse git remote %q url: %w", repository.Remote, err)
		}

		detected.Remote = repository.Remote
		repository = mergeRepositoryDescriptor(detected, repository)
	}

	normalizeRepositoryDescriptor(repository)

	return repository, nil
}
