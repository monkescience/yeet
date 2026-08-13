package config

import (
	"fmt"
	"net/url"
	"strings"
)

func validateProvider(provider ProviderType) error {
	switch provider {
	case ProviderAuto, ProviderGitHub, ProviderGitLab, ProviderAzureDevOps:
		return nil
	default:
		return fmt.Errorf(
			"%w: provider must be \"auto\", \"github\", \"gitlab\", or \"azuredevops\", got %q",
			ErrInvalidConfig,
			provider,
		)
	}
}

func validateRepositorySubsection(repository *RepositoryConfig, provider ProviderType) error {
	set := []ProviderType{}
	if repository.GitHub != nil {
		set = append(set, ProviderGitHub)
	}

	if repository.GitLab != nil {
		set = append(set, ProviderGitLab)
	}

	if repository.AzureDevOps != nil {
		set = append(set, ProviderAzureDevOps)
	}

	if len(set) > 1 {
		return fmt.Errorf(
			"%w: only one of repository.github, repository.gitlab, repository.azuredevops may be set",
			ErrInvalidConfig,
		)
	}

	if provider == ProviderAuto && len(set) == 1 {
		return fmt.Errorf(
			"%w: repository.%s set but provider is auto. Set an explicit provider",
			ErrInvalidConfig,
			set[0],
		)
	}

	if len(set) == 1 && set[0] != provider {
		return fmt.Errorf(
			"%w: repository.%s set but provider is %s",
			ErrInvalidConfig,
			set[0],
			provider,
		)
	}

	return nil
}

func validateRepositoryConfig(provider ProviderType, repository RepositoryConfig) error {
	if strings.TrimSpace(repository.Remote) == "" {
		return fmt.Errorf("%w: repository.remote must not be empty", ErrInvalidConfig)
	}

	switch provider {
	case ProviderGitHub:
		return validateGitHubRepositoryConfig(repository.GitHub)
	case ProviderGitLab:
		return validateGitLabRepositoryConfig(repository.GitLab)
	case ProviderAzureDevOps:
		return validateAzureDevOpsRepositoryConfig(repository.AzureDevOps)
	case ProviderAuto:
		return nil
	default:
		return nil
	}
}

func validateGitHubRepositoryConfig(github *GitHubRepositoryConfig) error {
	if github == nil {
		return nil
	}

	host := strings.TrimSpace(github.Host)
	owner := strings.TrimSpace(github.Owner)
	repo := strings.TrimSpace(github.Repo)
	project := normalizeRepositoryProjectPath(github.Project)

	if github.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.github.host must not be blank", ErrInvalidConfig)
	}

	if err := validateRepositoryURLs("repository.github", github.APIURL, github.WebURL); err != nil {
		return err
	}

	if github.Owner != "" && owner == "" {
		return fmt.Errorf("%w: repository.github.owner must not be blank", ErrInvalidConfig)
	}

	if github.Repo != "" && repo == "" {
		return fmt.Errorf("%w: repository.github.repo must not be blank", ErrInvalidConfig)
	}

	if github.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.github.project must not be blank", ErrInvalidConfig)
	}

	if (owner == "") != (repo == "") {
		return fmt.Errorf(
			"%w: repository.github.owner and repository.github.repo must be set together",
			ErrInvalidConfig,
		)
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		return fmt.Errorf(
			"%w: repository.github.project must match repository.github.owner/repo",
			ErrInvalidConfig,
		)
	}

	if strings.Contains(owner, "/") {
		return fmt.Errorf("%w: repository.github.owner must not contain '/'", ErrInvalidConfig)
	}

	if project != "" {
		projectOwner, _, ok := splitGitHubProjectPath(project)
		if !ok || strings.Contains(projectOwner, "/") {
			return fmt.Errorf(
				"%w: repository.github.project must be in owner/repo form",
				ErrInvalidConfig,
			)
		}
	}

	return nil
}

func validateGitLabRepositoryConfig(gitlab *GitLabRepositoryConfig) error {
	if gitlab == nil {
		return nil
	}

	host := strings.TrimSpace(gitlab.Host)
	project := normalizeRepositoryProjectPath(gitlab.Project)

	if gitlab.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.gitlab.host must not be blank", ErrInvalidConfig)
	}

	if err := validateRepositoryURLs("repository.gitlab", gitlab.APIURL, gitlab.WebURL); err != nil {
		return err
	}

	if gitlab.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.gitlab.project must not be blank", ErrInvalidConfig)
	}

	return nil
}

func validateAzureDevOpsRepositoryConfig(azure *AzureDevOpsRepositoryConfig) error {
	if azure == nil {
		return fmt.Errorf("%w: repository.azuredevops is required when provider is azuredevops", ErrInvalidConfig)
	}

	host := strings.TrimSpace(azure.Host)
	organization := strings.TrimSpace(azure.Organization)
	project := normalizeRepositoryProjectPath(azure.Project)
	repo := strings.TrimSpace(azure.Repo)
	collection := strings.TrimSpace(azure.Collection)

	if azure.Host != "" && host == "" {
		return fmt.Errorf("%w: repository.azuredevops.host must not be blank", ErrInvalidConfig)
	}

	if err := validateRepositoryURLs("repository.azuredevops", azure.APIURL, azure.WebURL); err != nil {
		return err
	}

	if azure.Organization != "" && organization == "" {
		return fmt.Errorf("%w: repository.azuredevops.organization must not be blank", ErrInvalidConfig)
	}

	if azure.Project != "" && project == "" {
		return fmt.Errorf("%w: repository.azuredevops.project must not be blank", ErrInvalidConfig)
	}

	if azure.Repo != "" && repo == "" {
		return fmt.Errorf("%w: repository.azuredevops.repo must not be blank", ErrInvalidConfig)
	}

	if azure.Collection != "" && collection == "" {
		return fmt.Errorf("%w: repository.azuredevops.collection must not be blank", ErrInvalidConfig)
	}

	if organization == "" {
		return fmt.Errorf("%w: repository.azuredevops.organization is required", ErrInvalidConfig)
	}

	if project == "" {
		return fmt.Errorf("%w: repository.azuredevops.project is required", ErrInvalidConfig)
	}

	if repo == "" {
		return fmt.Errorf("%w: repository.azuredevops.repo is required", ErrInvalidConfig)
	}

	return nil
}

func validateRepositoryURLs(path, apiURL, webURL string) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "api_url", value: apiURL},
		{name: "web_url", value: webURL},
	} {
		if field.value == "" {
			continue
		}

		if err := validateHTTPSURL(field.value); err != nil {
			return fmt.Errorf("%w: %s.%s %v", ErrInvalidConfig, path, field.name, err)
		}
	}

	return nil
}

func validateHTTPSURL(value string) error {
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("must not contain surrounding whitespace")
	}

	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.Opaque != "" {
		return fmt.Errorf("must be an absolute HTTPS URL")
	}

	if parsed.User != nil {
		return fmt.Errorf("must not contain credentials")
	}

	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("must not contain a query")
	}

	if strings.Contains(value, "#") {
		return fmt.Errorf("must not contain a fragment")
	}

	return nil
}

func normalizeRepositoryProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

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
