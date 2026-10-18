package provider //nolint:testpackage // validates unexported host trust rules directly

import (
	"context"
	"errors"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func githubTrustConfig(host string) *config.Config {
	cfg := config.Default()
	cfg.Provider = config.ProviderGitHub
	cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
		Host:  host,
		Owner: "platform",
		Repo:  "yeet",
	}

	return cfg
}

func TestValidateHostFormat(t *testing.T) {
	t.Parallel()

	t.Run("accepts a bare hostname", func(t *testing.T) {
		t.Parallel()

		// given: a bare enterprise hostname
		host := "gitlab.company.com"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is accepted
		testastic.NoError(t, err)
	})

	t.Run("rejects an empty host", func(t *testing.T) {
		t.Parallel()

		// given: an empty host value

		// when: validating an empty host
		err := validateHostFormat("")

		// then: the empty host is reported
		testastic.ErrorIs(t, err, ErrInvalidHost)
		testastic.Equal(t, "invalid provider host: host must not be empty", err.Error())
	})

	t.Run("rejects a scheme-prefixed host", func(t *testing.T) {
		t.Parallel()

		// given: a host carrying a scheme
		host := "https://gitlab.company.com"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is rejected as not bare
		testastic.ErrorIs(t, err, ErrInvalidHost)
		testastic.Equal(
			t,
			"invalid provider host: \"https://gitlab.company.com\" must be a bare hostname without "+
				"scheme, credentials, or path",
			err.Error(),
		)
	})

	t.Run("rejects a credential-bearing host", func(t *testing.T) {
		t.Parallel()

		// given: a host carrying userinfo
		host := "attacker@gitlab.company.com"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is rejected as not bare
		testastic.ErrorIs(t, err, ErrInvalidHost)
		testastic.Equal(
			t,
			"invalid provider host: \"attacker@gitlab.company.com\" must be a bare hostname without "+
				"scheme, credentials, or path",
			err.Error(),
		)
	})

	t.Run("rejects a path-bearing host", func(t *testing.T) {
		t.Parallel()

		// given: a host carrying a path segment
		host := "gitlab.company.com/api/v4"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is rejected as not bare
		testastic.ErrorIs(t, err, ErrInvalidHost)
		testastic.Equal(
			t,
			"invalid provider host: \"gitlab.company.com/api/v4\" must be a bare hostname without "+
				"scheme, credentials, or path",
			err.Error(),
		)
	})

	t.Run("rejects a whitespace-bearing host", func(t *testing.T) {
		t.Parallel()

		// given: a host carrying an inner space
		host := "gitlab company.com"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is rejected as not bare
		testastic.ErrorIs(t, err, ErrInvalidHost)
		testastic.Equal(
			t,
			"invalid provider host: \"gitlab company.com\" must be a bare hostname without "+
				"scheme, credentials, or path",
			err.Error(),
		)
	})

	t.Run("rejects a control-character host", func(t *testing.T) {
		t.Parallel()

		// given: a host carrying a control character
		host := "gitlab.company.com\x00"

		// when: validating its format
		err := validateHostFormat(host)

		// then: the host is rejected as not bare
		testastic.ErrorIs(t, err, ErrInvalidHost)
	})
}

func TestResolveRepositoryHostTrust(t *testing.T) {
	t.Parallel()

	t.Run("custom host matching the git remote is trusted", func(t *testing.T) {
		t.Parallel()

		// given: a custom enterprise host that matches where the repo is cloned from
		cfg := githubTrustConfig("github.company.com")

		// when: resolving with a remote on the same host
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "https://github.company.com/platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the host is accepted
		testastic.NoError(t, err)
		testastic.Equal(t, "github.company.com", repository.Host)
	})

	t.Run("custom host not matching the git remote is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a custom host pointing somewhere other than the git remote
		cfg := githubTrustConfig("evil.example")

		// when: resolving with a remote on github.com
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "https://github.com/platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: yeet refuses to send the token to the mismatched host
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"provider host is not trusted: \"evil.example\" does not match git remote host \"github.com\"",
			err.Error(),
		)
	})

	t.Run("default public host does not require a remote lookup", func(t *testing.T) {
		t.Parallel()

		// given: the default github host
		cfg := githubTrustConfig(DefaultGitHubHost)

		// when: resolving with a getter that fails if called
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", errors.New("git remote lookup should not run for a public host")
			},
			RepositoryOverrides{},
		)

		// then: the public host is trusted without consulting the remote
		testastic.NoError(t, err)
		testastic.Equal(t, DefaultGitHubHost, repository.Host)
	})

	t.Run("host with embedded credentials is rejected as malformed", func(t *testing.T) {
		t.Parallel()

		// given: a host that hides the real destination behind userinfo
		cfg := githubTrustConfig("github.com@evil.example")

		// when: resolving
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", errors.New("git remote lookup should not run for a malformed host")
			},
			RepositoryOverrides{},
		)

		// then: the malformed host is rejected before any remote lookup
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid provider host: \"github.com@evil.example\" must be a bare hostname without scheme, "+
				"credentials, or path",
			err.Error(),
		)
	})

	t.Run("custom host with an unresolvable remote is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a custom host and no resolvable git remote to verify against
		cfg := githubTrustConfig("github.company.com")

		// when: resolving with a getter that errors
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", errors.New("no remote")
			},
			RepositoryOverrides{},
		)

		// then: trust cannot be established, so the host is rejected
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"provider host is not trusted: \"github.company.com\" could not be verified against git "+
				"remote \"origin\": no remote",
			err.Error(),
		)
	})
}

