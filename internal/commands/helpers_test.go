package commands //nolint:testpackage // validates unexported repository helpers directly

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func TestProviderConcurrencyOptions(t *testing.T) {
	t.Run("returns no options when the env var is unset", func(t *testing.T) {
		// given: the override env var is empty
		t.Setenv(maxConcurrentRequestsEnv, "")

		// when: the concurrency options are resolved
		opts, err := providerConcurrencyOptions()

		// then: no option is produced and the provider default applies
		testastic.NoError(t, err)
		testastic.Equal(t, 0, len(opts))
	})

	t.Run("returns one option for a positive integer", func(t *testing.T) {
		// given: a positive override
		t.Setenv(maxConcurrentRequestsEnv, "16")

		// when: the concurrency options are resolved
		opts, err := providerConcurrencyOptions()

		// then: a single override option is produced
		testastic.NoError(t, err)
		testastic.Equal(t, 1, len(opts))
	})

	t.Run("errors on a non-integer value", func(t *testing.T) {
		// given: a non-integer override
		t.Setenv(maxConcurrentRequestsEnv, "abc")

		// when: the concurrency options are resolved
		_, err := providerConcurrencyOptions()

		// then: the invalid-value sentinel is returned
		testastic.ErrorIs(t, err, ErrInvalidMaxConcurrentRequests)
	})

	t.Run("errors on a non-positive value", func(t *testing.T) {
		// given: a non-positive override
		t.Setenv(maxConcurrentRequestsEnv, "0")

		// when: the concurrency options are resolved
		_, err := providerConcurrencyOptions()

		// then: the invalid-value sentinel is returned
		testastic.ErrorIs(t, err, ErrInvalidMaxConcurrentRequests)
	})
}

func TestResolveRepository(t *testing.T) {
	t.Parallel()

	t.Run("verifies an explicit custom host against the git remote", func(t *testing.T) {
		t.Parallel()

		// given: explicit gitlab coordinates on a custom enterprise host
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
			Host:    "gitlab.company.com",
			Project: "group/subgroup/service",
		}

		remoteLookedUp := false

		// when: resolving the repository against a remote on the same host
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				remoteLookedUp = true

				return "https://gitlab.company.com/group/subgroup/service.git", nil
			},
		)

		// then: the custom host is accepted only after matching the git remote
		testastic.NoError(t, err)
		testastic.True(t, remoteLookedUp)
		testastic.Equal(t, "gitlab", repository.Provider)
		testastic.Equal(t, "gitlab.company.com", repository.Host)
		testastic.Equal(t, "group/subgroup", repository.Owner)
		testastic.Equal(t, "service", repository.Repo)
		testastic.Equal(t, "group/subgroup/service", repository.Project)
		testastic.Equal(t, "origin", repository.Remote)
	})

	t.Run("uses explicit github coordinates without git remote access", func(t *testing.T) {
		t.Parallel()

		// given: explicit github coordinates in config
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Owner: "platform",
			Repo:  "yeet",
		}

		remoteLookedUp := false

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				remoteLookedUp = true

				return "", errors.New("git remote lookup should not run")
			},
		)

		// then: explicit github coordinates resolve without git remote access
		testastic.NoError(t, err)
		testastic.False(t, remoteLookedUp)
		testastic.Equal(t, "github", repository.Provider)
		testastic.Equal(t, "github.com", repository.Host)
		testastic.Equal(t, "platform", repository.Owner)
		testastic.Equal(t, "yeet", repository.Repo)
		testastic.Equal(t, "platform/yeet", repository.Project)
	})

	t.Run("uses configured remote name", func(t *testing.T) {
		t.Parallel()

		// given: a config with a non-default remote name
		cfg := config.Default()
		cfg.Repository.Remote = "upstream"

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(_ context.Context, remote string) (string, error) {
				testastic.Equal(t, "upstream", remote)

				return "git@github.com:platform/yeet.git", nil
			},
		)

		// then: the configured remote name is forwarded and coordinates are detected
		testastic.NoError(t, err)
		testastic.Equal(t, "github", repository.Provider)
		testastic.Equal(t, "github.com", repository.Host)
		testastic.Equal(t, "platform", repository.Owner)
		testastic.Equal(t, "yeet", repository.Repo)
		testastic.Equal(t, "platform/yeet", repository.Project)
		testastic.Equal(t, "upstream", repository.Remote)
	})

	t.Run("fails on unsupported host without explicit provider", func(t *testing.T) {
		t.Parallel()

		// given: a default config and a remote on an unknown host
		cfg := config.Default()

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@code.company.com:team/service.git", nil
			},
		)

		// then: resolution reports an unsupported host with remediation
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
		testastic.ErrorContains(t, err, "set provider, [repository], or pass explicit flags")
	})

	t.Run("fails on github custom host without explicit provider", func(t *testing.T) {
		t.Parallel()

		// given: a default config and a remote on a custom github host
		cfg := config.Default()

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.company.com:platform/yeet.git", nil
			},
		)

		// then: github custom hosts require an explicit provider override
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
		testastic.ErrorContains(t, err, "set provider, [repository], or pass explicit flags")
	})

	t.Run("honors explicit provider on unknown host", func(t *testing.T) {
		t.Parallel()

		// given: an explicit gitlab provider and a remote on an unknown host
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@code.company.com:group/subgroup/service.git", nil
			},
		)

		// then: the explicit provider is used and coordinates are detected from the remote
		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", repository.Provider)
		testastic.Equal(t, "code.company.com", repository.Host)
		testastic.Equal(t, "group/subgroup", repository.Owner)
		testastic.Equal(t, "service", repository.Repo)
		testastic.Equal(t, "group/subgroup/service", repository.Project)
	})

	t.Run("explicit coordinates override remote coordinates", func(t *testing.T) {
		t.Parallel()

		// given: explicit github coordinates while the remote points to a different repo
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Owner: "platform",
			Repo:  "yeet",
		}

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.com:other/repo.git", nil
			},
		)

		// then: explicit config wins over remote detection
		testastic.NoError(t, err)
		testastic.Equal(t, "github", repository.Provider)
		testastic.Equal(t, "platform", repository.Owner)
		testastic.Equal(t, "yeet", repository.Repo)
		testastic.Equal(t, "platform/yeet", repository.Project)
	})
}

