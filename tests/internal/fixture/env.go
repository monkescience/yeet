package fixture

import "net/http/httptest"

// GitHubEnv returns the env-var pairs that point the yeet binary at a fake
// GitHub server for branch.
func GitHubEnv(server *httptest.Server, branch string) []string {
	return []string{
		"GITHUB_TOKEN=test-token",
		"GITHUB_URL=" + server.URL + "/api/v3/",
		"GITHUB_REF=refs/heads/" + branch,
		"GITHUB_REF_NAME=" + branch,
	}
}

// GitLabEnv returns the env-var pairs that point the yeet binary at a fake
// GitLab server for branch.
func GitLabEnv(server *httptest.Server, branch string) []string {
	return []string{
		"GITLAB_TOKEN=test-token",
		"GITLAB_URL=" + server.URL + "/api/v4",
		"GITHUB_REF_NAME=" + branch,
	}
}

// AzureEnv returns the env-var pairs that point the yeet binary at a fake
// Azure DevOps server for branch.
func AzureEnv(server *httptest.Server, branch string) []string {
	return []string{
		"AZURE_DEVOPS_EXT_PAT=test-token",
		"AZURE_DEVOPS_URL=" + server.URL,
		"GITHUB_REF_NAME=" + branch,
	}
}
