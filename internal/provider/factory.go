package provider

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v90/github"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrMissingToken        = errors.New("missing auth token")
)

const (
	githubURLEnv = "GITHUB_URL"
	gitlabURLEnv = "GITLAB_URL"
	azureURLEnv  = "AZURE_DEVOPS_URL"
)

//nolint:gosec // G101: an environment variable name, not a credential
const azureDevOpsSystemAccessTokenEnv = "AZURE_DEVOPS_SYSTEM_ACCESSTOKEN"

type forgeSpec struct {
	providerName  string
	tokenEnvVars  []string
	urlEnvVar     string
	apiPathSuffix string
	defaultHost   string
}

type resolvedRepository interface {
	providerName() string
}

type resolvedGitHubRepository struct {
	Host   string
	APIURL string
	WebURL string
	Owner  string
	Repo   string
}

func (*resolvedGitHubRepository) providerName() string { return providerNameGitHub }

type resolvedGitLabRepository struct {
	Host    string
	APIURL  string
	WebURL  string
	Project string
}

func (*resolvedGitLabRepository) providerName() string { return providerNameGitLab }

type resolvedAzureDevOpsRepository struct {
	Host         string
	APIURL       string
	WebURL       string
	Organization string
	Collection   string
	Project      string
	Repo         string
}

func (*resolvedAzureDevOpsRepository) providerName() string { return providerNameAzureDevOps }

// forgeToken records which environment variable supplied the token, because
// Azure DevOps authenticates a pipeline system token differently from a PAT.
type forgeToken struct {
	envVar string
	value  string
}

var forgeSpecs = map[string]forgeSpec{
	providerNameGitHub: {
		providerName:  providerNameGitHub,
		tokenEnvVars:  []string{"GITHUB_TOKEN", "GH_TOKEN"},
		urlEnvVar:     githubURLEnv,
		apiPathSuffix: "/api/v3/",
		defaultHost:   DefaultGitHubHost,
	},
	providerNameGitLab: {
		providerName:  providerNameGitLab,
		tokenEnvVars:  []string{"GITLAB_TOKEN", "GL_TOKEN"},
		urlEnvVar:     gitlabURLEnv,
		apiPathSuffix: "/api/v4",
		defaultHost:   DefaultGitLabHost,
	},
	providerNameAzureDevOps: {
		providerName: providerNameAzureDevOps,
		tokenEnvVars: []string{azureDevOpsSystemAccessTokenEnv, "AZURE_DEVOPS_EXT_PAT"},
		urlEnvVar:    azureURLEnv,
		defaultHost:  DefaultAzureDevOpsHost,
	},
}

func create(repository resolvedRepository) (forge.Provider, error) {
	return createProvider(repository, newTracedRetryableClient)
}

func createConfigured(repository resolvedRepository, settings providerSettings) (forge.Provider, error) {
	network := config.Default().Network
	if settings.network != nil {
		network = *settings.network
	}

	return createProviderConfigured(repository, settings, func(forge string) *retryablehttp.Client {
		return newTracedRetryableClientWithConfig(forge, network)
	})
}

func createProvider(
	repository resolvedRepository,
	newHTTPClient func(forge string) *retryablehttp.Client,
) (forge.Provider, error) {
	return createProviderConfigured(repository, providerSettings{}, newHTTPClient)
}

func createProviderConfigured(
	repository resolvedRepository,
	settings providerSettings,
	newHTTPClient func(forge string) *retryablehttp.Client,
) (forge.Provider, error) {
	provider := repository.providerName()

	spec, known := forgeSpecs[provider]
	if !known {
		if provider == providerNameAuto {
			return nil, fmt.Errorf(
				"%w: %s (provider auto must be resolved before creation)",
				ErrUnsupportedProvider, provider,
			)
		}

		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}

	token, err := spec.resolveToken()
	if err != nil {
		return nil, err
	}

	httpClient := newHTTPClient(provider)

	switch repository := repository.(type) {
	case *resolvedGitHubRepository:
		return newGitHubProvider(spec, repository, token, httpClient, settings)
	case *resolvedGitLabRepository:
		return newGitLabProvider(spec, repository, token, httpClient, settings)
	case *resolvedAzureDevOpsRepository:
		return newAzureDevOpsProvider(spec, repository, token, httpClient, settings)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
}

func (s forgeSpec) resolveToken() (forgeToken, error) {
	for _, envVar := range s.tokenEnvVars {
		if value := os.Getenv(envVar); value != "" {
			return forgeToken{envVar: envVar, value: value}, nil
		}
	}

	return forgeToken{}, fmt.Errorf(
		"%w: %s environment variable is required",
		ErrMissingToken,
		strings.Join(s.tokenEnvVars, " or "),
	)
}

func (s forgeSpec) endpointOverride() string {
	return strings.TrimSpace(os.Getenv(s.urlEnvVar))
}

func (s forgeSpec) apiBaseURL(apiURL, host string) string {
	if override := s.endpointOverride(); override != "" {
		return override
	}

	if apiURL != "" {
		return apiURL
	}

	host = strings.TrimSpace(host)
	if s.providerName == providerNameAzureDevOps {
		return "https://" + azureDevOpsAPIHost(host)
	}

	if host == "" || strings.EqualFold(host, s.defaultHost) {
		return ""
	}

	return "https://" + host + s.apiPathSuffix
}

func (s forgeSpec) webBaseURL(webURL, host string) string {
	if webURL != "" {
		return strings.TrimRight(webURL, "/")
	}

	if override := s.endpointOverride(); override != "" {
		return webBaseFromAPIURL(s.providerName, override)
	}

	if s.providerName == providerNameAzureDevOps {
		host = azureDevOpsAPIHost(host)
	}

	return "https://" + strings.TrimRight(strings.TrimSpace(host), "/")
}

func webBaseFromAPIURL(providerType, apiURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(apiURL), "/")

	switch providerType {
	case providerNameGitHub:
		return strings.TrimSuffix(baseURL, "/api/v3")
	case providerNameGitLab:
		return strings.TrimSuffix(baseURL, "/api/v4")
	default:
		return baseURL
	}
}

