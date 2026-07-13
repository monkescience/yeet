package provider

import (
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var _ Provider = (*GitLab)(nil)

type GitLab struct {
	client                *gitlab.Client
	projectID             string
	repoURL               string
	maxConcurrentRequests int
}

func NewGitLab(client *gitlab.Client, project string, opts ...Option) *GitLab {
	baseURL := strings.TrimSuffix(client.BaseURL().String(), "/api/v4/")

	return &GitLab{
		client:                client,
		projectID:             project,
		repoURL:               baseURL + "/" + project,
		maxConcurrentRequests: newConcurrencyConfig(opts).maxConcurrentRequests,
	}
}

func (g *GitLab) RepoURL() string {
	return g.repoURL
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
