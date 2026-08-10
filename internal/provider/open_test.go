package provider //nolint:testpackage // validates private opening dependencies and ordering directly

import (
	"context"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("explicit coordinates avoid remote lookup and use provider defaults", func(t *testing.T) {
		t.Parallel()

		// given: explicit GitHub coordinates and a selected adapter
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "platform", Repo: "yeet"}
		expected := &GitHub{}

		remoteLookedUp := false

		// when: opening the configured provider
		actual, err := open(t.Context(), cfg, openDependencies{
			getRemoteURL: func(context.Context, string) (string, error) {
				remoteLookedUp = true

				return "", errors.New("git remote lookup should not run")
			},
			create: func(repository *repositoryDescriptor) (forge.Provider, error) {
				testastic.Equal(t, providerNameGitHub, repository.Provider)
				testastic.Equal(t, DefaultGitHubHost, repository.Host)
				testastic.Equal(t, "platform", repository.Owner)
				testastic.Equal(t, "yeet", repository.Repo)

				return expected, nil
			},
		})

		// then: opening returns the selected adapter without consulting Git
		testastic.NoError(t, err)
		testastic.False(t, remoteLookedUp)
		testastic.Equal(t, forge.Provider(expected), actual)
	})

	t.Run("missing coordinates use the configured remote and detect the provider", func(t *testing.T) {
		t.Parallel()

		// given: automatic provider configuration with a custom remote name
		cfg := config.Default()
		cfg.Repository.Remote = "upstream"

		// when: opening the provider
		_, err := open(t.Context(), cfg, openDependencies{
			getRemoteURL: func(_ context.Context, remote string) (string, error) {
				testastic.Equal(t, "upstream", remote)

				return "git@gitlab.com:group/subgroup/service.git", nil
			},
			create: func(repository *repositoryDescriptor) (forge.Provider, error) {
				testastic.Equal(t, providerNameGitLab, repository.Provider)
				testastic.Equal(t, DefaultGitLabHost, repository.Host)
				testastic.Equal(t, "group/subgroup/service", repository.Project)

				return &GitLab{}, nil
			},
		})

		// then: remote resolution and automatic detection complete before construction
		testastic.NoError(t, err)
	})

	t.Run("unsupported automatic host prevents adapter construction", func(t *testing.T) {
		t.Parallel()

		// given: an unsupported remote host and a tracked constructor
		cfg := config.Default()
		constructed := false

		// when: opening the provider
		_, err := open(t.Context(), cfg, openDependencies{
			getRemoteURL: func(context.Context, string) (string, error) {
				return "git@code.company.com:team/service.git", nil
			},
			create: func(*repositoryDescriptor) (forge.Provider, error) {
				constructed = true

				return &GitHub{}, nil
			},
		})

		// then: provider detection fails before construction
		testastic.ErrorIs(t, err, ErrUnsupportedHost)
		testastic.False(t, constructed)
	})

	t.Run("host trust prevents credential-backed construction", func(t *testing.T) {
		t.Parallel()

		// given: explicit coordinates whose host differs from the configured remote
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Host:  "github.company.com",
			Owner: "platform",
			Repo:  "yeet",
		}
		constructed := false

		// when: opening the provider
		_, err := open(t.Context(), cfg, openDependencies{
			getRemoteURL: func(context.Context, string) (string, error) {
				return "git@other.company.com:platform/yeet.git", nil
			},
			create: func(*repositoryDescriptor) (forge.Provider, error) {
				constructed = true

				return &GitHub{}, nil
			},
		})

		// then: trust validation fails before credentials or adapter construction
		testastic.ErrorIs(t, err, ErrUntrustedHost)
		testastic.False(t, constructed)
	})

	t.Run("construction failure preserves its cause", func(t *testing.T) {
		t.Parallel()

		// given: valid repository coordinates and a failing constructor
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "platform", Repo: "yeet"}
		cause := errors.New("adapter construction failed")

		// when: opening the provider
		_, err := open(t.Context(), cfg, openDependencies{
			getRemoteURL: func(context.Context, string) (string, error) {
				return "", errors.New("git remote lookup should not run")
			},
			create: func(*repositoryDescriptor) (forge.Provider, error) {
				return nil, cause
			},
		})

		// then: callers can still inspect the original construction cause
		testastic.ErrorIs(t, err, cause)
	})
}

func TestOpenReportsMissingCredentialsAfterRepositoryResolution(t *testing.T) {
	// given: complete trusted coordinates with no supported credential
	cfg := config.Default()
	cfg.Provider = config.ProviderGitHub
	cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "platform", Repo: "yeet"}

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	// when: opening the production provider
	_, err := Open(t.Context(), cfg)

	// then: repository resolution succeeds before credential lookup fails
	testastic.ErrorIs(t, err, ErrMissingToken)
}