func TestResolveConfigPath(t *testing.T) {
	t.Run("finds nearest ancestor config from nested directory", func(t *testing.T) {
		// given: nested directories with multiple ancestor config files
		repositoryPath := t.TempDir()
		rootConfigPath := filepath.Join(repositoryPath, config.DefaultFile)
		appsPath := filepath.Join(repositoryPath, "apps")
		servicePath := filepath.Join(appsPath, "service")

		err := os.WriteFile(rootConfigPath, []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)

		err = os.MkdirAll(servicePath, 0o755)
		testastic.NoError(t, err)

		appsConfigPath := filepath.Join(appsPath, config.DefaultFile)
		err = os.WriteFile(appsConfigPath, []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)
		t.Chdir(servicePath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolveConfigPath(context.Background(), "")

		// then: the nearest ancestor config is selected
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, appsConfigPath, resolvedPath)
	})

	t.Run("explicit path bypasses ancestor discovery", func(t *testing.T) {
		// given: a nested directory with an ancestor config file
		repositoryPath := t.TempDir()
		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)

		err = os.WriteFile(filepath.Join(repositoryPath, config.DefaultFile), []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving an explicit config path
		resolvedPath, resolveErr := resolveConfigPath(context.Background(), " custom.yaml ")

		// then: the explicit path is used as-is after trimming
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, "custom.yaml", resolvedPath)
	})

	t.Run("missing default config reports the default filename", func(t *testing.T) {
		// given: a directory tree without any yeet config file
		repositoryPath := t.TempDir()
		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolveConfigPath(context.Background(), "")

		// then: the missing path is reported against the default filename
		testastic.Equal(t, config.DefaultFile, resolvedPath)
		testastic.Error(t, resolveErr)
		testastic.ErrorIs(t, resolveErr, os.ErrNotExist)
	})

	t.Run("does not escape the repository root", func(t *testing.T) {
		// given: a git repository nested under a parent directory with an unrelated config file
		workspacePath := t.TempDir()
		parentConfigPath := filepath.Join(workspacePath, config.DefaultFile)
		err := os.WriteFile(parentConfigPath, []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)

		repositoryPath := filepath.Join(workspacePath, "service")
		_, err = git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolveConfigPath(context.Background(), "")

		// then: discovery stops at the repo root instead of using the parent config
		testastic.Equal(t, config.DefaultFile, resolvedPath)
		testastic.Error(t, resolveErr)
		testastic.ErrorIs(t, resolveErr, os.ErrNotExist)
	})
}

