package provider //nolint:testpackage // validates unexported provider construction directly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestCreateGitHubProviderPrefersGitHubURLOverRepositoryHost(t *testing.T) {
	// given: a custom github enterprise host and an explicit GITHUB_URL
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://ghe-proxy.example/api/v3/")

	// when: creating the github provider
	githubProvider, err := createConfigured(&resolvedGitHubRepository{
		Host:  "github.company.com",
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{})

	// then: the explicit GITHUB_URL wins over the URL derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://ghe-proxy.example/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderDerivesURLFromRepositoryHost(t *testing.T) {
	// given: a custom github enterprise host and no GITHUB_URL
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "")

	// when: creating the github provider
	githubProvider, err := createConfigured(&resolvedGitHubRepository{
		Host:  "github.company.com",
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{})

	// then: the API URL is derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.company.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderUsesConfiguredAPIAndWebURLs(t *testing.T) {
	// given: distinct trusted API and browser paths on one GitHub Enterprise host
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv(githubURLEnv, "")

	// when: creating the GitHub provider
	created, err := createConfigured(&resolvedGitHubRepository{
		Host:   "github.company.com",
		APIURL: "https://github.company.com/root/api/v3/",
		WebURL: "https://github.company.com/root",
		Owner:  "platform",
		Repo:   "yeet",
	}, providerSettings{})

	// then: API requests and browser links use their separately configured roots
	testastic.NoError(t, err)

	githubProvider, ok := created.(*GitHub)
	testastic.True(t, ok)
	testastic.Equal(t, "https://github.company.com/root/api/v3/", githubProvider.client.BaseURL())
	testastic.Equal(t, "https://github.company.com/root/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderEnvironmentURLWinsOverConfiguredAPIURL(t *testing.T) {
	// given: an operator URL override plus checked-in API and web URLs
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv(githubURLEnv, "https://proxy.example/api/v3/")

	// when: creating the GitHub provider
	created, err := createConfigured(&resolvedGitHubRepository{
		Host:   "github.company.com",
		APIURL: "https://github.company.com/root/api/v3/",
		WebURL: "https://github.company.com/root",
		Owner:  "platform",
		Repo:   "yeet",
	}, providerSettings{})

	// then: the environment controls API requests while browser links retain their configured root
	testastic.NoError(t, err)

	githubProvider, ok := created.(*GitHub)
	testastic.True(t, ok)
	testastic.Equal(t, "https://proxy.example/api/v3/", githubProvider.client.BaseURL())
	testastic.Equal(t, "https://github.company.com/root/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderAppliesConfiguredReleaseBranch(t *testing.T) {
	// given: GitHub coordinates and a rendered custom release branch
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "")

	// when: creating the configured provider
	created, err := createConfigured(&resolvedGitHubRepository{
		Host:  DefaultGitHubHost,
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{
		releaseBranch: "automation/main/release",
		mergePolling: &config.ReleaseMergePollingConfig{
			InitialInterval: time.Second,
			MaxInterval:     7 * time.Second,
			Timeout:         3 * time.Minute,
		},
	})

	// then: provider queries and trust checks share that exact branch
	testastic.NoError(t, err)

	githubProvider, ok := created.(*GitHub)
	testastic.True(t, ok)
	testastic.Equal(t, "automation/main/release", githubProvider.releaseBranch)
	testastic.Equal(t, time.Second, githubProvider.polling.interval)
	testastic.Equal(t, 7*time.Second, githubProvider.polling.maxInterval)
	testastic.Equal(t, 3*time.Minute, githubProvider.polling.timeout)
}

func TestCreateConfiguredProviderAppliesRequestTimeoutPerAttempt(t *testing.T) {
	network := config.Default().Network
	network.RequestTimeout = 500 * time.Millisecond
	network.Retry.MaxAttempts = 2
	network.Retry.MinBackoff = 200 * time.Millisecond
	network.Retry.MaxBackoff = 200 * time.Millisecond
	settings := providerSettings{network: &network}

	slowFirstAttemptServer := func(t *testing.T) (*httptest.Server, *atomic.Int32) {
		t.Helper()

		var attempts atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				time.Sleep(350 * time.Millisecond)
				w.WriteHeader(http.StatusBadGateway)

				return
			}

			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		return server, &attempts
	}

	assertAttemptTimesOut := func(t *testing.T, client *http.Client) {
		t.Helper()

		var attempts atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)

			select {
			case <-time.After(700 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(server.Close)

		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		testastic.NoError(t, err)

		response, err := client.Do(request)
		if err == nil {
			testastic.NoError(t, response.Body.Close())
		}

		testastic.Error(t, err)
		testastic.Equal(t, int32(network.Retry.MaxAttempts), attempts.Load())
	}

	t.Run("github", func(t *testing.T) {
		// given: a GitHub adapter whose first attempt is slow and fails with a retryable status
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv(githubURLEnv, "")

		server, attempts := slowFirstAttemptServer(t)

		created, err := createConfigured(&resolvedGitHubRepository{
			Host:  DefaultGitHubHost,
			Owner: "platform",
			Repo:  "yeet",
		}, settings)
		testastic.NoError(t, err)

		githubProvider, ok := created.(*GitHub)
		testastic.True(t, ok)
		assertAttemptTimesOut(t, githubProvider.client.Client())

		// when: sending a request through the effective GitHub HTTP client
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		testastic.NoError(t, err)

		response, err := githubProvider.client.Client().Do(request)

		// then: the timeout bounds each attempt, so the retry succeeds
		testastic.NoError(t, err)

		if err != nil {
			return
		}

		testastic.Equal(t, http.StatusOK, response.StatusCode)
		testastic.NoError(t, response.Body.Close())
		testastic.Equal(t, int32(2), attempts.Load())
	})

	t.Run("azure devops", func(t *testing.T) {
		// given: an Azure DevOps adapter whose first attempt is slow and fails with a retryable status
		t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
		t.Setenv(azureURLEnv, "")

		server, attempts := slowFirstAttemptServer(t)

		created, err := createConfigured(&resolvedAzureDevOpsRepository{
			Host:         DefaultAzureDevOpsHost,
			Organization: "platform",
			Project:      "release-tools",
			Repo:         "yeet",
		}, settings)
		testastic.NoError(t, err)

		azureProvider, ok := created.(*AzureDevOps)
		testastic.True(t, ok)
		assertAttemptTimesOut(t, azureProvider.httpClient)

		// when: sending a request through the HTTP client used by the Azure DevOps git client
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		testastic.NoError(t, err)

		response, err := azureProvider.httpClient.Do(request)

		// then: the timeout bounds each attempt, so the retry succeeds
		testastic.NoError(t, err)

		if err != nil {
			return
		}

		testastic.Equal(t, http.StatusOK, response.StatusCode)
		testastic.NoError(t, response.Body.Close())
		testastic.Equal(t, int32(2), attempts.Load())

		// then: the non-retrying SDK connection keeps the configured timeout
		expectedTimeout := network.RequestTimeout
		testastic.DeepEqual(t, &expectedTimeout, azureProvider.conn.Timeout)
	})
}

func TestCreateConfiguredProviderRetriesTimedOutAttempt(t *testing.T) {
	network := config.Default().Network
	network.RequestTimeout = 300 * time.Millisecond
	network.Retry.MinBackoff = time.Millisecond
	network.Retry.MaxBackoff = time.Millisecond
	settings := providerSettings{network: &network}

	hungFirstAttemptServer := func(t *testing.T) (*httptest.Server, *atomic.Int32) {
		t.Helper()

		var attempts atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				time.Sleep(500 * time.Millisecond)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Cleanup(server.Close)

		return server, &attempts
	}

	sendGet := func(t *testing.T, client *http.Client, url string) error {
		t.Helper()

		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		testastic.NoError(t, err)

		response, err := client.Do(request)
		if err != nil {
			return err
		}

		return response.Body.Close()
	}

	t.Run("github", func(t *testing.T) {
		// given: a GitHub adapter whose first attempt exceeds the request timeout
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv(githubURLEnv, "")

		server, attempts := hungFirstAttemptServer(t)

		created, err := createConfigured(&resolvedGitHubRepository{
			Host:  DefaultGitHubHost,
			Owner: "platform",
			Repo:  "yeet",
		}, settings)
		testastic.NoError(t, err)

		githubProvider, ok := created.(*GitHub)
		testastic.True(t, ok)

		// when: sending a read through the effective GitHub HTTP client
		err = sendGet(t, githubProvider.client.Client(), server.URL)

		// then: the timed-out attempt is retried
		testastic.NoError(t, err)
		testastic.Equal(t, int32(2), attempts.Load())
	})

	t.Run("gitlab", func(t *testing.T) {
		// given: a GitLab adapter whose first attempt exceeds the request timeout
		server, attempts := hungFirstAttemptServer(t)

		t.Setenv("GITLAB_TOKEN", "test-token")
		t.Setenv(gitlabURLEnv, server.URL)

		created, err := createConfigured(&resolvedGitLabRepository{Project: "group/service"}, settings)
		testastic.NoError(t, err)

		gitlabProvider, ok := created.(*GitLab)
		testastic.True(t, ok)

		// when: sending a read through the GitLab SDK
		_, _, err = gitlabProvider.client.Version.GetVersion()

		// then: the timed-out attempt is retried
		testastic.NoError(t, err)
		testastic.Equal(t, int32(2), attempts.Load())
	})

	t.Run("azure devops", func(t *testing.T) {
		// given: an Azure DevOps adapter whose first attempt exceeds the request timeout
		t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
		t.Setenv(azureURLEnv, "")

		server, attempts := hungFirstAttemptServer(t)

		created, err := createConfigured(&resolvedAzureDevOpsRepository{
			Host:         DefaultAzureDevOpsHost,
			Organization: "platform",
			Project:      "release-tools",
			Repo:         "yeet",
		}, settings)
		testastic.NoError(t, err)

		azureProvider, ok := created.(*AzureDevOps)
		testastic.True(t, ok)

		// when: sending a read through the HTTP client used by the Azure DevOps git client
		err = sendGet(t, azureProvider.httpClient, server.URL)

		// then: the timed-out attempt is retried
		testastic.NoError(t, err)
		testastic.Equal(t, int32(2), attempts.Load())
	})
}

func TestCreateGitHubProviderFallsBackToGHToken(t *testing.T) {
	// given: only the GH_TOKEN fallback in the environment
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "")

	// when: creating the github provider
	githubProvider, err := createConfigured(&resolvedGitHubRepository{
		Host:  DefaultGitHubHost,
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{})

	// then: the fallback variable authenticates the client
	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderReportsBothTokenNames(t *testing.T) {
	// given: no github token in the environment
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	// when: creating the github provider
	_, err := createConfigured(&resolvedGitHubRepository{
		Host:  DefaultGitHubHost,
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{})

	// then: the error names both supported environment variables
	testastic.ErrorIs(t, err, ErrMissingToken)
	testastic.Equal(
		t,
		"missing auth token: GITHUB_TOKEN or GH_TOKEN environment variable is required",
		err.Error(),
	)
}

func TestCreateGitLabProviderFallsBackToGLToken(t *testing.T) {
	// given: only the GL_TOKEN fallback in the environment
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "")

	// when: creating the gitlab provider
	gitlabProvider, err := createConfigured(&resolvedGitLabRepository{
		Host:    DefaultGitLabHost,
		Project: "group/subgroup/service",
	}, providerSettings{})

	// then: the fallback variable authenticates the client
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab.com/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitLabProviderReportsBothTokenNames(t *testing.T) {
	// given: no gitlab token in the environment
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")

	// when: creating the gitlab provider
	_, err := createConfigured(&resolvedGitLabRepository{
		Host:    DefaultGitLabHost,
		Project: "group/subgroup/service",
	}, providerSettings{})

	// then: the error names both supported environment variables
	testastic.ErrorIs(t, err, ErrMissingToken)
	testastic.Equal(
		t,
		"missing auth token: GITLAB_TOKEN or GL_TOKEN environment variable is required",
		err.Error(),
	)
}

func TestCreateGitLabProviderPrefersGitLabURLOverRepositoryHost(t *testing.T) {
	// given: a custom gitlab host and an explicit GITLAB_URL
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://gitlab-proxy.example/api/v4")

	// when: creating the gitlab provider
	gitlabProvider, err := createConfigured(&resolvedGitLabRepository{
		Host:    "gitlab.company.com",
		Project: "group/subgroup/service",
	}, providerSettings{})

	// then: the explicit GITLAB_URL wins over the URL derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab-proxy.example/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitLabProviderDerivesURLFromRepositoryHost(t *testing.T) {
	// given: a custom gitlab host and no GITLAB_URL
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "")

	// when: creating the gitlab provider
	gitlabProvider, err := createConfigured(&resolvedGitLabRepository{
		Host:    "gitlab.company.com",
		Project: "group/subgroup/service",
	}, providerSettings{})

	// then: the API URL is derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab.company.com/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitLabProviderUsesConfiguredAPIAndWebURLs(t *testing.T) {
	// given: distinct trusted API and browser paths on one GitLab host
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv(gitlabURLEnv, "")

	// when: creating the GitLab provider
	created, err := createConfigured(&resolvedGitLabRepository{
		Host:    "gitlab.company.com",
		APIURL:  "https://gitlab.company.com/root/api/v4",
		WebURL:  "https://gitlab.company.com/root",
		Project: "group/service",
	}, providerSettings{})

	// then: API requests and browser links use their separately configured roots
	testastic.NoError(t, err)

	gitlabProvider, ok := created.(*GitLab)
	testastic.True(t, ok)
	testastic.Equal(t, "https://gitlab.company.com/root/api/v4/", gitlabProvider.client.BaseURL().String())
	testastic.Equal(t, "https://gitlab.company.com/root/group/service", gitlabProvider.RepoURL())
}

func TestCreateGitHubProviderHonorsGitHubURLOnDefaultHost(t *testing.T) {
	// given: the default github host with GITHUB_URL overriding the API endpoint
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://example.test/api/v3/")

	// when: creating the github provider
	githubProvider, err := createConfigured(&resolvedGitHubRepository{
		Host:  DefaultGitHubHost,
		Owner: "platform",
		Repo:  "yeet",
	}, providerSettings{})

	// then: GITHUB_URL is honored on the default host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://example.test/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitLabProviderHonorsGitLabURLOnDefaultHost(t *testing.T) {
	// given: the default gitlab host with GITLAB_URL overriding the API endpoint
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://example.test/api/v4")

	// when: creating the gitlab provider
	gitlabProvider, err := createConfigured(&resolvedGitLabRepository{
		Host:    DefaultGitLabHost,
		Project: "group/subgroup/service",
	}, providerSettings{})

	// then: GITLAB_URL is honored on the default host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://example.test/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesNativePATEnv(t *testing.T) {
	// given: a PAT supplied via AZURE_DEVOPS_EXT_PAT
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createConfigured(&resolvedAzureDevOpsRepository{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	}, providerSettings{})

	// then: the provider is constructed with the configured repository URL
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/platform/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesConfiguredAPIAndWebURLs(t *testing.T) {
	// given: distinct API and browser roots on an Azure DevOps Server host
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
	t.Setenv(azureURLEnv, "")

	// when: creating the Azure DevOps provider
	created, err := createConfigured(&resolvedAzureDevOpsRepository{
		Host:         "devops.company.com",
		APIURL:       "https://devops.company.com/api/tfs",
		WebURL:       "https://devops.company.com/web/tfs",
		Organization: "platform",
		Collection:   "collection",
		Project:      "release-tools",
		Repo:         "yeet",
	}, providerSettings{})

	// then: API requests and browser links use their separately configured roots
	testastic.NoError(t, err)

	azureProvider, ok := created.(*AzureDevOps)
	testastic.True(t, ok)
	testastic.Equal(t, "https://devops.company.com/api/tfs/collection", azureProvider.conn.BaseUrl)
	testastic.Equal(
		t,
		"https://devops.company.com/web/tfs/collection/release-tools/_git/yeet",
		azureProvider.RepoURL(),
	)
}

func TestCreateAzureDevOpsProviderNormalizesLegacyHost(t *testing.T) {
	// given: configured legacy Azure DevOps coordinates and no endpoint override
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")
	t.Setenv(azureURLEnv, "")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createConfigured(&resolvedAzureDevOpsRepository{
		Host:         "contoso.visualstudio.com",
		Organization: "contoso",
		Project:      "release-tools",
		Repo:         "yeet",
	}, providerSettings{})

	// then: the legacy host is normalized before the organization is appended
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/contoso/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesNativeSystemAccessTokenEnv(t *testing.T) {
	// given: a token supplied via AZURE_DEVOPS_SYSTEM_ACCESSTOKEN
	t.Setenv("AZURE_DEVOPS_SYSTEM_ACCESSTOKEN", "test-token")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := createConfigured(&resolvedAzureDevOpsRepository{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	}, providerSettings{})

	// then: the provider is constructed with the configured repository URL
	testastic.NoError(t, err)
	testastic.Equal(t, "https://dev.azure.com/platform/release-tools/_git/yeet", azureDevOpsProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderReportsNativeTokenNames(t *testing.T) {
	// given: no Azure DevOps token in the environment
	t.Setenv("AZURE_DEVOPS_SYSTEM_ACCESSTOKEN", "")
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "")

	// when: creating the Azure DevOps provider
	_, err := createConfigured(&resolvedAzureDevOpsRepository{
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	}, providerSettings{})

	// then: the error names both supported environment variables
	testastic.Error(t, err)
	testastic.Equal(
		t,
		"missing auth token: AZURE_DEVOPS_SYSTEM_ACCESSTOKEN or AZURE_DEVOPS_EXT_PAT environment "+
			"variable is required",
		err.Error(),
	)
}

func TestGetGitRemoteURL(t *testing.T) {
	t.Run("reads origin url from repository root", func(t *testing.T) {
		// given: a repository with an origin remote
		repositoryPath := t.TempDir()
		initializeRepositoryWithRemote(t, repositoryPath, "origin", "git@github.com:platform/yeet.git")
		t.Chdir(repositoryPath)

		// when: reading the remote URL
		remoteURL, err := gitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := gitRemoteURL(context.Background(), "upstream")

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

		repositoryConfig.URLs = []*gitconfig.URL{
			{
				Name:       "ssh://git@example.com/",
				InsteadOfs: []string{"https://example.com/"},
			},
		}

		err = repository.SetConfig(repositoryConfig)
		testastic.NoError(t, err)
		t.Chdir(repositoryPath)

		// when: reading the remote URL
		remoteURL, getErr := gitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := gitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := gitRemoteURL(context.Background(), "origin")

		// then: a clear error is returned
		testastic.Equal(t, "", remoteURL)
		testastic.Error(t, getErr)
		testastic.ErrorIs(t, getErr, ErrGitRemoteNotFound)
		testastic.Equal(t, "git remote not found: \"origin\"", getErr.Error())
	})
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
