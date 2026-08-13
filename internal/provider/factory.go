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
	tokenEnvVars  []string
	urlEnvVar     string
	apiPathSuffix string
	defaultHost   string
	construct     func(
		forgeSpec,
		*repositoryDescriptor,
		forgeToken,
		*retryablehttp.Client,
		providerSettings,
	) (forge.Provider, error)
}

// forgeToken records which environment variable supplied the token, because
// Azure DevOps authenticates a pipeline system token differently from a PAT.
type forgeToken struct {
	envVar string
	value  string
}

var forgeSpecs = map[string]forgeSpec{
	providerNameGitHub: {
		tokenEnvVars:  []string{"GITHUB_TOKEN", "GH_TOKEN"},
		urlEnvVar:     githubURLEnv,
		apiPathSuffix: "/api/v3/",
		defaultHost:   DefaultGitHubHost,
		construct:     newGitHubProvider,
	},
	providerNameGitLab: {
		tokenEnvVars:  []string{"GITLAB_TOKEN", "GL_TOKEN"},
		urlEnvVar:     gitlabURLEnv,
		apiPathSuffix: "/api/v4",
		defaultHost:   DefaultGitLabHost,
		construct:     newGitLabProvider,
	},
	providerNameAzureDevOps: {
		tokenEnvVars: []string{azureDevOpsSystemAccessTokenEnv, "AZURE_DEVOPS_EXT_PAT"},
		urlEnvVar:    azureURLEnv,
		defaultHost:  DefaultAzureDevOpsHost,
		construct:    newAzureDevOpsProvider,
	},
}

func create(repository *repositoryDescriptor) (forge.Provider, error) {
	return createProvider(repository, newTracedRetryableClient)
}

func createConfigured(repository *repositoryDescriptor, settings providerSettings) (forge.Provider, error) {
	network := config.Default().Network
	if settings.network != nil {
		network = *settings.network
	}

	return createProviderConfigured(repository, settings, func(forge string) *retryablehttp.Client {
		return newTracedRetryableClientWithConfig(forge, network)
	})
}

func createProvider(
	repository *repositoryDescriptor,
	newHTTPClient func(forge string) *retryablehttp.Client,
) (forge.Provider, error) {
	return createProviderConfigured(repository, providerSettings{}, newHTTPClient)
}

func createProviderConfigured(
	repository *repositoryDescriptor,
	settings providerSettings,
	newHTTPClient func(forge string) *retryablehttp.Client,
) (forge.Provider, error) {
	spec, known := forgeSpecs[repository.Provider]
	if !known {
		if repository.Provider == providerNameAuto {
			return nil, fmt.Errorf(
				"%w: %s (provider auto must be resolved before creation)",
				ErrUnsupportedProvider, repository.Provider,
			)
		}

		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}

	token, err := spec.resolveToken()
	if err != nil {
		return nil, err
	}

	return spec.construct(spec, repository, token, newHTTPClient(repository.Provider), settings)
}

func (spec forgeSpec) resolveToken() (forgeToken, error) {
	for _, envVar := range spec.tokenEnvVars {
		if value := os.Getenv(envVar); value != "" {
			return forgeToken{envVar: envVar, value: value}, nil
		}
	}

	return forgeToken{}, fmt.Errorf(
		"%w: %s environment variable is required",
		ErrMissingToken,
		strings.Join(spec.tokenEnvVars, " or "),
	)
}

func (spec forgeSpec) endpointOverride() string {
	return strings.TrimSpace(os.Getenv(spec.urlEnvVar))
}

func (spec forgeSpec) apiBaseURL(repository *repositoryDescriptor) string {
	if override := spec.endpointOverride(); override != "" {
		return override
	}

	if repository.APIURL != "" {
		return repository.APIURL
	}

	host := strings.TrimSpace(repository.Host)
	if repository.Provider == providerNameAzureDevOps {
		return "https://" + azureDevOpsAPIHost(host)
	}

	if host == "" || strings.EqualFold(host, spec.defaultHost) {
		return ""
	}

	return "https://" + host + spec.apiPathSuffix
}

func (spec forgeSpec) webBaseURL(repository *repositoryDescriptor) string {
	if repository.WebURL != "" {
		return strings.TrimRight(repository.WebURL, "/")
	}

	if override := spec.endpointOverride(); override != "" {
		return webBaseFromAPIURL(repository.Provider, override)
	}

	host := repository.Host
	if repository.Provider == providerNameAzureDevOps {
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
	repository *repositoryDescriptor,
	token forgeToken,
	httpClient *retryablehttp.Client,
	settings providerSettings,
) (forge.Provider, error) {
	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(httpClient.StandardClient()),
		github.WithAuthToken(token.value),
	}

	if baseURL := spec.apiBaseURL(repository); baseURL != "" {
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
	provider.baseURL = spec.webBaseURL(repository)
	provider.releaseBranch = settings.releaseBranch

	return provider, nil
}

func newGitLabProvider(
	spec forgeSpec,
	repository *repositoryDescriptor,
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

	if baseURL := spec.apiBaseURL(repository); baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token.value, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	provider := NewGitLab(client, repository.Project, configuredMergePollingOptions(settings)...)
	provider.repoURL = spec.webBaseURL(repository) + "/" + repository.Project
	provider.releaseBranch = settings.releaseBranch

	return provider, nil
}

func newAzureDevOpsProvider(
	spec forgeSpec,
	repository *repositoryDescriptor,
	token forgeToken,
	httpClient *retryablehttp.Client,
	settings providerSettings,
) (forge.Provider, error) {
	baseURL := spec.apiBaseURL(repository)
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
		provider.baseURL = spec.webBaseURL(repository)
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
	provider.baseURL = spec.webBaseURL(repository)
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