func TestResolveRepositoryHostTrustHonorsProviderURLEnv(t *testing.T) {
	// given: an operator-set GITHUB_URL naming the configured host, which mismatches the remote
	t.Setenv("GITHUB_URL", "https://github.company.com/api/v3/")

	cfg := githubTrustConfig("github.company.com")

	// when: resolving with a remote on a different host
	repository, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "https://github.com/platform/yeet.git", nil
		},
		RepositoryOverrides{},
	)

	// then: the operator-controlled env var is trusted regardless of the remote
	testastic.NoError(t, err)
	testastic.Equal(t, "github.company.com", repository.Host)
}

func TestResolveRepositoryAPIURLHostTrust(t *testing.T) {
	t.Parallel()

	t.Run("accepts a configured API URL on the repository host", func(t *testing.T) {
		t.Parallel()

		// given: a custom GitHub host and API URL on that host
		cfg := githubTrustConfig("github.company.com")
		cfg.Repository.GitHub.APIURL = "https://github.company.com/root/api/v3"

		// when: resolving against a matching repository remote
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.company.com:platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the configured API URL is accepted
		testastic.NoError(t, err)
		testastic.Equal(t, cfg.Repository.GitHub.APIURL, repository.APIURL)
	})

	t.Run("rejects a configured API URL on another host", func(t *testing.T) {
		t.Parallel()

		// given: a custom GitHub host and API URL on another host
		cfg := githubTrustConfig("github.company.com")
		cfg.Repository.GitHub.APIURL = "https://credentials.example/api/v3"

		// when: resolving against a matching repository remote
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.company.com:platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the cross-host API URL is rejected as untrusted
		testastic.ErrorIs(t, err, ErrUntrustedHost)

		var setup *SetupError
		testastic.True(t, errors.As(err, &setup))
		testastic.Equal(t, "credentials.example", setup.APIHost)
		testastic.Equal(t, "configured provider api host does not match repository host", setup.Problem)
		testastic.Equal(t, "use an api_url on the repository host", setup.Hint)
		testastic.Equal(
			t,
			"provider host is not trusted: configured api_url host \"credentials.example\" "+
				"does not match repository host \"github.company.com\"",
			err.Error(),
		)
	})
}

func TestResolveRepositoryRejectsHostUnrelatedToProviderURLEnv(t *testing.T) {
	// given: an operator-set GITLAB_URL for one self-hosted forge and a checked-in
	// config naming an entirely different host, with the remote on neither
	t.Setenv(gitlabURLEnv, "https://gitlab.selfhosted.test/api/v4")

	cfg := config.Default()
	cfg.Provider = config.ProviderGitLab
	cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
		Host:    "attacker.invalid",
		Project: "group/service",
	}

	// when: resolving the repository
	_, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "https://gitlab.selfhosted.test/group/service.git", nil
		},
		RepositoryOverrides{},
	)

	// then: the unrelated host is not trusted by the environment variable
	testastic.ErrorIs(t, err, ErrUntrustedHost)
	testastic.ErrorContains(
		t,
		err,
		"provider host is not trusted: \"attacker.invalid\" does not match git remote host "+
			"\"gitlab.selfhosted.test\"",
	)
}

