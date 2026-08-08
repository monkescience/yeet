package release //nolint:testpackage // validates unexported release option precedence directly

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"go.yaml.in/yaml/v4"
)

func TestPrepare(t *testing.T) {
	for name, environment := range map[string]map[string]string{
		"allows GitHub pull request ref for dry run": {
			"GITHUB_REF":      "refs/pull/123/merge",
			"GITHUB_REF_NAME": "123/merge",
		},
		"allows Azure pull request ref for dry run": {
			"BUILD_SOURCEBRANCH": "refs/pull/123/merge",
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given: a pull request CI ref and a stable-only release config
			t.Chdir(t.TempDir())
			clearCurrentBranchEnv(t)

			for envName, value := range environment {
				t.Setenv(envName, value)
			}

			writeTestConfig(t, func(cfg *config.Config) {})

			// when: resolving release configuration for a dry run
			cfg, err := prepare(context.Background(), config.DefaultFile, Options{DryRun: true})

			// then: stable preview mode is selected despite the synthetic PR ref
			testastic.NoError(t, err)
			testastic.NotNil(t, cfg)

			if cfg != nil {
				testastic.Equal(t, "", cfg.ActiveChannel)
			}
		})
	}

	t.Run("configured repository coordinates under provider auto are rejected", func(t *testing.T) {
		// given: a config file naming provider auto together with github coordinates
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
				Host:  "github.com",
				Owner: "platform",
				Repo:  "yeet",
			}
		})

		// when: resolving release configuration
		_, err := prepare(t.Context(), config.DefaultFile, Options{DryRun: true})

		// then: the coordinates are rejected instead of being silently discarded
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"load config: invalid config: repository.github set but provider is auto. "+
				"Set an explicit provider",
			err.Error(),
		)
	})

	t.Run("invalid provider override is rejected", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		writeTestConfig(t, func(_ *config.Config) {})

		_, err := prepare(t.Context(), config.DefaultFile, Options{
			DryRun:   true,
			Provider: new("wrongo"),
		})

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err != nil {
			testastic.Equal(
				t,
				"invalid release options: invalid config: provider must be \"auto\", \"github\", "+
					"\"gitlab\", or \"azuredevops\", got \"wrongo\"",
				err.Error(),
			)
		}
	})

	t.Run("invalid auto merge method override is rejected", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		writeTestConfig(t, func(_ *config.Config) {})

		_, err := prepare(t.Context(), config.DefaultFile, Options{
			AutoMerge:       new(true),
			AutoMergeMethod: new("wrongo"),
			DryRun:          true,
		})

		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)

		if err != nil {
			testastic.Equal(
				t,
				"invalid release options: invalid config: release.auto_merge_method must be \"auto\", "+
					"\"squash\", \"rebase\", or \"merge\", got \"wrongo\"",
				err.Error(),
			)
		}
	})

	t.Run("branch fallback log explains the selected behavior", func(t *testing.T) {
		// given: a valid config outside a git checkout with debug logging enabled
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		writeTestConfig(t, func(_ *config.Config) {})

		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		// when: resolving release configuration without a detectable branch
		_, err := prepare(t.Context(), config.DefaultFile, Options{DryRun: true})

		// then: the fallback is logged without compressed punctuation
		testastic.NoError(t, err)
		testastic.True(
			t,
			strings.Contains(
				logOutput.String(),
				`"msg":"could not determine current branch (using configured default branch)"`,
			),
		)
	})
}

