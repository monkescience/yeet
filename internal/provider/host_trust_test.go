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
	)

	// then: the operator-controlled env var is trusted regardless of the remote
	testastic.NoError(t, err)
	testastic.Equal(t, "github.company.com", repository.Host)
}

func TestResolveRepositoryAPIURLHostTrust(t *testing.T) {
	t.Parallel()

	t.Run("accepts a configured API URL on the repository host", func(t *testing.T) {
		t.Parallel()

		cfg := githubTrustConfig("github.company.com")
		cfg.Repository.GitHub.APIURL = "https://github.company.com/root/api/v3"

		repository, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.company.com:platform/yeet.git", nil
			},
		)

		testastic.NoError(t, err)
		testastic.Equal(t, cfg.Repository.GitHub.APIURL, repository.APIURL)
	})

	t.Run("rejects a configured API URL on another host", func(t *testing.T) {
		t.Parallel()

		cfg := githubTrustConfig("github.company.com")
		cfg.Repository.GitHub.APIURL = "https://credentials.example/api/v3"

		_, err := resolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "git@github.company.com:platform/yeet.git", nil
			},
		)

		testastic.ErrorIs(t, err, ErrUntrustedHost)
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
	)

	// then: both the trust sentinel and the underlying cause are recoverable
	testastic.ErrorIs(t, err, ErrUntrustedHost)
	testastic.ErrorIs(t, err, ErrGitRemoteNotFound)
}
