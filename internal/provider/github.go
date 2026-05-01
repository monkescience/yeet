package provider

import (
	"fmt"
	"strings"

	"github.com/google/go-github/v84/github"
)

type GitHub struct {
	client  *github.Client
	repo    RepoInfo
	baseURL string
}

func NewGitHub(client *github.Client, owner, repo string) *GitHub {
	baseURL := strings.TrimSuffix(client.BaseURL.String(), "/")

	// Default github.com API uses api.github.com; enterprise uses <host>/api/v3.
	if baseURL == "https://api.github.com" {
		baseURL = "https://github.com"
	} else {
		baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	}

	return &GitHub{
		client:  client,
		repo:    RepoInfo{Owner: owner, Name: repo},
		baseURL: baseURL,
	}
}

func (g *GitHub) RepoURL() string {
	return fmt.Sprintf("%s/%s/%s", g.baseURL, g.repo.Owner, g.repo.Name)
}

func (g *GitHub) PathPrefix() string {
	return ""
}

func gitHubNextPage(resp *github.Response) int {
	if resp == nil {
		return 0
	}

	return resp.NextPage
}