func newGitHubProvider(
	spec forgeSpec,
	repository *resolvedGitHubRepository,
	token forgeToken,
	httpClient *retryablehttp.Client,
	settings providerSettings,
) (forge.Provider, error) {
	standardClient := httpClient.StandardClient()
	standardClient.Timeout = httpClient.HTTPClient.Timeout

	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(standardClient),
		github.WithAuthToken(token.value),
	}

	if baseURL := spec.apiBaseURL(repository.APIURL, repository.Host); baseURL != "" {
		opts = append(opts, github.WithEnterpriseURLs(baseURL, baseURL))
	}

	client, err := github.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("configure github client: %w", err)
	}

	provider := NewGitHub(
		client,
		repository.Owner,
		repository.Repo,
		configuredMergePollingOptions(settings)...,
	)
	provider.baseURL = spec.webBaseURL(repository.WebURL, repository.Host)
	provider.releaseBranch = settings.releaseBranch

	return provider, nil
}

func newGitLabProvider(
	spec forgeSpec,
	repository *resolvedGitLabRepository,
	token forgeToken,
	httpClient *retryablehttp.Client,
	settings providerSettings,
) (forge.Provider, error) {
	// client-go owns its own retryablehttp layer, so it takes the traced inner
	// client and the same bounds rather than a second retrying round tripper.
	opts := []gitlab.ClientOptionFunc{
		gitlab.WithHTTPClient(httpClient.HTTPClient),
		gitlab.WithRequestLogHook(httpClient.RequestLogHook),
		gitlab.WithCustomRetryMax(httpClient.RetryMax),
		gitlab.WithCustomRetryWaitMinMax(httpClient.RetryWaitMin, httpClient.RetryWaitMax),
		gitlab.WithOnlyIdempotentRetries(),
	}

	if baseURL := spec.apiBaseURL(repository.APIURL, repository.Host); baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token.value, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	provider := NewGitLab(client, repository.Project, configuredMergePollingOptions(settings)...)
	provider.repoURL = spec.webBaseURL(repository.WebURL, repository.Host) + "/" + repository.Project
	provider.releaseBranch = settings.releaseBranch

	return provider, nil
}

func newAzureDevOpsProvider(
	spec forgeSpec,
	repository *resolvedAzureDevOpsRepository,
	token forgeToken,
	httpClient *retryablehttp.Client,
	settings providerSettings,
) (forge.Provider, error) {
	baseURL := spec.apiBaseURL(repository.APIURL, repository.Host)
	if baseURL == "" {
		baseURL = "https://" + azureDevOpsAPIHost(repository.Host)
	}

	organization := strings.TrimSpace(repository.Organization)

	collection := strings.TrimSpace(repository.Collection)
	if collection == "" {
		collection = organization
	}

	project := strings.TrimSpace(repository.Project)
	repo := strings.TrimSpace(repository.Repo)
	standardClient := httpClient.StandardClient()
	standardClient.Timeout = httpClient.HTTPClient.Timeout

	if token.envVar == azureDevOpsSystemAccessTokenEnv {
		provider := NewAzureDevOpsWithSystemAccessToken(
			standardClient,
			baseURL,
			token.value,
			organization,
			collection,
			project,
			repo,
			configuredMergePollingOptions(settings)...,
		)
		provider.baseURL = spec.webBaseURL(repository.WebURL, repository.Host)
		provider.releaseBranch = settings.releaseBranch

		return provider, nil
	}

	provider := NewAzureDevOps(
		standardClient,
		baseURL,
		token.value,
		organization,
		collection,
		project,
		repo,
		configuredMergePollingOptions(settings)...,
	)
	provider.baseURL = spec.webBaseURL(repository.WebURL, repository.Host)
	provider.releaseBranch = settings.releaseBranch

	return provider, nil
}

func configuredMergePollingOptions(settings providerSettings) []MergePollingOption {
	if settings.mergePolling == nil {
		return nil
	}

	polling := settings.mergePolling

	return []MergePollingOption{WithMergePolling(
		polling.InitialInterval,
		polling.MaxInterval,
		polling.Timeout,
	)}
}
