package cli //nolint:testpackage // validates unexported repository helpers directly

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
)

func TestResolveRepository(t *testing.T) {
	t.Parallel()

	t.Run("uses explicit config without git remote access", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.Host = "gitlab.company.com"
		cfg.Repository.Project = "group/subgroup/service"

		remoteLookedUp := false

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				remoteLookedUp = true

				return "", errors.New("git remote lookup should not run")
			},
		)

		testastic.NoError(t, err)
		testastic.False(t, remoteLookedUp)
		testastic.Equal(t, "gitlab", repository.Provider)
		testastic.Equal(t, "gitlab.company.com", repository.Host)
		testastic.Equal(t, "group/subgroup", repository.Owner)
		testastic.Equal(t, "service", repository.Repo)
		testastic.Equal(t, "group/subgroup/service", repository.Project)
		testastic.Equal(t, "origin", repository.Remote)
	})

	t.Run("uses explicit github coordinates without git remote access", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.Owner = "platform"
		cfg.Repository.Repo = "yeet"

		remoteLookedUp := false

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				remoteLookedUp = true

				return "", errors.New("git remote lookup should not run")
			},
		)

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

		cfg := config.Default()
		cfg.Repository.Remote = "upstream"

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(_ context.Context, remote string) (string, error) {
				testastic.Equal(t, "upstream", remote)

				return "git@github.com:platform/yeet.git", nil
			},
		)

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

		cfg := config.Default()

		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@code.company.com:team/service.git", nil
			},
		)

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, provider.ErrUnsupportedHost)
		testastic.ErrorContains(t, err, "set provider, [repository], or pass explicit flags")
	})

	t.Run("honors explicit provider on unknown host", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@code.company.com:group/subgroup/service.git", nil
			},
		)

		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", repository.Provider)
		testastic.Equal(t, "code.company.com", repository.Host)
		testastic.Equal(t, "group/subgroup", repository.Owner)
		testastic.Equal(t, "service", repository.Repo)
		testastic.Equal(t, "group/subgroup/service", repository.Project)
	})

	t.Run("explicit coordinates override remote coordinates", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Repository.Owner = "platform"
		cfg.Repository.Repo = "yeet"

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@gitlab.com:group/other.git", nil
			},
		)

		testastic.NoError(t, err)
		testastic.Equal(t, "gitlab", repository.Provider)
		testastic.Equal(t, "platform", repository.Owner)
		testastic.Equal(t, "yeet", repository.Repo)
		testastic.Equal(t, "platform/yeet", repository.Project)
	})

	t.Run("fails when project conflicts with owner and repo", func(t *testing.T) {
		t.Parallel()

		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.Project = "group/subgroup/service"
		cfg.Repository.Owner = "platform"
		cfg.Repository.Repo = "yeet"

		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", errors.New("git remote lookup should not run")
			},
		)

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrRepositoryConflict)
		testastic.ErrorContains(t, err, "project \"group/subgroup/service\" does not match owner/repo \"platform/yeet\"")
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
		resolvedPath, resolveErr := resolveConfigPath("")

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
		resolvedPath, resolveErr := resolveConfigPath(" custom.toml ")

		// then: the explicit path is used as-is after trimming
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, "custom.toml", resolvedPath)
	})

	t.Run("missing default config reports the default filename", func(t *testing.T) {
		// given: a directory tree without any yeet config file
		repositoryPath := t.TempDir()
		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: resolving the default config path
		resolvedPath, resolveErr := resolveConfigPath("")

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
		resolvedPath, resolveErr := resolveConfigPath("")

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
		resolvedPath, resolveErr := resolveInitConfigPath("")

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
		resolvedPath, resolveErr := resolveInitConfigPath("")

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
		resolvedPath, resolveErr := resolveInitConfigPath("")

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
		resolvedPath, resolveErr := resolveInitConfigPath("")

		// then: init still targets the repo root config path
		testastic.NoError(t, resolveErr)
		testastic.Equal(t, filepath.Join(repositoryPath, config.DefaultFile), resolvedPath)
	})
}

func TestCreateGitHubProviderUsesRepositoryHost(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_URL", "https://ignored.example/api/v3/")

	githubProvider, err := createGitHubProvider(&provider.RepositoryDescriptor{
		Host:  "github.company.com",
		Owner: "platform",
		Repo:  "yeet",
	})

	testastic.NoError(t, err)
	testastic.Equal(t, "https://github.company.com/platform/yeet", githubProvider.RepoURL())
}

func TestCreateGitLabProviderUsesRepositoryHost(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token")
	t.Setenv("GITLAB_URL", "https://ignored.example/api/v4")

	gitlabProvider, err := createGitLabProvider(&provider.RepositoryDescriptor{
		Host:    "gitlab.company.com",
		Project: "group/subgroup/service",
	})

	testastic.NoError(t, err)
	testastic.Equal(t, "https://gitlab.company.com/group/subgroup/service", gitlabProvider.RepoURL())
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
				Name:      "ssh://git@example.com/",
				InsteadOf: "https://example.com/",
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
