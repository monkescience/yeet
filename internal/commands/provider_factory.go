package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/go-github/v88/github"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	ErrUnsupportedProvider     = errors.New("unsupported provider")
	ErrMissingToken            = errors.New("missing auth token")
	ErrGitHubRepoRequired      = errors.New("resolve github repository: owner and repo are required")
	ErrGitHubOwnerInvalid      = errors.New("resolve github repository: owner must not contain '/'")
	ErrGitLabProjectNeeded     = errors.New("resolve gitlab repository: project or owner/repo are required")
	ErrAzureDevOpsCoordsNeeded = errors.New("resolve azuredevops repository: organization, project, and repo are required")
	ErrRepositoryConflict      = errors.New("resolve repository: project does not match owner/repo")
	ErrInvalidHost             = errors.New("invalid provider host")
	ErrUntrustedHost           = errors.New("provider host is not trusted")
)

const (
	githubURLEnv = "GITHUB_URL"
	gitlabURLEnv = "GITLAB_URL"
	azureURLEnv  = "AZURE_DEVOPS_URL"
)

var ErrInvalidMaxConcurrentRequests = errors.New("invalid " + maxConcurrentRequestsEnv)

const (
	httpClientTimeout = 30 * time.Second
	httpRetryMax      = 3
	httpRetryWaitMin  = 1 * time.Second
	httpRetryWaitMax  = 10 * time.Second
)

const maxConcurrentRequestsEnv = "YEET_MAX_CONCURRENT_REQUESTS"

// providerConcurrencyOptions reads the optional YEET_MAX_CONCURRENT_REQUESTS
// override. An unset or empty value keeps the provider default. A value that is
// not a positive integer is a user error and fails the run.
func providerConcurrencyOptions() ([]provider.Option, error) {
	raw := strings.TrimSpace(os.Getenv(maxConcurrentRequestsEnv))
	if raw == "" {
		return nil, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return nil, fmt.Errorf("%w: must be a positive integer, got %q", ErrInvalidMaxConcurrentRequests, raw)
	}

	return []provider.Option{provider.WithMaxConcurrentRequests(limit)}, nil
}

type gitRemoteURLGetter func(context.Context, string) (string, error)

func newRetryableHTTPClient() *http.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = httpRetryMax
	client.RetryWaitMin = httpRetryWaitMin
	client.RetryWaitMax = httpRetryWaitMax
	client.Logger = nil

	client.HTTPClient.Timeout = httpClientTimeout

	return client.StandardClient()
}

func createProvider(repository *provider.RepositoryDescriptor) (provider.Provider, error) {
	opts, err := providerConcurrencyOptions()
	if err != nil {
		return nil, err
	}

	switch config.ProviderType(repository.Provider) {
	case config.ProviderGitHub:
		return createGitHubProvider(repository, opts...)
	case config.ProviderGitLab:
		return createGitLabProvider(repository, opts...)
	case config.ProviderAzureDevOps:
		return createAzureDevOpsProvider(repository, opts...)
	case config.ProviderAuto:
		return nil, fmt.Errorf(
			"%w: %s (provider auto must be resolved before creation)",
			ErrUnsupportedProvider, repository.Provider,
		)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}
}

func createGitHubProvider(
	repository *provider.RepositoryDescriptor,
	providerOpts ...provider.Option,
) (*provider.GitHub, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITHUB_TOKEN or GH_TOKEN environment variable is required", ErrMissingToken)
	}

	baseURL := strings.TrimSpace(os.Getenv(githubURLEnv))

	if baseURL == "" {
		host := strings.TrimSpace(repository.Host)
		if host != "" && !strings.EqualFold(host, provider.DefaultGitHubHost) {
			baseURL = fmt.Sprintf("https://%s/api/v3/", host)
		}
	}

	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(newRetryableHTTPClient()),
		github.WithAuthToken(token),
	}
	if baseURL != "" {
		opts = append(opts, github.WithEnterpriseURLs(baseURL, baseURL))
	}

	client, err := github.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("configure github client: %w", err)
	}

	return provider.NewGitHub(client, repository.Owner, repository.Repo, providerOpts...), nil
}

