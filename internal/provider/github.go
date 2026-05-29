package provider

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-github/v88/github"
)

const (
	gitHubFallbackTaggerName  = "yeet-bot"
	gitHubFallbackTaggerEmail = "noreply@yeet.dev"
)

var _ Provider = (*GitHub)(nil)

type GitHub struct {
	client  *github.Client
	repo    RepoInfo
	baseURL string

	taggerOnce  sync.Once
	taggerName  string
	taggerEmail string
}

func NewGitHub(client *github.Client, owner, repo string) *GitHub {
	baseURL := strings.TrimSuffix(client.BaseURL(), "/")

	// Default github.com API uses api.github.com. Enterprise uses <host>/api/v3.
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

func (g *GitHub) CompareURL(fromRef, toRef string) string {
	return fmt.Sprintf("%s/compare/%s...%s", g.RepoURL(), fromRef, toRef)
}

func gitHubNextPage(resp *github.Response) int {
	if resp == nil {
		return 0
	}

	return resp.NextPage
}