func TestResolveInitConfigPath(t *testing.T) {
	t.Run("targets repository root from nested directory", func(t *testing.T) {
		// given: a nested directory inside a git repository without an existing config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitConfigPath(context.Background(), "")

		// then: init targets the repository root config path
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, filepath.Join(repositoryPath, config.DefaultFile), resolvedPath)
	})

	t.Run("reuses existing ancestor config path", func(t *testing.T) {
		// given: a nested directory below an existing root config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		configPath := filepath.Join(repositoryPath, config.DefaultFile)
		err = os.WriteFile(configPath, []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitConfigPath(context.Background(), "")

		// then: init points at the existing ancestor config file
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, configPath, resolvedPath)
	})

	t.Run("falls back to current directory outside git repositories", func(t *testing.T) {
		// given: a nested directory tree outside any git repository
		workspacePath := t.TempDir()
		nestedPath := filepath.Join(workspacePath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitConfigPath(context.Background(), "")

		// then: init falls back to the local default filename
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, config.DefaultFile, resolvedPath)
	})

	t.Run("does not reuse config outside the repository root", func(t *testing.T) {
		// given: a git repository nested under a parent directory with an unrelated config file
		workspacePath := t.TempDir()
		err := os.WriteFile(filepath.Join(workspacePath, config.DefaultFile), []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)

		repositoryPath := filepath.Join(workspacePath, "service")
		_, err = git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default init destination
		resolvedPath, resolveErr := resolveInitConfigPath(context.Background(), "")

		// then: init still targets the repo root config path
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, filepath.Join(repositoryPath, config.DefaultFile), resolvedPath)
	})
}

func TestNewRetryableHTTPClient(t *testing.T) {
	t.Parallel()

	// when: constructing the retryable HTTP client
	httpClient := newRetryableHTTPClient()

	// then: the client is configured with the package-level retry and timeout constants
	roundTripper, ok := httpClient.Transport.(*retryablehttp.RoundTripper)
	testastic.True(t, ok)
	testastic.Equal(t, httpRetryMax, roundTripper.Client.RetryMax)
	testastic.Equal(t, httpRetryWaitMin, roundTripper.Client.RetryWaitMin)
	testastic.Equal(t, httpRetryWaitMax, roundTripper.Client.RetryWaitMax)
	testastic.Equal(t, httpClientTimeout, roundTripper.Client.HTTPClient.Timeout)
	testastic.True(t, roundTripper.Client.Logger == nil)
}

