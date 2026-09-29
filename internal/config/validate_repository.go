package config

import (
	"errors"
	"net/url"
	"strings"
)

var (
	errHTTPSURLWhitespace  = errors.New("must not contain surrounding whitespace")
	errHTTPSURLAbsolute    = errors.New("must be an absolute HTTPS URL")
	errHTTPSURLCredentials = errors.New("must not contain credentials")
	errHTTPSURLQuery       = errors.New("must not contain a query")
	errHTTPSURLFragment    = errors.New("must not contain a fragment")
)

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
		return Invalidf("only one of repository.github, repository.gitlab, repository.azuredevops may be set")
	}

	if provider == ProviderAuto && len(set) == 1 {
		return Invalidf("repository.%s set but provider is auto. Set an explicit provider", set[0])
	}

	if len(set) == 1 && set[0] != provider {
		return Invalidf("repository.%s set but provider is %s", set[0],
			provider,
		)
	}

	return nil
}

func validateRepositoryConfig(provider ProviderType, repository RepositoryConfig) error {
	if strings.TrimSpace(repository.Remote) == "" {
		return Invalidf("repository.remote must not be empty")
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
	project := NormalizeRepositoryProjectPath(github.Project)

	if github.Host != "" && host == "" {
		return Invalidf("repository.github.host must not be blank")
	}

	err := validateRepositoryURLs("repository.github", github.APIURL, github.WebURL)
	if err != nil {
		return err
	}

	if github.Owner != "" && owner == "" {
		return Invalidf("repository.github.owner must not be blank")
	}

	if github.Repo != "" && repo == "" {
		return Invalidf("repository.github.repo must not be blank")
	}

	if github.Project != "" && project == "" {
		return Invalidf("repository.github.project must not be blank")
	}

	return github.ValidateCoordinates()
}

func validateGitLabRepositoryConfig(gitlab *GitLabRepositoryConfig) error {
	if gitlab == nil {
		return nil
	}

	host := strings.TrimSpace(gitlab.Host)
	project := NormalizeRepositoryProjectPath(gitlab.Project)

	if gitlab.Host != "" && host == "" {
		return Invalidf("repository.gitlab.host must not be blank")
	}

	err := validateRepositoryURLs("repository.gitlab", gitlab.APIURL, gitlab.WebURL)
	if err != nil {
		return err
	}

	if gitlab.Project != "" && project == "" {
		return Invalidf("repository.gitlab.project must not be blank")
	}

	return nil
}

func validateAzureDevOpsRepositoryConfig(azure *AzureDevOpsRepositoryConfig) error {
	if azure == nil {
		return Invalidf("repository.azuredevops is required when provider is azuredevops")
	}

	host := strings.TrimSpace(azure.Host)
	organization := strings.TrimSpace(azure.Organization)
	project := NormalizeRepositoryProjectPath(azure.Project)
	repo := strings.TrimSpace(azure.Repo)
	collection := strings.TrimSpace(azure.Collection)

	if azure.Host != "" && host == "" {
		return Invalidf("repository.azuredevops.host must not be blank")
	}

	err := validateRepositoryURLs("repository.azuredevops", azure.APIURL, azure.WebURL)
	if err != nil {
		return err
	}

	if azure.Organization != "" && organization == "" {
		return Invalidf("repository.azuredevops.organization must not be blank")
	}

	if azure.Project != "" && project == "" {
		return Invalidf("repository.azuredevops.project must not be blank")
	}

	if azure.Repo != "" && repo == "" {
		return Invalidf("repository.azuredevops.repo must not be blank")
	}

	if azure.Collection != "" && collection == "" {
		return Invalidf("repository.azuredevops.collection must not be blank")
	}

	return azure.ValidateCoordinates()
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

		err := validateHTTPSURL(field.value)
		if err != nil {
			return Invalidf("%s.%s %v", path, field.name, err)
		}
	}

	return nil
}

func validateHTTPSURL(value string) error {
	if strings.TrimSpace(value) != value {
		return errHTTPSURLWhitespace
	}

	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.Opaque != "" {
		return errHTTPSURLAbsolute
	}

	if parsed.User != nil {
		return errHTTPSURLCredentials
	}

	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errHTTPSURLQuery
	}

	if strings.Contains(value, "#") {
		return errHTTPSURLFragment
	}

	return nil
}
