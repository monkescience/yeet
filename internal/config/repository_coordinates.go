package config

import "strings"

const githubProjectSegments = 2

func (p ProviderType) Validate() error {
	switch p {
	case ProviderAuto, ProviderGitHub, ProviderGitLab, ProviderAzureDevOps:
		return nil
	default:
		return Invalidf("provider must be \"auto\", \"github\", \"gitlab\", or \"azuredevops\", got %q", p)
	}
}

func (g GitHubRepositoryConfig) ValidateCoordinates() error {
	owner := strings.TrimSpace(g.Owner)
	repo := strings.TrimSpace(g.Repo)
	project := NormalizeRepositoryProjectPath(g.Project)

	if (owner == "") != (repo == "") {
		return Invalidf("repository.github.owner and repository.github.repo must be set together")
	}

	if project != "" && owner != "" && repo != "" && project != owner+"/"+repo {
		return Invalidf("repository.github.project must match repository.github.owner/repo")
	}

	if strings.Contains(owner, "/") {
		return Invalidf("repository.github.owner must not contain '/'")
	}

	if project != "" && !validGitHubProjectPath(project) {
		return Invalidf("repository.github.project must be in owner/repo form")
	}

	return nil
}

func (a AzureDevOpsRepositoryConfig) ValidateCoordinates() error {
	if strings.TrimSpace(a.Organization) == "" {
		return Invalidf("repository.azuredevops.organization is required")
	}

	if NormalizeRepositoryProjectPath(a.Project) == "" {
		return Invalidf("repository.azuredevops.project is required")
	}

	if strings.TrimSpace(a.Repo) == "" {
		return Invalidf("repository.azuredevops.repo is required")
	}

	return nil
}

func NormalizeRepositoryProjectPath(project string) string {
	return strings.Trim(strings.TrimSpace(project), "/")
}

func validGitHubProjectPath(project string) bool {
	parts := strings.Split(project, "/")
	if len(parts) != githubProjectSegments {
		return false
	}

	return strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}
