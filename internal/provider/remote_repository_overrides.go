package provider

import (
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

func validateOverriddenRepositoryFields(
	cfg *config.Config,
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
) error {
	hasFieldOverride := overrides.Host != nil || overrides.Owner != nil ||
		overrides.Repo != nil || overrides.Project != nil

	hasAzureProviderOverride := overrides.Provider != nil &&
		strings.EqualFold(strings.TrimSpace(*overrides.Provider), providerNameAzureDevOps)
	if !hasFieldOverride && !hasAzureProviderOverride {
		return nil
	}

	fieldError := func(path, field, message string) error {
		return config.Invalidf("%s.%s %s", path, field, message)
	}

	if overrides.Host != nil && *overrides.Host != "" && strings.TrimSpace(*overrides.Host) == "" {
		path := "repository." + repository.Provider

		return fieldError(path, "host", "must not be blank")
	}

	switch repository.Provider {
	case providerNameGitHub:
		return validateGitHubOverrideFields(repository, overrides, fieldError)

	case providerNameGitLab:
		return validateGitLabOverrideFields(repository, overrides, fieldError)

	case providerNameAzureDevOps:
		return validateAzureOverrideFields(cfg, repository, overrides, fieldError)
	}

	return nil
}

func validateGitHubOverrideFields(
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	owner := strings.TrimSpace(repository.Owner)
	repo := strings.TrimSpace(repository.Repo)
	project := normalizeRepositoryProjectPath(repository.Project)

	if overrides.Owner != nil && *overrides.Owner != "" && owner == "" {
		return fieldError("repository.github", "owner", "must not be blank")
	}

	if overrides.Repo != nil && *overrides.Repo != "" && repo == "" {
		return fieldError("repository.github", "repo", "must not be blank")
	}

	if overrides.Project != nil && *overrides.Project != "" && project == "" {
		return fieldError("repository.github", "project", "must not be blank")
	}

	if (owner == "") != (repo == "") {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.github.owner and repository.github.repo must be set together")
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.github.project must match repository.github.owner/repo")
	}

	if strings.Contains(owner, "/") {
		return fieldError("repository.github", "owner", "must not contain '/'")
	}

	if project == "" {
		return nil
	}

	projectOwner, _, ok := splitGitHubProjectPath(project)
	if !ok || strings.Contains(projectOwner, "/") {
		return fieldError("repository.github", "project", "must be in owner/repo form")
	}

	return nil
}

func validateGitLabOverrideFields(
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	if overrides.Project == nil || *overrides.Project == "" || normalizeRepositoryProjectPath(repository.Project) != "" {
		return nil
	}

	return fieldError("repository.gitlab", "project", "must not be blank")
}

func validateAzureOverrideFields(
	cfg *config.Config,
	repository *repositoryDescriptor,
	overrides RepositoryOverrides,
	fieldError func(string, string, string) error,
) error {
	organization := strings.TrimSpace(repository.Organization)
	project := normalizeRepositoryProjectPath(repository.Project)
	repo := strings.TrimSpace(repository.Repo)

	if overrides.Project != nil && *overrides.Project != "" && project == "" {
		return fieldError("repository.azuredevops", "project", "must not be blank")
	}

	if overrides.Repo != nil && *overrides.Repo != "" && repo == "" {
		return fieldError("repository.azuredevops", "repo", "must not be blank")
	}

	requiresExplicitCoordinates := overrides.Provider != nil || cfg.Provider == config.ProviderAzureDevOps
	if !requiresExplicitCoordinates {
		return nil
	}

	if organization == "" {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.azuredevops.organization is required")
	}

	if project == "" {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.azuredevops.project is required")
	}

	if repo == "" {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.azuredevops.repo is required")
	}

	return nil
}

func normalizeRepositoryProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

const githubProjectSegments = 2

func splitGitHubProjectPath(project string) (string, string, bool) {
	parts := strings.Split(project, "/")
	if len(parts) != githubProjectSegments {
		return "", "", false
	}

	owner := strings.TrimSpace(parts[0])

	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}

	return owner, repo, true
}

func validateRepositoryOverrides(cfg *config.Config, overrides RepositoryOverrides) error {
	provider := cfg.Provider
	if provider == config.ProviderAuto && overrides.Provider == nil {
		var section string

		switch {
		case cfg.Repository.GitHub != nil:
			section = providerNameGitHub
		case cfg.Repository.GitLab != nil:
			section = providerNameGitLab
		case cfg.Repository.AzureDevOps != nil:
			section = providerNameAzureDevOps
		}

		if section != "" {
			//nolint:wrapcheck // config supplies the typed validation context.
			return config.Invalidf("repository.%s set but provider is auto. Set an explicit provider", section)
		}
	}

	if overrides.Provider != nil {
		provider = config.ProviderType(*overrides.Provider)

		providerErr := validateOverrideProvider(provider)
		if providerErr != nil {
			return providerErr
		}
	}

	if overrides.Remote != nil && strings.TrimSpace(*overrides.Remote) == "" {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository.remote must not be empty")
	}

	hasCoordinates := overrides.Host != nil || overrides.Owner != nil ||
		overrides.Repo != nil || overrides.Project != nil
	if provider == config.ProviderAuto && hasCoordinates {
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("repository field flags require an explicit --provider (auto cannot route them)")
	}

	return validateRepositoryOverrideRouting(provider, overrides)
}