func TestApplyReleaseOptions(t *testing.T) {
	t.Parallel()

	t.Run("github repository overrides update sub-section when set", func(t *testing.T) {
		t.Parallel()

		// given: a github config with existing repository values
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.Remote = "origin"
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Host:  "github.com",
			Owner: "old",
			Repo:  "old",
		}

		// when: applying explicit repository overrides
		err := applyOptions(cfg, Options{
			RepositoryRemote: new("upstream"),
			RepositoryHost:   new("github.company.com"),
			RepositoryOwner:  new("platform"),
			RepositoryRepo:   new("yeet"),
		})

		// then: the overrides become the effective release config
		testastic.NoError(t, err)
		testastic.Equal(t, config.ProviderGitHub, cfg.Provider)
		testastic.Equal(t, "upstream", cfg.Repository.Remote)
		testastic.NotNil(t, cfg.Repository.GitHub)
		testastic.Equal(t, "github.company.com", cfg.Repository.GitHub.Host)
		testastic.Equal(t, "platform", cfg.Repository.GitHub.Owner)
		testastic.Equal(t, "yeet", cfg.Repository.GitHub.Repo)
	})

	t.Run("provider switch clears the previous sub-section", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab-style repository config
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
			Host:    "gitlab.company.com",
			Project: "group/subgroup/service",
		}

		// when: switching provider to github with github overrides
		err := applyOptions(cfg, Options{
			Provider:        new(string(config.ProviderGitHub)),
			RepositoryOwner: new("platform"),
			RepositoryRepo:  new("yeet"),
		})

		// then: the gitlab sub-section is cleared and github is populated
		testastic.NoError(t, err)
		testastic.Equal(t, config.ProviderGitHub, cfg.Provider)
		testastic.Nil(t, cfg.Repository.GitLab)
		testastic.NotNil(t, cfg.Repository.GitHub)
		testastic.Equal(t, "platform", cfg.Repository.GitHub.Owner)
		testastic.Equal(t, "yeet", cfg.Repository.GitHub.Repo)
	})

	t.Run("provider override without host falls back to provider default host", func(t *testing.T) {
		t.Parallel()

		// given: gitlab config overridden to github without an explicit host override
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
			Host:    "gitlab.company.com",
			Project: "group/subgroup/service",
		}

		err := applyOptions(cfg, Options{
			Provider:        new(string(config.ProviderGitHub)),
			RepositoryOwner: new("platform"),
			RepositoryRepo:  new("yeet"),
		})
		testastic.NoError(t, err)

		// when: resolving the repository after applying overrides
		repository, err := provider.ResolveRepository(
			context.Background(),
			cfg,
			func(context.Context, string) (string, error) {
				return "", errors.New("git remote lookup should not run")
			},
		)

		// then: yeet uses the github default host instead of the stale gitlab host
		testastic.NoError(t, err)
		testastic.Equal(t, string(config.ProviderGitHub), repository.Provider)
		testastic.Equal(t, provider.DefaultGitHubHost, repository.Host)
		testastic.Equal(t, "platform", repository.Owner)
		testastic.Equal(t, "yeet", repository.Repo)
	})

	t.Run("project override on github clears stale owner and repo", func(t *testing.T) {
		t.Parallel()

		// given: a github sub-section with owner+repo
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Owner: "platform",
			Repo:  "yeet",
		}

		// when: applying a project-only github override
		err := applyOptions(cfg, Options{
			RepositoryProject: new("other/widgets"),
		})

		// then: project is set and stale owner/repo are cleared
		testastic.NoError(t, err)
		testastic.Equal(t, config.ProviderGitHub, cfg.Provider)
		testastic.NotNil(t, cfg.Repository.GitHub)
		testastic.Equal(t, "", cfg.Repository.GitHub.Owner)
		testastic.Equal(t, "", cfg.Repository.GitHub.Repo)
		testastic.Equal(t, "other/widgets", cfg.Repository.GitHub.Project)
	})

	t.Run("repository field flags require explicit provider", func(t *testing.T) {
		t.Parallel()

		// given: a config with provider auto
		cfg := config.Default()

		// when: applying github-shaped CLI overrides without --provider
		err := applyOptions(cfg, Options{
			RepositoryOwner: new("platform"),
			RepositoryRepo:  new("yeet"),
		})

		// then: the override set is rejected
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid config: repository field flags require an explicit --provider (auto cannot route "+
				"them)",
			err.Error(),
		)
	})

	t.Run("provider override to auto clears coordinates instead of failing", func(t *testing.T) {
		t.Parallel()

		// given: a github config whose provider the user overrides back to auto detection
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
			Host:  "github.company.com",
			Owner: "platform",
			Repo:  "yeet",
		}

		// when: switching the provider to auto without repository field flags
		err := applyOptions(cfg, Options{Provider: new(string(config.ProviderAuto))})

		// then: the previous provider's sub-section is discarded so detection starts clean
		testastic.NoError(t, err)
		testastic.Equal(t, config.ProviderAuto, cfg.Provider)
		testastic.Nil(t, cfg.Repository.GitHub)
		testastic.Nil(t, cfg.Repository.GitLab)
		testastic.Nil(t, cfg.Repository.AzureDevOps)
	})
}

func TestResolveReleaseMode(t *testing.T) {
	t.Parallel()

	t.Run("stable branch uses stable mode", func(t *testing.T) {
		t.Parallel()

		// given: a config with a stable branch and a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving release mode on main
		err := resolveMode(cfg, "main", Options{})

		// then: stable mode is selected
		testastic.NoError(t, err)
		testastic.Equal(t, "main", cfg.Branch)
		testastic.Equal(t, "", cfg.ActiveChannel)
	})

	t.Run("channel branch selects channel mode", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving release mode on beta
		err := resolveMode(cfg, "beta", Options{})

		// then: beta mode is selected and branch is scoped to beta
		testastic.NoError(t, err)
		testastic.Equal(t, "beta", cfg.Branch)
		testastic.Equal(t, "beta", cfg.ActiveChannel)
	})

	t.Run("unconfigured branch fails for mutating release", func(t *testing.T) {
		t.Parallel()

		// given: a config with no feature branch release mode
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving release mode on an unconfigured branch
		err := resolveMode(cfg, "feature/demo", Options{})

		// then: mutating release is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errUnconfiguredReleaseBranch)
	})

	t.Run("unconfigured branch is allowed for dry run", func(t *testing.T) {
		t.Parallel()

		// given: a config with no feature branch release mode
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving release mode for dry-run on an unconfigured branch
		err := resolveMode(cfg, "feature/demo", Options{DryRun: true})

		// then: dry-run falls back to stable branch planning
		testastic.NoError(t, err)
		testastic.Equal(t, "main", cfg.Branch)
		testastic.Equal(t, "", cfg.ActiveChannel)
	})
}

