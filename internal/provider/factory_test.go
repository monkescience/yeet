package provider //nolint:testpackage // validates unexported provider construction directly

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	"github.com/monkescience/testastic"
)

func TestCreateGitHubProviderPrefersGitHubURLOverRepositoryHost(t *testing.T) {
	// given: a custom github enterprise host and an explicit GITHUB_URL
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://ghe-proxy.example/api/v3/")

	// when: creating the github provider
	githubProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitHub,
		Host:     "github.company.com",
		Owner:    "platform",
		Repo:     "yeet",
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
	githubProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitHub,
		Host:     "github.company.com",
		Owner:    "platform",
		Repo:     "yeet",
	})

	// then: the API URL is derived from the repository host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.company.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderFallsBackToGHToken(t *testing.T) {
	// given: only the GH_TOKEN fallback in the environment
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "")

	// when: creating the github provider
	githubProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitHub,
		Host:     DefaultGitHubHost,
		Owner:    "platform",
		Repo:     "yeet",
	})

	// then: the fallback variable authenticates the client
	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitHubProviderReportsBothTokenNames(t *testing.T) {
	// given: no github token in the environment
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	// when: creating the github provider
	_, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitHub,
		Host:     DefaultGitHubHost,
		Owner:    "platform",
		Repo:     "yeet",
	})

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
	gitlabProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitLab,
		Host:     DefaultGitLabHost,
		Project:  "group/subgroup/service",
	})

	// then: the fallback variable authenticates the client
	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab.com/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateGitLabProviderReportsBothTokenNames(t *testing.T) {
	// given: no gitlab token in the environment
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")

	// when: creating the gitlab provider
	_, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitLab,
		Host:     DefaultGitLabHost,
		Project:  "group/subgroup/service",
	})

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
	gitlabProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitLab,
		Host:     "gitlab.company.com",
		Project:  "group/subgroup/service",
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
	gitlabProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitLab,
		Host:     "gitlab.company.com",
		Project:  "group/subgroup/service",
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
	githubProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitHub,
		Host:     DefaultGitHubHost,
		Owner:    "platform",
		Repo:     "yeet",
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
	gitlabProvider, err := Create(&RepositoryDescriptor{
		Provider: providerNameGitLab,
		Host:     DefaultGitLabHost,
		Project:  "group/subgroup/service",
	})

	// then: GITLAB_URL is honored on the default host
	testastic.NoError(t, err)
	testastic.Equal(t, "https://example.test/group/subgroup/service", gitlabProvider.RepoURL())
}

func TestCreateAzureDevOpsProviderUsesNativePATEnv(t *testing.T) {
	// given: a PAT supplied via AZURE_DEVOPS_EXT_PAT
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "test-token")

	// when: creating the Azure DevOps provider
	azureDevOpsProvider, err := Create(&RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
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
	azureDevOpsProvider, err := Create(&RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
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
	azureDevOpsProvider, err := Create(&RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
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
	_, err := Create(&RepositoryDescriptor{
		Provider:     providerNameAzureDevOps,
		Host:         "dev.azure.com",
		Organization: "platform",
		Project:      "release-tools",
		Repo:         "yeet",
	})

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
		remoteURL, err := GitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := GitRemoteURL(context.Background(), "upstream")

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
		remoteURL, getErr := GitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := GitRemoteURL(context.Background(), "origin")

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
		remoteURL, getErr := GitRemoteURL(context.Background(), "origin")

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