func validateRepositoryOverrideRouting(
	provider config.ProviderType,
	overrides RepositoryOverrides,
) error {
	switch provider {
	case config.ProviderGitLab:
		if overrides.Owner != nil || overrides.Repo != nil {
			//nolint:wrapcheck // config supplies the typed validation context.
			return config.Invalidf("--owner/--repo are not valid for provider gitlab. Use --project")
		}
	case config.ProviderAzureDevOps:
		if overrides.Owner != nil {
			//nolint:wrapcheck // config supplies the typed validation context.
			return config.Invalidf("--owner is not valid for provider azuredevops")
		}
	case config.ProviderAuto, config.ProviderGitHub:
	}

	return nil
}

func validateOverrideProvider(provider config.ProviderType) error {
	switch provider {
	case config.ProviderAuto, config.ProviderGitHub, config.ProviderGitLab, config.ProviderAzureDevOps:
		return nil
	default:
		//nolint:wrapcheck // config supplies the typed validation context.
		return config.Invalidf("provider must be \"auto\", \"github\", \"gitlab\", or \"azuredevops\", got %q", provider)
	}
}

func repositoryFromConfig(cfg *config.Config) *repositoryDescriptor {
	descriptor := &repositoryDescriptor{
		Provider: normalizedRepositoryProvider(cfg.Provider),
		Remote:   strings.TrimSpace(cfg.Repository.Remote),
	}

	switch cfg.Provider {
	case config.ProviderGitHub:
		if cfg.Repository.GitHub == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.GitHub.Host)
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.GitHub.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.GitHub.WebURL)
		descriptor.Owner = strings.TrimSpace(cfg.Repository.GitHub.Owner)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.GitHub.Repo)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitHub.Project)
	case config.ProviderGitLab:
		if cfg.Repository.GitLab == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.GitLab.Host)
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.GitLab.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.GitLab.WebURL)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitLab.Project)
	case config.ProviderAzureDevOps:
		if cfg.Repository.AzureDevOps == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.AzureDevOps.Host)
		descriptor.APIURL = strings.TrimSpace(cfg.Repository.AzureDevOps.APIURL)
		descriptor.WebURL = strings.TrimSpace(cfg.Repository.AzureDevOps.WebURL)
		descriptor.Organization = strings.TrimSpace(cfg.Repository.AzureDevOps.Organization)
		descriptor.Project = strings.TrimSpace(cfg.Repository.AzureDevOps.Project)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.AzureDevOps.Repo)
		descriptor.Collection = strings.TrimSpace(cfg.Repository.AzureDevOps.Collection)
	case config.ProviderAuto:
	}

	return descriptor
}

func applyRepositoryOverrides(
	repository *repositoryDescriptor,
	configuredProvider config.ProviderType,
	overrides RepositoryOverrides,
) {
	if overrides.Provider != nil {
		overrideProvider := strings.TrimSpace(*overrides.Provider)

		remote := repository.Remote
		if overrideProvider == providerNameAuto ||
			(normalizedRepositoryProvider(configuredProvider) != "" &&
				overrideProvider != normalizedRepositoryProvider(configuredProvider)) {
			*repository = repositoryDescriptor{}
			repository.Remote = remote
		}

		repository.Provider = normalizedRepositoryProvider(config.ProviderType(overrideProvider))
	}

	if overrides.Remote != nil {
		repository.Remote = *overrides.Remote
	}

	if overrides.Host != nil {
		repository.Host = *overrides.Host
	}

	if overrides.Owner != nil {
		repository.Owner = *overrides.Owner
	}

	if overrides.Repo != nil {
		repository.Repo = *overrides.Repo
	}

	if overrides.Project != nil {
		repository.Project = *overrides.Project

		if overrides.Owner == nil {
			repository.Owner = ""
		}

		if overrides.Repo == nil {
			repository.Repo = ""
		}
	}

	if overrides.Project == nil &&
		(overrides.Owner != nil || overrides.Repo != nil) &&
		strings.TrimSpace(repository.Owner) != "" &&
		strings.TrimSpace(repository.Repo) != "" {
		repository.Project = ""
	}
}

func normalizedRepositoryProvider(providerType config.ProviderType) string {
	provider := strings.TrimSpace(string(providerType))
	if provider == providerNameAuto {
		return ""
	}

	return provider
}
