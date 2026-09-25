package provider

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/google/go-github/v92/github"
	"github.com/monkescience/yeet/internal/forge"
)

const (
	gitHubFallbackTaggerName  = "yeet-bot"
	gitHubFallbackTaggerEmail = "noreply@yeet.dev"
	gitHubPageSize            = 100
)

var _ forge.Provider = (*GitHub)(nil)

type GitHub struct {
	logger        *slog.Logger
	client        *github.Client
	repo          repoInfo
	baseURL       string
	graphqlURL    string
	polling       mergePolling
	releaseBranch string

	taggerOnce  sync.Once
	taggerName  string
	taggerEmail string
	labels      labelDefinitionCache
}

func NewGitHub(
	logger *slog.Logger,
	client *github.Client,
	owner, repo string,
	options ...MergePollingOption,
) *GitHub {
	apiBaseURL := strings.TrimSuffix(client.BaseURL(), "/")
	baseURL := apiBaseURL
	graphqlURL := apiBaseURL + "/graphql"

	switch {
	case baseURL == "https://api.github.com":
		baseURL = "https://github.com"
	case strings.HasSuffix(baseURL, "/api/v3"):
		enterpriseBaseURL, _ := strings.CutSuffix(baseURL, "/api/v3")
		graphqlURL = enterpriseBaseURL + "/api/graphql"
		baseURL = enterpriseBaseURL
	}

	return &GitHub{
		logger:     providerLogger(logger, providerNameGitHub),
		client:     client,
		repo:       repoInfo{Owner: owner, Name: repo},
		baseURL:    baseURL,
		graphqlURL: graphqlURL,
		polling:    newMergePolling(options...),
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
