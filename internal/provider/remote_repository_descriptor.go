package provider

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrGitHubRepoRequired      = errors.New("resolve github repository: owner and repo are required")
	ErrGitHubOwnerInvalid      = errors.New("resolve github repository: owner must not contain '/'")
	ErrGitLabProjectNeeded     = errors.New("resolve gitlab repository: project or owner/repo are required")
	ErrAzureDevOpsCoordsNeeded = errors.New("resolve azuredevops repository: organization, project, and repo are required")
	ErrRepositoryConflict      = errors.New("resolve repository: project does not match owner/repo")
)

func resolveRepositoryProvider(repository *repositoryDescriptor) error {
	if repository.Provider == "" {
		providerType, err := detectType(repository.Host)
		if err != nil {
			return unsupportedAutoProviderError(repository.Host, err)
		}

		repository.Provider = providerType
	}

	return nil
}

func applyRepositoryProviderDefaults(repository *repositoryDescriptor) {
	spec, known := forgeSpecs[repository.Provider]
	if !known {
		return
	}

	if repository.Host == "" {
		repository.Host = spec.defaultHost
	}

	if repository.Provider == providerNameAzureDevOps && repository.Collection == "" {
		repository.Collection = repository.Organization
	}
}

func unsupportedAutoProviderError(host string, err error) error {
	return fmt.Errorf(
		"resolve repository provider for host %q: %w. "+
			"Auto-detection only supports github.com, gitlab.com, and dev.azure.com. "+
			"Set provider, [repository], or pass explicit flags for custom domains",
		host,
		err,
	)
}

func needsRemoteLookup(repository *repositoryDescriptor) bool {
	if !hasRepositoryCoordinates(repository) {
		return true
	}

	return repository.Provider == "" && repository.Host == ""
}

func hasRepositoryCoordinates(repository *repositoryDescriptor) bool {
	return repository.Project != "" || (repository.Owner != "" && repository.Repo != "")
}

func mergeRepositoryDescriptor(
	base *repositoryDescriptor,
	override *repositoryDescriptor,
) *repositoryDescriptor {
	if override.Provider != "" {
		base.Provider = override.Provider
	}

	if override.Host != "" {
		base.Host = override.Host
	}

	if override.APIURL != "" {
		base.APIURL = override.APIURL
	}

	if override.WebURL != "" {
		base.WebURL = override.WebURL
	}

	mergeRepositoryCoordinates(base, override)

	if override.Organization != "" {
		base.Organization = override.Organization
	}

	if override.Collection != "" {
		base.Collection = override.Collection
	}

	if override.Remote != "" {
		base.Remote = override.Remote
	}

	return base
}

func mergeRepositoryCoordinates(base *repositoryDescriptor, override *repositoryDescriptor) {
	switch {
	case override.Project != "":
		base.Project = override.Project
		base.Owner = override.Owner
		base.Repo = override.Repo
	case override.Owner != "" && override.Repo != "":
		base.Owner = override.Owner
		base.Repo = override.Repo
		base.Project = ""
	default:
		if override.Owner != "" {
			base.Owner = override.Owner
		}

		if override.Repo != "" {
			base.Repo = override.Repo
		}
	}
}

func normalizeRepositoryDescriptor(repository *repositoryDescriptor) {
	repository.Provider = strings.TrimSpace(repository.Provider)
	repository.Host = strings.TrimSpace(repository.Host)
	repository.APIURL = strings.TrimSpace(repository.APIURL)
	repository.WebURL = strings.TrimSpace(repository.WebURL)
	repository.Owner = strings.TrimSpace(repository.Owner)
	repository.Repo = strings.TrimSpace(repository.Repo)
	repository.Project = strings.Trim(strings.TrimSpace(repository.Project), "/")
	repository.Organization = strings.TrimSpace(repository.Organization)
	repository.Collection = strings.TrimSpace(repository.Collection)
	repository.Remote = strings.TrimSpace(repository.Remote)

	if repository.Project == "" && repository.Owner != "" && repository.Repo != "" {
		repository.Project = repository.Owner + "/" + repository.Repo
	}

	if repository.Project == "" || (repository.Owner != "" && repository.Repo != "") {
		return
	}

	owner, repo := splitProjectPath(repository.Project)
	if repository.Owner == "" {
		repository.Owner = owner
	}

	if repository.Repo == "" {
		repository.Repo = repo
	}
}

func validateRepositoryDescriptor(repository *repositoryDescriptor) error {
	err := validateRepositoryCoordinates(repository)
	if err != nil {
		return err
	}

	switch repository.Provider {
	case providerNameGitHub:
		if repository.Owner == "" || repository.Repo == "" {
			return ErrGitHubRepoRequired
		}

		if strings.Contains(repository.Owner, "/") {
			return fmt.Errorf("%w: %q", ErrGitHubOwnerInvalid, repository.Owner)
		}
	case providerNameGitLab:
		if repository.Project == "" {
			return ErrGitLabProjectNeeded
		}
	case providerNameAzureDevOps:
		if repository.Organization == "" || repository.Project == "" || repository.Repo == "" {
			return ErrAzureDevOpsCoordsNeeded
		}
	case providerNameAuto:
		return fmt.Errorf(
			"%w: %s (provider auto must be resolved before validation)",
			ErrUnsupportedProvider, repository.Provider,
		)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}

	return nil
}

func validateRepositoryCoordinates(repository *repositoryDescriptor) error {
	if repository.Project == "" || repository.Owner == "" || repository.Repo == "" {
		return nil
	}

	expectedProject := repository.Owner + "/" + repository.Repo
	if strings.EqualFold(repository.Project, expectedProject) {
		return nil
	}

	return fmt.Errorf(
		"%w: project %q does not match owner/repo %q",
		ErrRepositoryConflict,
		repository.Project,
		expectedProject,
	)
}

type repositoryDescriptor struct {
	Provider     string
	Host         string
	APIURL       string
	WebURL       string
	Owner        string
	Repo         string
	Project      string
	Organization string
	Collection   string
	Remote       string
}

func resolvedRepositoryFromDescriptor(repository *repositoryDescriptor) (resolvedRepository, error) {
	switch repository.Provider {
	case providerNameGitHub:
		return &resolvedGitHubRepository{
			Host:   repository.Host,
			APIURL: repository.APIURL,
			WebURL: repository.WebURL,
			Owner:  repository.Owner,
			Repo:   repository.Repo,
		}, nil
	case providerNameGitLab:
		return &resolvedGitLabRepository{
			Host:    repository.Host,
			APIURL:  repository.APIURL,
			WebURL:  repository.WebURL,
			Project: repository.Project,
		}, nil
	case providerNameAzureDevOps:
		return &resolvedAzureDevOpsRepository{
			Host:         repository.Host,
			APIURL:       repository.APIURL,
			WebURL:       repository.WebURL,
			Organization: repository.Organization,
			Collection:   repository.Collection,
			Project:      repository.Project,
			Repo:         repository.Repo,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}
}

func detectType(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", fmt.Errorf("%w: empty host", ErrUnsupportedHost)
	}

	if host == DefaultGitHubHost {
		return providerNameGitHub, nil
	}

	if host == DefaultGitLabHost {
		return providerNameGitLab, nil
	}

	if isAzureDevOpsHost(host) {
		return providerNameAzureDevOps, nil
	}

	return "", fmt.Errorf("%w: %s", ErrUnsupportedHost, host)
}

func isAzureDevOpsHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))

	return host == DefaultAzureDevOpsHost ||
		host == azureDevOpsSSHHost ||
		strings.HasSuffix(host, azureDevOpsLegacyHostSuffix)
}
