package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/google/go-github/v90/github"
	"github.com/monkescience/yeet/internal/forge"
)

const (
	gitHubFallbackTaggerName  = "yeet-bot"
	gitHubFallbackTaggerEmail = "noreply@yeet.dev"
	gitHubPageSize            = 100
)

var _ forge.Provider = (*GitHub)(nil)

type GitHub struct {
	client  *github.Client
	repo    repoInfo
	baseURL string
	polling mergePolling

	taggerOnce  sync.Once
	taggerName  string
	taggerEmail string
}

func NewGitHub(client *github.Client, owner, repo string, options ...MergePollingOption) *GitHub {
	baseURL := strings.TrimSuffix(client.BaseURL(), "/")

	// Default github.com API uses api.github.com. Enterprise uses <host>/api/v3.
	if baseURL == "https://api.github.com" {
		baseURL = "https://github.com"
	} else {
		baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	}

	return &GitHub{
		client:  client,
		repo:    repoInfo{Owner: owner, Name: repo},
		baseURL: baseURL,
		polling: newMergePolling(options...),
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

func isGitHubNotFound(err error) bool {
	var errorResponse *github.ErrorResponse
	if !errors.As(err, &errorResponse) || errorResponse.Response == nil {
		return false
	}

	return errorResponse.Response.StatusCode == http.StatusNotFound
}

func gitHubNextPage(resp *github.Response) int {
	if resp == nil {
		return 0
	}

	return resp.NextPage
}
