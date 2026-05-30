package provider

import (
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var _ Provider = (*GitLab)(nil)

type GitLab struct {
	client                *gitlab.Client
	pid                   string
	baseURL               string
	maxConcurrentRequests int
}

// NewGitLab creates a provider. pid is the project ID or full path (e.g., "owner/repo").
func NewGitLab(client *gitlab.Client, project string, opts ...Option) *GitLab {
	baseURL := strings.TrimSuffix(client.BaseURL().String(), "/api/v4/")

	return &GitLab{
		client:                client,
		pid:                   project,
		baseURL:               baseURL + "/" + project,
		maxConcurrentRequests: newConcurrencyConfig(opts).maxConcurrentRequests,
	}
}

func (g *GitLab) RepoURL() string {
	return g.baseURL
}

func (g *GitLab) PathPrefix() string {
	return "/-"
}

func (g *GitLab) CompareURL(fromRef, toRef string) string {
	return fmt.Sprintf("%s/-/compare/%s...%s", g.RepoURL(), fromRef, toRef)
}

func gitLabNextPage(resp *gitlab.Response) int {
	if resp == nil {
		return 0
	}

	return int(resp.NextPage)
}
