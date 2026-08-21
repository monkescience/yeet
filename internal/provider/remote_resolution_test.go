package provider //nolint:testpackage // validates unexported repository resolution directly

import (
	"context"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

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

	t.Run("preserves configured API and web URLs through remote resolution", func(t *testing.T) {
		t.Parallel()

		// given: GitLab URLs with repository coordinates left to the remote
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
			APIURL: "https://gitlab.company.com/root/api/v4",
			WebURL: "https://gitlab.company.com/root",
		}

		// when: resolving the repository from a GitLab SSH remote
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@gitlab.company.com:group/service.git", nil
			},
		)

		// then: the configured provider URLs are preserved
		testastic.NoError(t, err)
		testastic.Equal(t, "https://gitlab.company.com/root/api/v4", repository.APIURL)
		testastic.Equal(t, "https://gitlab.company.com/root", repository.WebURL)
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
		testastic.ErrorIs(t, err, ErrUnsupportedHost)
		testastic.Equal(
			t,
			"resolve repository provider for host \"code.company.com\": unsupported remote host: "+
				"code.company.com. Auto-detection only supports github.com, gitlab.com, and "+
				"dev.azure.com. Set provider, [repository], or pass explicit flags for custom domains",
			err.Error(),
		)
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
		testastic.ErrorIs(t, err, ErrUnsupportedHost)
		testastic.Equal(
			t,
			"resolve repository provider for host \"github.company.com\": unsupported remote host: "+
				"github.company.com. Auto-detection only supports github.com, gitlab.com, and "+
				"dev.azure.com. Set provider, [repository], or pass explicit flags for custom domains",
			err.Error(),
		)
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

func TestResolveRepositoryRejectsConfiguredCoordinatesUnderAutoProvider(t *testing.T) {
	t.Parallel()

	// given: configured github coordinates while the provider is left on auto
	cfg := config.Default()
	cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
		Owner: "platform",
		Repo:  "yeet",
	}

	// when: resolving the repository
	_, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "git@github.com:other/repo.git", nil
		},
	)

	// then: auto rejects the coordinates it cannot route instead of ignoring them
	testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	testastic.Equal(
		t,
		"invalid config: repository.github set but provider is auto. Set an explicit provider",
		err.Error(),
	)
}

func TestResolveRepositoryAcceptsMixedCaseProjectAndOwner(t *testing.T) {
	t.Parallel()

	// given: a project written in a different case from the owner and repo
	cfg := config.Default()
	cfg.Provider = config.ProviderGitHub
	cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
		Project: "Acme/Widgets",
		Owner:   "acme",
		Repo:    "widgets",
	}

	// when: resolving the repository
	repository, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "", errors.New("git remote lookup should not run")
		},
	)

	// then: the case difference is not reported as a coordinate conflict
	testastic.NoError(t, err)
	testastic.Equal(t, "Acme/Widgets", repository.Project)
	testastic.Equal(t, "acme", repository.Owner)
	testastic.Equal(t, "widgets", repository.Repo)
}
