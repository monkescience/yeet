package cli //nolint:testpackage // validates unexported repository helpers directly

import (
	"context"
	"errors"
	"testing"

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
