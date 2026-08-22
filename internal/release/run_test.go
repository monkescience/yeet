package release //nolint:testpackage // validates unexported release option precedence directly

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
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
			cfg, err := prepare(context.Background(), Options{DryRun: true})

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
		_, err := prepare(t.Context(), Options{DryRun: true})

		// then: the coordinates are rejected instead of being silently discarded
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
		testastic.Equal(
			t,
			"load release config: load config: invalid config: repository.github set but provider is auto. "+
				"Set an explicit provider",
			err.Error(),
		)
	})

	t.Run("invalid auto merge method override is rejected", func(t *testing.T) {
		// given: a valid config with an unsupported auto-merge method override
		t.Chdir(t.TempDir())
		clearCurrentBranchEnv(t)
		writeTestConfig(t, func(_ *config.Config) {})

		// when: preparing a dry-run release
		_, err := prepare(t.Context(), Options{
			AutoMerge:       new(true),
			AutoMergeMethod: new("wrongo"),
			DryRun:          true,
		})

		// then: option validation rejects the method with a specific diagnostic
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
		_, err := prepare(t.Context(), Options{DryRun: true})

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

func TestRepositoryOverrides(t *testing.T) {
	// given: release options containing each optional repository override
	options := Options{
		Provider:          new(string(config.ProviderGitHub)),
		RepositoryRemote:  new("upstream"),
		RepositoryHost:    new("github.company.com"),
		RepositoryOwner:   new("platform"),
		RepositoryRepo:    new("yeet"),
		RepositoryProject: new("platform/yeet"),
	}

	// when: release wiring builds the provider-owned record
	overrides := repositoryOverrides(options)

	// then: pointer identity preserves omitted versus explicitly empty values
	testastic.Equal(t, options.Provider, overrides.Provider)
	testastic.Equal(t, options.RepositoryRemote, overrides.Remote)
	testastic.Equal(t, options.RepositoryHost, overrides.Host)
	testastic.Equal(t, options.RepositoryOwner, overrides.Owner)
	testastic.Equal(t, options.RepositoryRepo, overrides.Repo)
	testastic.Equal(t, options.RepositoryProject, overrides.Project)
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