func createGitLabProvider(
	repository *provider.RepositoryDescriptor,
	providerOpts ...provider.Option,
) (*provider.GitLab, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		token = os.Getenv("GL_TOKEN")
	}

	if token == "" {
		return nil, fmt.Errorf("%w: GITLAB_TOKEN or GL_TOKEN environment variable is required", ErrMissingToken)
	}

	baseURL := strings.TrimSpace(os.Getenv(gitlabURLEnv))

	if baseURL == "" {
		host := strings.TrimSpace(repository.Host)
		if host != "" && !strings.EqualFold(host, provider.DefaultGitLabHost) {
			baseURL = fmt.Sprintf("https://%s/api/v4", host)
		}
	}

	var opts []gitlab.ClientOptionFunc

	if baseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(baseURL))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	return provider.NewGitLab(client, repository.Project, providerOpts...), nil
}

func createAzureDevOpsProvider(
	repository *provider.RepositoryDescriptor,
	providerOpts ...provider.Option,
) (*provider.AzureDevOps, error) {
	systemAccessToken := os.Getenv("AZURE_DEVOPS_SYSTEM_ACCESSTOKEN")
	pat := os.Getenv("AZURE_DEVOPS_EXT_PAT")

	if systemAccessToken == "" && pat == "" {
		return nil, fmt.Errorf(
			"%w: AZURE_DEVOPS_SYSTEM_ACCESSTOKEN or AZURE_DEVOPS_EXT_PAT environment variable is required",
			ErrMissingToken,
		)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(azureURLEnv)), "/")

	host := strings.TrimSpace(repository.Host)
	if host != "" && baseURL == "" {
		baseURL = "https://" + host
	}

	if baseURL == "" {
		baseURL = "https://" + provider.DefaultAzureDevOpsHost
	}

	collection := strings.TrimSpace(repository.Collection)
	if collection == "" {
		collection = strings.TrimSpace(repository.Organization)
	}

	organization := strings.TrimSpace(repository.Organization)
	project := strings.TrimSpace(repository.Project)
	repo := strings.TrimSpace(repository.Repo)
	httpClient := newRetryableHTTPClient()

	if systemAccessToken != "" {
		return provider.NewAzureDevOpsWithSystemAccessToken(
			httpClient,
			baseURL,
			systemAccessToken,
			organization,
			collection,
			project,
			repo,
			providerOpts...,
		), nil
	}

	return provider.NewAzureDevOps(
		httpClient,
		baseURL,
		pat,
		organization,
		collection,
		project,
		repo,
		providerOpts...,
	), nil
}

//nolint:funlen // Repository resolution centralizes per-provider defaulting and validation.
func resolveRepository(
	ctx context.Context,
	cfg *config.Config,
	getRemoteURL gitRemoteURLGetter,
) (*provider.RepositoryDescriptor, error) {
	repository := repositoryFromConfig(cfg)
	if repository.Remote == "" {
		repository.Remote = "origin"
	}

	if needsRemoteLookup(repository) {
		remoteURL, err := getRemoteURL(ctx, repository.Remote)
		if err != nil {
			return nil, fmt.Errorf("get git remote %q url: %w", repository.Remote, err)
		}

		detected, err := provider.ParseRemote(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse git remote %q url: %w", repository.Remote, err)
		}

		detected.Remote = repository.Remote
		repository = mergeRepositoryDescriptor(detected, repository)
	}

	normalizeRepositoryDescriptor(repository)

	if repository.Provider == "" {
		providerType, err := provider.DetectType(repository.Host)
		if err != nil {
			return nil, unsupportedAutoProviderError(repository.Host, err)
		}

		repository.Provider = providerType
	}

	switch config.ProviderType(repository.Provider) {
	case config.ProviderGitHub:
		if repository.Host == "" {
			repository.Host = provider.DefaultGitHubHost
		}
	case config.ProviderGitLab:
		if repository.Host == "" {
			repository.Host = provider.DefaultGitLabHost
		}
	case config.ProviderAzureDevOps:
		if repository.Host == "" {
			repository.Host = provider.DefaultAzureDevOpsHost
		}

		if repository.Collection == "" {
			repository.Collection = repository.Organization
		}
	case config.ProviderAuto:
		// auto is resolved via remote detection. no default host needed.
	}

	normalizeRepositoryDescriptor(repository)

	if err := validateRepositoryDescriptor(repository); err != nil {
		return nil, err
	}

	if err := validateProviderHostTrust(ctx, repository, getRemoteURL); err != nil {
		return nil, err
	}

	return repository, nil
}