func TestResolveExplicitReleaseChannel(t *testing.T) {
	t.Parallel()

	t.Run("unknown channel is rejected", func(t *testing.T) {
		t.Parallel()

		// given: a config with one beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: requesting a channel that does not exist
		err := resolveExplicitChannel(cfg, "beta", Options{Channel: new("alpha")})

		// then: the unknown channel error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errUnknownReleaseChannel)
	})

	t.Run("matching branch activates the channel", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving the explicit channel on its branch
		err := resolveExplicitChannel(cfg, "beta", Options{Channel: new("beta")})

		// then: the channel becomes active and the branch is scoped
		testastic.NoError(t, err)
		testastic.Equal(t, "beta", cfg.Branch)
		testastic.Equal(t, "beta", cfg.ActiveChannel)
	})

	t.Run("branch mismatch is rejected for mutating release", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: requesting beta from a non-beta branch
		err := resolveExplicitChannel(cfg, "main", Options{Channel: new("beta")})

		// then: the unconfigured branch error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, errUnconfiguredReleaseBranch)
	})

	t.Run("branch mismatch is allowed for dry run", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: dry-running the explicit channel from a different branch
		err := resolveExplicitChannel(
			cfg,
			"main",
			Options{Channel: new("beta"), DryRun: true},
		)

		// then: the channel is activated despite the branch mismatch
		testastic.NoError(t, err)
		testastic.Equal(t, "beta", cfg.Branch)
		testastic.Equal(t, "beta", cfg.ActiveChannel)
	})

	t.Run("channel name whitespace is trimmed", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving with surrounding whitespace in the channel option
		err := resolveExplicitChannel(cfg, "beta", Options{Channel: new("  beta  ")})

		// then: the channel is found and activated
		testastic.NoError(t, err)
		testastic.Equal(t, "beta", cfg.ActiveChannel)
	})
}

func TestApplyReleaseBehaviorOptions(t *testing.T) {
	t.Parallel()

	t.Run("explicit auto merge false disables configured force", func(t *testing.T) {
		t.Parallel()

		// given: a config with force merge enabled and an explicit auto-merge=false option
		cfg := config.Default()
		cfg.Release.AutoMerge = true
		cfg.Release.AutoMergeForce = true

		options := Options{
			AutoMerge: new(false),
		}

		// when: applying options
		applyBehaviorOptions(cfg, options)

		// then: the explicit flag disables both normal and forced auto-merge
		testastic.False(t, cfg.Release.AutoMerge)
		testastic.False(t, cfg.Release.AutoMergeForce)
	})

	t.Run("auto merge force implies auto merge", func(t *testing.T) {
		t.Parallel()

		// given: a config with auto merge disabled and force enabled via options
		cfg := config.Default()
		cfg.Release.AutoMerge = false

		options := Options{
			AutoMergeForce: new(true),
		}

		// when: applying options
		applyBehaviorOptions(cfg, options)

		// then: auto merge is enabled by force
		testastic.True(t, cfg.Release.AutoMerge)
		testastic.True(t, cfg.Release.AutoMergeForce)
	})

	t.Run("auto merge method is set", func(t *testing.T) {
		t.Parallel()

		// given: options specifying a merge method
		cfg := config.Default()

		options := Options{
			AutoMergeMethod: new(string(config.AutoMergeMethodSquash)),
		}

		// when: applying options
		applyBehaviorOptions(cfg, options)

		// then: merge method is applied
		testastic.Equal(t, config.AutoMergeMethodSquash, cfg.Release.AutoMergeMethod)
	})
}

func writeTestConfig(t *testing.T, mutate func(*config.Config)) {
	t.Helper()

	cfg := config.Default()
	cfg.Targets = map[string]config.Target{
		"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
	}
	mutate(cfg)

	data, err := yaml.Marshal(cfg)
	testastic.NoError(t, err)

	err = os.WriteFile(config.DefaultFile, data, 0o644)
	testastic.NoError(t, err)
}
