package provider

import (
	"context"
	"errors"
	"strings"

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
		if _, ok := errors.AsType[*SetupError](err); ok {
			return nil, "", err
		}

		providerName, remote := string(cfg.Provider), cfg.Repository.Remote
		if overrides.Provider != nil {
			providerName = strings.TrimSpace(*overrides.Provider)
		}

		if overrides.Remote != nil {
			remote = strings.TrimSpace(*overrides.Remote)
		}

		return nil, "", unresolvedRepositoryError(providerName, remote, err)
	}

	resolved, err := resolvedRepositoryFromDescriptor(repository)
	if err != nil {
		setup := unresolvedRepositoryError(repository.Provider, repository.Remote, err)
		setup.Host = repository.Host

		return nil, "", setup
	}

	provider, err := dependencies.create(resolved, settings)
	if err != nil {
		return nil, "", &SetupError{
			Host:     repository.Host,
			Remote:   repository.Remote,
			Provider: repository.Provider,
			Problem:  "provider setup failed",
			Err:      err,
		}
	}

	return provider, config.ProviderType(repository.Provider), nil
}

func unresolvedRepositoryError(providerName, remote string, err error) *SetupError {
	diagnosis := setupProblem(err)

	return &SetupError{
		Provider:   providerName,
		Remote:     remote,
		RemoteHost: diagnosis.remoteHost,
		Problem:    diagnosis.problem,
		Hint:       diagnosis.hint,
		Err:        err,
	}
}

type providerSettings struct {
	releaseBranch string
	network       *config.NetworkConfig
	mergePolling  *config.ReleaseMergePollingConfig
}