// validateProviderHostTrust ensures the host that will receive the auth token is
// one the operator controls, not one an attacker set through the repo-controlled
// config. An operator-supplied *_URL env var or a known public host is trusted
// outright. Any other host must match the git remote the repo is cloned from,
// which lives in .git/config (set by CI, not the committed tree).
func validateProviderHostTrust(
	ctx context.Context,
	repository *provider.RepositoryDescriptor,
	getRemoteURL gitRemoteURLGetter,
) error {
	host := strings.TrimSpace(repository.Host)
	if err := validateHostFormat(host); err != nil {
		return err
	}

	if providerURLEnvSet(repository.Provider) {
		return nil
	}

	if _, err := provider.DetectType(host); err == nil {
		return nil
	}

	remoteURL, err := getRemoteURL(ctx, repository.Remote)
	if err != nil {
		return fmt.Errorf(
			"%w: %q could not be verified against git remote %q: %s",
			ErrUntrustedHost, host, repository.Remote, err.Error(),
		)
	}

	detected, err := provider.ParseRemote(remoteURL)
	if err != nil {
		return fmt.Errorf(
			"%w: %q could not be verified against git remote %q: %s",
			ErrUntrustedHost, host, repository.Remote, err.Error(),
		)
	}

	if !strings.EqualFold(strings.TrimSpace(detected.Host), host) {
		return fmt.Errorf("%w: %q does not match git remote host %q", ErrUntrustedHost, host, detected.Host)
	}

	return nil
}

func validateHostFormat(host string) error {
	if host == "" {
		return fmt.Errorf("%w: host must not be empty", ErrInvalidHost)
	}

	for _, r := range host {
		if r == '/' || r == '@' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%w: %q must be a bare hostname without scheme, credentials, or path", ErrInvalidHost, host)
		}
	}

	return nil
}

func providerURLEnvSet(providerType string) bool {
	switch config.ProviderType(providerType) {
	case config.ProviderGitHub:
		return strings.TrimSpace(os.Getenv(githubURLEnv)) != ""
	case config.ProviderGitLab:
		return strings.TrimSpace(os.Getenv(gitlabURLEnv)) != ""
	case config.ProviderAzureDevOps:
		return strings.TrimSpace(os.Getenv(azureURLEnv)) != ""
	case config.ProviderAuto:
		return false
	default:
		return false
	}
}

func unsupportedAutoProviderError(host string, err error) error {
	return fmt.Errorf(
		"resolve repository provider for host %q: %w; "+
			"auto-detection only supports github.com, gitlab.com, and dev.azure.com; "+
			"set provider, [repository], or pass explicit flags for custom domains",
		host,
		err,
	)
}

func repositoryFromConfig(cfg *config.Config) *provider.RepositoryDescriptor {
	descriptor := &provider.RepositoryDescriptor{
		Provider: normalizedRepositoryProvider(cfg.Provider),
		Remote:   strings.TrimSpace(cfg.Repository.Remote),
	}

	switch cfg.Provider {
	case config.ProviderGitHub:
		if cfg.Repository.GitHub == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.GitHub.Host)
		descriptor.Owner = strings.TrimSpace(cfg.Repository.GitHub.Owner)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.GitHub.Repo)
		descriptor.Project = strings.TrimSpace(cfg.Repository.GitHub.Project)
	case config.ProviderGitLab:
		if cfg.Repository.GitLab != nil {
			descriptor.Host = strings.TrimSpace(cfg.Repository.GitLab.Host)
			descriptor.Project = strings.TrimSpace(cfg.Repository.GitLab.Project)
		}
	case config.ProviderAzureDevOps:
		if cfg.Repository.AzureDevOps == nil {
			break
		}

		descriptor.Host = strings.TrimSpace(cfg.Repository.AzureDevOps.Host)
		descriptor.Organization = strings.TrimSpace(cfg.Repository.AzureDevOps.Organization)
		descriptor.Project = strings.TrimSpace(cfg.Repository.AzureDevOps.Project)
		descriptor.Repo = strings.TrimSpace(cfg.Repository.AzureDevOps.Repo)
		descriptor.Collection = strings.TrimSpace(cfg.Repository.AzureDevOps.Collection)
	case config.ProviderAuto:
	}

	return descriptor
}