func TestCreateGitHubProviderPrefersGitHubURLOverRepositoryHost(t *testing.T) {
	// given: a custom github enterprise host and an explicit GITHUB_URL
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://ghe-proxy.example/api/v3/")

	// when: creating the github provider
	githubProvider, err := createGitHubProvider(&provider.RepositoryDescriptor{
		Host:  "github.company.com",
		Owner: "platform",
		Repo:  "yeet",
	})

	// then: the explicit GITHUB_URL wins over the URL derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://ghe-proxy.example/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderDerivesURLFromRepositoryHost(t *testing.T) {
	// given: a custom github enterprise host and no GITHUB_URL
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "")

	// when: creating the github provider
	githubProvider, err := createGitHubProvider(&provider.RepositoryDescriptor{
		Host:  "github.company.com",
		Owner: "platform",
		Repo:  "yeet",
	})

	// then: the API URL is derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.company.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitLabProviderPrefersGitLabURLOverRepositoryHost(t *testing.T) {
	// given: a custom gitlab host and an explicit GITLAB_URL
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://gitlab-proxy.example/api/v4")

	// when: creating the gitlab provider
	gitlabProvider, err := createGitLabProvider(&provider.RepositoryDescriptor{
		Host:    "gitlab.company.com",
		Project: "group/subgroup/service",
	})

	// then: the explicit GITLAB_URL wins over the URL derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab-proxy.example/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitLabProviderDerivesURLFromRepositoryHost(t *testing.T) {
	// given: a custom gitlab host and no GITLAB_URL
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "")

	// when: creating the gitlab provider
	gitlabProvider, err := createGitLabProvider(&provider.RepositoryDescriptor{
		Host:    "gitlab.company.com",
		Project: "group/subgroup/service",
	})

	// then: the API URL is derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab.company.com/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitHubProviderHonorsGitHubURLOnDefaultHost(t *testing.T) {
	// given: the default github host with GITHUB_URL overriding the API endpoint
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://example.test/api/v3/")

	// when: creating the github provider
	githubProvider, err := createGitHubProvider(&provider.RepositoryDescriptor{
		Host:  provider.DefaultGitHubHost,
		Owner: "platform",
		Repo:  "yeet",
	})

	// then: GITHUB_URL is honored on the default host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://example.test/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitLabProviderHonorsGitLabURLOnDefaultHost(t *testing.T) {
	// given: the default gitlab host with GITLAB_URL overriding the API endpoint
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://example.test/api/v4")

	// when: creating the gitlab provider
	gitlabProvider, err := createGitLabProvider(&provider.RepositoryDescriptor{
		Host:    provider.DefaultGitLabHost,
		Project: "group/subgroup/service",
	})

	// then: GITLAB_URL is honored on the default host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://example.test/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesNativePATEnv(t *testing.T) {
	// given: a PAT supplied via AZURE_DEVOPS_EXT_PAT
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createAzureDevOpsProvider(&provider.RepositoryDescriptor{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	})

	// then: the provider is constructed with the configured repository URL
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/platform/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderNormalizesLegacyHost(t *testing.T) {
	// given: configured legacy Azure DevOps coordinates and no endpoint override
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
	t.Setenv(azureURLEnv, "")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createAzureDevOpsProvider(&provider.RepositoryDescriptor{
		Host:         "contoso.visualstudio.com",
		Organization: "contoso",
		Project:      "release-tools",
		Repo:         "yeet",
	})

	// then: the legacy host is normalized before the organization is appended
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/contoso/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesNativeSystemAccessTokenEnv(t *testing.T) {
	// given: a token supplied via AZURE_DEVOPS_SYSTEM_ACCESSTOKEN
	t.Setenv("AZURE_DEVOPS_SYSTEM_ACCESSTOKEN", "test-token")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createAzureDevOpsProvider(&provider.RepositoryDescriptor{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	})

	// then: the provider is constructed with the configured repository URL
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/platform/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderReportsNativeTokenNames(t *testing.T) {
	// given: no Azure DevOps token in the environment
	t.Setenv("AZURE_DEVOPS_SYSTEM_ACCESSTOKEN", "")
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "")

	// when: creating the Azure DevOps provider
	_, err := createAzureDevOpsProvider(&provider.RepositoryDescriptor{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	})

	// then: the error names both supported environment variables
	testastic.Error(t, err)
	testastic.ErrorContains(t, err, "AZURE_DEVOPS_SYSTEM_ACCESSTOKEN or AZURE_DEVOPS_EXT_PAT")
}

func TestGetGitRemoteURL(t *testing.T) {
	t.Run("reads origin url from repository root", func(t *testing.T) {
		// given: a repository with an origin remote
		repositoryPath := t.TempDir()
		initializeRepositoryWithRemote(t, repositoryPath, "origin", "git@github.com:platform/yeet.git")
		t.Chdir(repositoryPath)

		// when: reading the remote URL
		remoteURL, err := getGitRemoteURL(context.Background(), "origin")

		// then: the configured URL is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "git@github.com:platform/yeet.git", remoteURL)
	})

	t.Run("detects repository from nested directory", func(t *testing.T) {
		// given: a nested directory inside a repository with a custom remote
		repositoryPath := t.TempDir()
		initializeRepositoryWithRemote(t, repositoryPath, "upstream", "git@gitlab.com:group/subgroup/service.git")

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: reading the custom remote URL
		remoteURL, getErr := getGitRemoteURL(context.Background(), "upstream")

		// then: the repository is discovered automatically
		testastic.NoError(t, getErr)
		testastic.Equal(t, "git@gitlab.com:group/subgroup/service.git", remoteURL)
	})

	t.Run("applies insteadOf rewrite rules", func(t *testing.T) {
		// given: a repository with a remote URL rewritten by git config
		repositoryPath := t.TempDir()
		repository := initializeRepositoryWithRemote(t, repositoryPath, "origin", "https://example.com/platform/yeet.git")

		repositoryConfig, err := repository.Config()
		testastic.NoError(t, err)

		repositoryConfig.URLs = map[string]*gitconfig.URL{
			"ssh://git@example.com/": {
				Name:       "ssh://git@example.com/",
				InsteadOfs: []string{"https://example.com/"},
			},
		}

		err = repository.SetConfig(repositoryConfig)
		testastic.NoError(t, err)
		t.Chdir(repositoryPath)

		// when: reading the remote URL
		remoteURL, getErr := getGitRemoteURL(context.Background(), "origin")

		// then: the rewritten URL matches git behavior
		testastic.NoError(t, getErr)
		testastic.Equal(t, "ssh://git@example.com/platform/yeet.git", remoteURL)
	})

	t.Run("reads repositories with worktree config enabled", func(t *testing.T) {
		// given: a repository shaped like modern CI checkouts that enable worktree-specific config
		repositoryPath := t.TempDir()
		initializeRepositoryWithRemote(t, repositoryPath, "origin", "https://dev.azure.com/org/project/_git/repo")
		configPath := filepath.Join(repositoryPath, ".git", "config")
		configContent, err := os.ReadFile(configPath)
		testastic.NoError(t, err)

		configContent = append(configContent, []byte("\n[extensions]\n\tworktreeConfig = true\n")...)
		err = os.WriteFile(configPath, configContent, 0o644)
		testastic.NoError(t, err)
		t.Chdir(repositoryPath)

		// when: reading the remote URL
		remoteURL, getErr := getGitRemoteURL(context.Background(), "origin")

		// then: worktreeConfig does not block repository discovery
		testastic.NoError(t, getErr)
		testastic.Equal(t, "https://dev.azure.com/org/project/_git/repo", remoteURL)
	})

	t.Run("fails when remote is missing", func(t *testing.T) {
		// given: a repository without the requested remote
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)
		t.Chdir(repositoryPath)

		// when: reading an unknown remote
		remoteURL, getErr := getGitRemoteURL(context.Background(), "origin")

		// then: a clear error is returned
		testastic.Equal(t, "", remoteURL)
		testastic.Error(t, getErr)
		testastic.ErrorIs(t, getErr, ErrGitRemoteNotFound)
		testastic.ErrorContains(t, getErr, `"origin"`)
	})
}

