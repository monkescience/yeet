package fixture

import "net/http/httptest"

func GitHubEnv(server *httptest.Server, branch string) []string {
	return []string{
		"GITHUB_TOKEN=test-token",
		"GITHUB_URL=" + server.URL + "/api/v3/",
		"GITHUB_REF=refs/heads/" + branch,
		"GITHUB_REF_NAME=" + branch,
	}
}

func GitLabEnv(server *httptest.Server, branch string) []string {
	return []string{
		"GITLAB_TOKEN=test-token",
		"GITLAB_URL=" + server.URL + "/api/v4",
		"GITHUB_REF_NAME=" + branch,
	}
}

func AzureEnv(server *httptest.Server, branch string) []string {
	return []string{
		"AZURE_DEVOPS_EXT_PAT=test-token",
		"AZURE_DEVOPS_URL=" + server.URL,
		"GITHUB_REF_NAME=" + branch,
	}
}