func normalizedRepositoryProvider(providerType config.ProviderType) string {
	provider := strings.TrimSpace(string(providerType))
	if config.ProviderType(provider) == config.ProviderAuto {
		return ""
	}

	return provider
}

func needsRemoteLookup(repository *provider.RepositoryDescriptor) bool {
	if !hasRepositoryCoordinates(repository) {
		return true
	}

	return repository.Provider == "" && repository.Host == ""
}

func hasRepositoryCoordinates(repository *provider.RepositoryDescriptor) bool {
	return repository.Project != "" || (repository.Owner != "" && repository.Repo != "")
}

func mergeRepositoryDescriptor(
	base *provider.RepositoryDescriptor,
	override *provider.RepositoryDescriptor,
) *provider.RepositoryDescriptor {
	if override.Provider != "" {
		base.Provider = override.Provider
	}

	if override.Host != "" {
		base.Host = override.Host
	}

	mergeRepositoryCoordinates(base, override)

	if override.Organization != "" {
		base.Organization = override.Organization
	}

	if override.Collection != "" {
		base.Collection = override.Collection
	}

	if override.Remote != "" {
		base.Remote = override.Remote
	}

	return base
}

func mergeRepositoryCoordinates(base *provider.RepositoryDescriptor, override *provider.RepositoryDescriptor) {
	switch {
	case override.Project != "":
		base.Project = override.Project
		base.Owner = override.Owner
		base.Repo = override.Repo
	case override.Owner != "" && override.Repo != "":
		base.Owner = override.Owner
		base.Repo = override.Repo
		base.Project = ""
	default:
		if override.Owner != "" {
			base.Owner = override.Owner
		}

		if override.Repo != "" {
			base.Repo = override.Repo
		}
	}
}

func normalizeRepositoryDescriptor(repository *provider.RepositoryDescriptor) {
	repository.Provider = strings.TrimSpace(repository.Provider)
	repository.Host = strings.TrimSpace(repository.Host)
	repository.Owner = strings.TrimSpace(repository.Owner)
	repository.Repo = strings.TrimSpace(repository.Repo)
	repository.Project = strings.Trim(strings.TrimSpace(repository.Project), "/")
	repository.Organization = strings.TrimSpace(repository.Organization)
	repository.Collection = strings.TrimSpace(repository.Collection)
	repository.Remote = strings.TrimSpace(repository.Remote)

	if repository.Project == "" && repository.Owner != "" && repository.Repo != "" {
		repository.Project = repository.Owner + "/" + repository.Repo
	}

	if repository.Project == "" || (repository.Owner != "" && repository.Repo != "") {
		return
	}

	owner, repo := provider.SplitProjectPath(repository.Project)
	if repository.Owner == "" {
		repository.Owner = owner
	}

	if repository.Repo == "" {
		repository.Repo = repo
	}
}

func validateRepositoryDescriptor(repository *provider.RepositoryDescriptor) error {
	if err := validateRepositoryCoordinates(repository); err != nil {
		return err
	}

	switch config.ProviderType(repository.Provider) {
	case config.ProviderGitHub:
		if repository.Owner == "" || repository.Repo == "" {
			return ErrGitHubRepoRequired
		}

		if strings.Contains(repository.Owner, "/") {
			return fmt.Errorf("%w: %q", ErrGitHubOwnerInvalid, repository.Owner)
		}
	case config.ProviderGitLab:
		if repository.Project == "" {
			return ErrGitLabProjectNeeded
		}
	case config.ProviderAzureDevOps:
		if repository.Organization == "" || repository.Project == "" || repository.Repo == "" {
			return ErrAzureDevOpsCoordsNeeded
		}
	case config.ProviderAuto:
		return fmt.Errorf(
			"%w: %s (provider auto must be resolved before validation)",
			ErrUnsupportedProvider, repository.Provider,
		)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, repository.Provider)
	}

	return nil
}

func validateRepositoryCoordinates(repository *provider.RepositoryDescriptor) error {
	if repository.Project == "" || repository.Owner == "" || repository.Repo == "" {
		return nil
	}

	expectedProject := repository.Owner + "/" + repository.Repo
	if repository.Project == expectedProject {
		return nil
	}

	return fmt.Errorf(
		"%w: project %q does not match owner/repo %q",
		ErrRepositoryConflict,
		repository.Project,
		expectedProject,
	)
}