func TestResolveRepositoryTrustsHostOfProviderURLEnv(t *testing.T) {
	// given: an operator-set GITLAB_URL whose host is the configured host, and a
	// remote pointing somewhere else entirely
	t.Setenv(gitlabURLEnv, "https://gitlab.selfhosted.test/api/v4")

	cfg := config.Default()
	cfg.Provider = config.ProviderGitLab
	cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
		Host:    "gitlab.selfhosted.test",
		Project: "group/service",
	}

	// when: resolving the repository
	repository, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "https://gitlab.com/group/service.git", nil
		},
		RepositoryOverrides{},
	)

	// then: the operator-controlled host is trusted without consulting the remote
	testastic.NoError(t, err)
	testastic.Equal(t, "gitlab.selfhosted.test", repository.Host)
}

func TestResolveRepositoryWrapsGitRemoteFailure(t *testing.T) {
	t.Parallel()

	// given: a custom host and a git remote lookup that fails with a known cause
	cfg := config.Default()
	cfg.Provider = config.ProviderGitLab
	cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
		Host:    "gitlab.company.com",
		Project: "group/service",
	}

	// when: resolving the repository
	_, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "", ErrGitRemoteNotFound
		},
		RepositoryOverrides{},
	)

	// then: the repository cause remains recoverable without a host-trust classification
	testastic.False(t, errors.Is(err, ErrUntrustedHost))
	testastic.ErrorIs(t, err, ErrGitRemoteNotFound)
}