func TestCurrentGitBranch(t *testing.T) {
	t.Run("uses Azure Pipelines full source branch", func(t *testing.T) {
		// given: an Azure Pipelines checkout where Git HEAD cannot provide the branch
		t.Chdir(t.TempDir())
		clearBranchEnv(t)
		t.Setenv("BUILD_SOURCEBRANCH", " refs/heads/release/2026 ")

		// when: resolving the current branch
		branch, err := currentGitBranch(context.Background())

		// then: the heads prefix is removed without losing nested branch segments
		testastic.NoError(t, err)
		testastic.Equal(t, "release/2026", branch)
	})

	for name, ref := range map[string]string{
		"rejects Azure Pipelines pull request ref": "refs/pull/123/merge",
		"rejects Azure Pipelines tag ref":          "refs/tags/v1.2.3",
	} {
		t.Run(name, func(t *testing.T) {
			// given: an Azure Pipelines source ref that is not a branch
			t.Chdir(t.TempDir())
			clearBranchEnv(t)
			t.Setenv("BUILD_SOURCEBRANCH", ref)

			// when: resolving the current branch
			branch, err := currentGitBranch(context.Background())

			// then: the ref is rejected instead of being returned as a branch
			testastic.Equal(t, "", branch)
			testastic.Error(t, err)
			testastic.ErrorContains(t, err, "not a branch")
		})
	}

	for name, refs := range map[string]struct {
		full string
		name string
	}{
		"rejects GitHub Actions pull request ref": {full: "refs/pull/123/merge", name: "123/merge"},
		"rejects GitHub Actions tag ref":          {full: "refs/tags/v1.2.3", name: "v1.2.3"},
	} {
		t.Run(name, func(t *testing.T) {
			// given: a GitHub Actions ref that is not a branch
			t.Chdir(t.TempDir())
			clearBranchEnv(t)
			t.Setenv("GITHUB_REF", refs.full)
			t.Setenv("GITHUB_REF_NAME", refs.name)

			// when: resolving the current branch
			branch, err := currentGitBranch(context.Background())

			// then: the ref is rejected instead of its short name being returned as a branch
			testastic.Equal(t, "", branch)
			testastic.Error(t, err)
			testastic.ErrorContains(t, err, "not a branch")
		})
	}
}

func initializeRepositoryWithRemote(t *testing.T, path, remoteName, remoteURL string) *git.Repository {
	t.Helper()

	repository, err := git.PlainInit(path, false)
	testastic.NoError(t, err)

	_, err = repository.CreateRemote(&gitconfig.RemoteConfig{
		Name: remoteName,
		URLs: []string{remoteURL},
	})
	testastic.NoError(t, err)

	return repository
}