func TestResolveRepositoryDefaultedHostTrust(t *testing.T) {
	t.Parallel()

	t.Run("rejects a defaulted gitlab host when the remote is self-hosted", func(t *testing.T) {
		t.Parallel()

		// given: gitlab coordinates without a host and a remote on a self-hosted gitlab
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{Project: "group/sub/repo"}

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@gitlab.company.com:group/sub/repo.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the token is not sent to the public default host
		testastic.ErrorIs(t, err, ErrUntrustedHost)

		var setup *SetupError
		testastic.True(t, errors.As(err, &setup))
		testastic.Equal(t, "gitlab.com", setup.Host)
		testastic.Equal(t, "gitlab.company.com", setup.RemoteHost)
		testastic.Equal(t, "provider host does not match git remote host", setup.Problem)
		testastic.Equal(
			t,
			"provider host is not trusted: default host \"gitlab.com\" does not match git remote host "+
				"\"gitlab.company.com\"",
			err.Error(),
		)
	})

	t.Run("rejects a defaulted github host from flag overrides on an enterprise checkout", func(t *testing.T) {
		t.Parallel()

		// given: github coordinates passed as flags and a remote on github enterprise
		provider, owner, repo := "github", "acme", "widgets"

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			config.Default(),
			func(context.Context, string) (string, error) {
				return "https://github.company.com/acme/widgets.git", nil
			},
			RepositoryOverrides{Provider: &provider, Owner: &owner, Repo: &repo},
		)

		// then: the defaulted public host is rejected
		testastic.ErrorIs(t, err, ErrUntrustedHost)
		testastic.ErrorContains(t, err, "does not match git remote host \"github.company.com\"")
	})

	t.Run("accepts a defaulted host when the remote is on the public host", func(t *testing.T) {
		t.Parallel()

		// given: github coordinates without a host and a remote on github.com
		cfg := githubTrustConfig("")

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.com:platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the default host is used
		testastic.NoError(t, err)
		testastic.Equal(t, DefaultGitHubHost, repository.Host)
	})

	t.Run("accepts a defaulted azure host when the remote uses the azure ssh host", func(t *testing.T) {
		t.Parallel()

		// given: azure coordinates without a host and a remote on the azure ssh host
		cfg := config.Default()
		cfg.Provider = config.ProviderAzureDevOps
		cfg.Repository.AzureDevOps = &config.AzureDevOpsRepositoryConfig{
			Organization: "org",
			Project:      "project",
			Repo:         "repo",
		}

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@ssh.dev.azure.com:v3/org/project/repo", nil
			},
			RepositoryOverrides{},
		)

		// then: the default host is used
		testastic.NoError(t, err)
		testastic.Equal(t, DefaultAzureDevOpsHost, repository.Host)
	})

	acceptedRemotes := []struct {
		name      string
		provider  config.ProviderType
		remoteURL string
		host      string
	}{
		{
			name:      "public host with an explicit port",
			provider:  config.ProviderGitHub,
			remoteURL: "https://github.com:443/platform/yeet.git",
			host:      DefaultGitHubHost,
		},
		{
			name:      "github ssh over the https port",
			provider:  config.ProviderGitHub,
			remoteURL: "ssh://git@ssh.github.com:443/platform/yeet.git",
			host:      DefaultGitHubHost,
		},
		{
			name:      "gitlab ssh over the https port",
			provider:  config.ProviderGitLab,
			remoteURL: "ssh://git@altssh.gitlab.com:443/platform/yeet.git",
			host:      DefaultGitLabHost,
		},
		{
			name:      "local file remote",
			provider:  config.ProviderGitHub,
			remoteURL: "file:///srv/git/yeet.git",
			host:      DefaultGitHubHost,
		},
		{
			name:      "remote without a parseable host",
			provider:  config.ProviderGitHub,
			remoteURL: "/srv/git/yeet.git",
			host:      DefaultGitHubHost,
		},
	}

	for _, tc := range acceptedRemotes {
		t.Run("accepts a defaulted host for a "+tc.name, func(t *testing.T) {
			t.Parallel()

			// given: coordinates without a host and a remote that does not name another forge
			cfg := config.Default()
			cfg.Provider = tc.provider
			cfg.Repository.GitHub = &config.GitHubRepositoryConfig{Owner: "platform", Repo: "yeet"}

			if tc.provider == config.ProviderGitLab {
				cfg.Repository.GitHub = nil
				cfg.Repository.GitLab = &config.GitLabRepositoryConfig{Project: "platform/yeet"}
			}

			// when: resolving the repository
			repository, err := resolveRepository(
				context.Background(),
				cfg,
				func(context.Context, string) (string, error) {
					return tc.remoteURL, nil
				},
				RepositoryOverrides{},
			)

			// then: the default host is used
			testastic.NoError(t, err)
			testastic.Equal(t, tc.host, repository.Host)
		})
	}

	t.Run("rejects a defaulted host when the remote is on another public forge", func(t *testing.T) {
		t.Parallel()

		// given: github coordinates without a host and a remote on gitlab.com
		cfg := githubTrustConfig("")

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@gitlab.com:platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the token is not sent to the default github host
		testastic.ErrorIs(t, err, ErrUntrustedHost)
		testastic.ErrorContains(t, err, "does not match git remote host \"gitlab.com\"")
	})

	t.Run("rejects a defaulted host for an ssh alias and hints at an explicit host", func(t *testing.T) {
		t.Parallel()

		// given: github coordinates without a host and a remote using an ssh config alias
		cfg := githubTrustConfig("")

		// when: resolving the repository
		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github-work:platform/yeet.git", nil
			},
			RepositoryOverrides{},
		)

		// then: the alias is rejected with a hint to configure the host explicitly
		testastic.ErrorIs(t, err, ErrUntrustedHost)

		var setup *SetupError
		testastic.True(t, errors.As(err, &setup))
		testastic.Equal(t, "github-work", setup.RemoteHost)
		testastic.Equal(t, "set the provider host explicitly to the forge serving this repository", setup.Hint)
	})

	t.Run("accepts a defaulted host when no git remote is available", func(t *testing.T) {
		t.Parallel()

		// given: github coordinates without a host and no readable git remote
		cfg := githubTrustConfig("")

		// when: resolving the repository
		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", ErrGitRemoteNotFound
			},
			RepositoryOverrides{},
		)

		// then: the default host is used because nothing points elsewhere
		testastic.NoError(t, err)
		testastic.Equal(t, DefaultGitHubHost, repository.Host)
	})
}

func TestResolveRepositoryDefaultedHostHonorsProviderURLEnv(t *testing.T) {
	// given: an operator-set GITLAB_URL and gitlab coordinates without a host
	t.Setenv(gitlabURLEnv, "https://gitlab.company.com/api/v4")

	cfg := config.Default()
	cfg.Provider = config.ProviderGitLab
	cfg.Repository.GitLab = &config.GitLabRepositoryConfig{Project: "group/service"}

	// when: resolving with a getter that fails if called
	repository, err := resolveRepository(
		context.Background(),
		cfg,
		func(context.Context, string) (string, error) {
			return "", errors.New("git remote lookup should not run with a provider url override")
		},
		RepositoryOverrides{},
	)

	// then: the operator-controlled endpoint is trusted without consulting the remote
	testastic.NoError(t, err)
	testastic.Equal(t, DefaultGitLabHost, repository.Host)
}
