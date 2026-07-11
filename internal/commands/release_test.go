package commands //nolint:testpackage // validates unexported release helpers directly

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/release"
	"go.yaml.in/yaml/v4"
)

func TestReleaseCommand(t *testing.T) {
	t.Run("reports invalid configuration", func(t *testing.T) {
		// given: a config file with an invalid enum value
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Versioning = "broken"
		})

		// when: running release with the invalid config
		_, _, err := executeCommand(t, "release")

		// then: the CLI categorizes the failure as configuration-related
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid configuration")
		testastic.ErrorContains(t, err, "versioning must be")
	})

	t.Run("loads config from a nested directory", func(t *testing.T) {
		// given: a root config file and execution from a nested subdirectory
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, config.DefaultFile)

		cfg := config.Default()
		cfg.Versioning = "broken"
		cfg.Targets = map[string]config.Target{
			"default": {Type: config.TargetTypePath, Path: ".", TagPrefix: "v"},
		}

		data, err := yaml.Marshal(cfg)
		testastic.NoError(t, err)

		err = os.WriteFile(configPath, data, 0o644)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(tempDir, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: running release from the nested directory
		_, _, err = executeCommand(t, "release")

		// then: the ancestor config is loaded instead of reporting a missing file
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid configuration")
		testastic.ErrorContains(t, err, "versioning must be")
	})

	t.Run("provider flag overrides unsupported host auto detection", func(t *testing.T) {
		// given: repository coordinates on an unknown host with an explicit github
		// provider, and an operator-set GITHUB_URL that trusts that host
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		clearBranchEnv(t)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Provider = config.ProviderGitHub
			cfg.Repository.GitHub = &config.GitHubRepositoryConfig{
				Host:  "code.company.com",
				Owner: "platform",
				Repo:  "yeet",
			}
		})
		t.Setenv("GITHUB_URL", "https://code.company.com/api/v3/")
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		// when: running release with an explicit github provider override
		_, _, err := executeCommand(t, "release", "--provider", "github")

		// then: repository resolution succeeds and provider setup uses the override
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "provider setup failed")
		testastic.ErrorContains(t, err, "GITHUB_TOKEN or GH_TOKEN")
	})

	t.Run("repository flags override configured provider and coordinates", func(t *testing.T) {
		// given: a gitlab config overridden by explicit github flags
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		clearBranchEnv(t)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Provider = config.ProviderGitLab
			cfg.Repository.GitLab = &config.GitLabRepositoryConfig{
				Host:    "gitlab.company.com",
				Project: "group/subgroup/service",
			}
		})
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		// when: running release with explicit github targeting flags
		_, _, err := executeCommand(t, "release", "--provider", "github", "--owner", "platform", "--repo", "yeet")

		// then: the github override wins
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "provider setup failed")
		testastic.ErrorContains(t, err, "GITHUB_TOKEN or GH_TOKEN")
	})

	t.Run("conflicting repository flags fail as invalid release options", func(t *testing.T) {
		// given: a valid config file and provider-incompatible CLI flags
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running release with gitlab provider but github-shaped --owner/--repo
		_, _, err := executeCommand(
			t,
			"release",
			"--provider",
			"gitlab",
			"--project",
			"group/subgroup/service",
			"--owner",
			"platform",
			"--repo",
			"yeet",
		)

		// then: the override set is rejected before repository resolution
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid release options")
		testastic.ErrorContains(t, err, "--owner/--repo are not valid for provider gitlab")
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
		err := applyReleaseOptions(cfg, releaseRunOptions{
			repositoryRemote:    "upstream",
			repositoryRemoteSet: true,
			repositoryHost:      "github.company.com",
			repositoryHostSet:   true,
			repositoryOwner:     "platform",
			repositoryOwnerSet:  true,
			repositoryRepo:      "yeet",
			repositoryRepoSet:   true,
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
		err := applyReleaseOptions(cfg, releaseRunOptions{
			provider:           string(config.ProviderGitHub),
			providerSet:        true,
			repositoryOwner:    "platform",
			repositoryOwnerSet: true,
			repositoryRepo:     "yeet",
			repositoryRepoSet:  true,
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

		err := applyReleaseOptions(cfg, releaseRunOptions{
			provider:           string(config.ProviderGitHub),
			providerSet:        true,
			repositoryOwner:    "platform",
			repositoryOwnerSet: true,
			repositoryRepo:     "yeet",
			repositoryRepoSet:  true,
		})
		testastic.NoError(t, err)

		// when: resolving the repository after applying overrides
		repository, err := resolveRepository(
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
		err := applyReleaseOptions(cfg, releaseRunOptions{
			repositoryProject:    "other/widgets",
			repositoryProjectSet: true,
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
		err := applyReleaseOptions(cfg, releaseRunOptions{
			repositoryOwner:    "platform",
			repositoryOwnerSet: true,
			repositoryRepo:     "yeet",
			repositoryRepoSet:  true,
		})

		// then: the override set is rejected
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "explicit --provider")
	})
}

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
		testastic.ErrorContains(t, err, "does not match git remote host")
	})

	t.Run("default public host does not require a remote lookup", func(t *testing.T) {
		t.Parallel()

		// given: the default github host
		cfg := githubTrustConfig(provider.DefaultGitHubHost)

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
		testastic.Equal(t, provider.DefaultGitHubHost, repository.Host)
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
		testastic.ErrorContains(t, err, "bare hostname")
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
		testastic.ErrorContains(t, err, "could not be verified against git remote")
	})
}

func TestResolveRepositoryHostTrustHonorsProviderURLEnv(t *testing.T) {
	// given: an operator-set GITHUB_URL and a config host that mismatches the remote
	t.Setenv("GITHUB_URL", "https://ghe-proxy.example/api/v3/")

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
		err := resolveReleaseMode(cfg, "main", releaseRunOptions{})

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
		err := resolveReleaseMode(cfg, "beta", releaseRunOptions{})

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
		err := resolveReleaseMode(cfg, "feature/demo", releaseRunOptions{})

		// then: mutating release is rejected
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrUnconfiguredReleaseBranch)
	})

	t.Run("unconfigured branch is allowed for dry run", func(t *testing.T) {
		t.Parallel()

		// given: a config with no feature branch release mode
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving release mode for dry-run on an unconfigured branch
		err := resolveReleaseMode(cfg, "feature/demo", releaseRunOptions{dryRun: true})

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
		err := resolveExplicitReleaseChannel(cfg, "beta", releaseRunOptions{channel: "alpha", channelSet: true})

		// then: the unknown channel error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrUnknownReleaseChannel)
	})

	t.Run("matching branch activates the channel", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: resolving the explicit channel on its branch
		err := resolveExplicitReleaseChannel(cfg, "beta", releaseRunOptions{channel: "beta", channelSet: true})

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
		err := resolveExplicitReleaseChannel(cfg, "main", releaseRunOptions{channel: "beta", channelSet: true})

		// then: the unconfigured branch error is returned
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrUnconfiguredReleaseBranch)
	})

	t.Run("branch mismatch is allowed for dry run", func(t *testing.T) {
		t.Parallel()

		// given: a config with a beta channel
		cfg := config.Default()
		cfg.Release.Channels = map[string]config.ReleaseChannelConfig{
			"beta": {Branch: "beta", Prerelease: "beta"},
		}

		// when: dry-running the explicit channel from a different branch
		err := resolveExplicitReleaseChannel(
			cfg,
			"main",
			releaseRunOptions{channel: "beta", channelSet: true, dryRun: true},
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
		err := resolveExplicitReleaseChannel(cfg, "beta", releaseRunOptions{channel: "  beta  ", channelSet: true})

		// then: the channel is found and activated
		testastic.NoError(t, err)
		testastic.Equal(t, "beta", cfg.ActiveChannel)
	})
}

func TestHandleReleaseResult(t *testing.T) {
	t.Parallel()

	t.Run("no plans and no releases is a no-op", func(t *testing.T) {
		t.Parallel()

		// given: an empty result
		result := &release.Result{}

		var buf bytes.Buffer

		// when: handling the result
		err := handleReleaseResult(context.Background(), &buf, result, false)

		// then: nothing is written and no error is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "", buf.String())
	})

	t.Run("finalized release without plans does not write output", func(t *testing.T) {
		t.Parallel()

		// given: a result with finalized releases but no new plans
		result := &release.Result{
			Releases: []*provider.Release{{TagName: "v1.2.3"}},
		}

		var buf bytes.Buffer

		// when: handling the result
		err := handleReleaseResult(context.Background(), &buf, result, false)

		// then: the writer is untouched (the message goes through slog) and no error is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "", buf.String())
	})

	t.Run("dry run with plans writes the dry-run output", func(t *testing.T) {
		t.Parallel()

		// given: a result with one plan and dry-run enabled
		result := &release.Result{
			Plans: []release.TargetPlan{
				{
					ID:             "default",
					CurrentVersion: "1.0.0",
					NextVersion:    "1.1.0",
					NextTag:        "v1.1.0",
					BumpType:       commit.BumpMinor,
					CommitCount:    3,
					Changelog:      "### Features\n\n- something new\n",
				},
			},
		}

		var buf bytes.Buffer

		// when: handling the result in dry-run mode
		err := handleReleaseResult(context.Background(), &buf, result, true)

		// then: the writer receives the dry-run summary
		testastic.NoError(t, err)

		output := ansi.Strip(buf.String())
		testastic.True(t, len(output) > 0)
		testastic.Contains(t, output, "v1.1.0")
	})

	t.Run("non dry run with plans does not write output", func(t *testing.T) {
		t.Parallel()

		// given: a result with one plan and dry-run disabled
		result := &release.Result{
			Plans: []release.TargetPlan{
				{ID: "default", NextTag: "v1.1.0"},
			},
		}

		var buf bytes.Buffer

		// when: handling the result without dry-run
		err := handleReleaseResult(context.Background(), &buf, result, false)

		// then: the writer is untouched and no error is returned
		testastic.NoError(t, err)
		testastic.Equal(t, "", buf.String())
	})
}

func TestWrapReleaseExecutionError(t *testing.T) {
	t.Parallel()

	t.Run("merge blocked suggests the next action", func(t *testing.T) {
		t.Parallel()

		// given: an auto-merge attempt blocked by provider readiness rules
		err := wrapReleaseExecutionError(fmt.Errorf("%w: required checks pending", provider.ErrMergeBlocked))

		// then: the top-level message explains how to proceed
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "release execution failed: merge blocked")
		testastic.ErrorContains(t, err, "--auto-merge-force")
	})

	t.Run("multiple pending PRs advises cleanup", func(t *testing.T) {
		t.Parallel()

		// given: multiple pending release PRs found
		err := wrapReleaseExecutionError(fmt.Errorf("%w: found 2", release.ErrMultiplePendingReleasePRs))

		// then: the message advises closing stale entries
		testastic.ErrorIs(t, err, release.ErrMultiplePendingReleasePRs)
		testastic.ErrorContains(t, err, "multiple pending release PRs/MRs found")
	})

	t.Run("generic error wraps with execution prefix", func(t *testing.T) {
		t.Parallel()

		// given: an unrecognized error
		err := wrapReleaseExecutionError(errors.New("unexpected failure"))

		// then: the message wraps with the generic prefix
		testastic.ErrorContains(t, err, "release execution failed: unexpected failure")
	})
}

func TestPrintDryRun(t *testing.T) {
	t.Parallel()

	t.Run("prints plan details", func(t *testing.T) {
		t.Parallel()

		// given: a result with one plan
		result := &release.Result{
			Plans: []release.TargetPlan{
				{
					ID:             "default",
					CurrentVersion: "1.0.0",
					NextVersion:    "1.1.0",
					NextTag:        "v1.1.0",
					BumpType:       commit.BumpMinor,
					CommitCount:    3,
					Changelog:      "### Features\n\n- something new\n",
				},
			},
		}

		var buf bytes.Buffer

		// when: printing the dry run
		printDryRun(&buf, result)

		// then: output matches expected layout
		output := ansi.Strip(buf.String())
		testastic.AssertFile(t, "testdata/dry_run_single_target.expected.txt", output)
	})

	t.Run("prints no changed targets for empty plans", func(t *testing.T) {
		t.Parallel()

		// given: a result with no plans
		result := &release.Result{}

		var buf bytes.Buffer

		// when: printing the dry run
		printDryRun(&buf, result)

		// then: output matches expected empty layout
		output := ansi.Strip(buf.String())
		testastic.AssertFile(t, "testdata/dry_run_empty.expected.txt", output)
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

		options := releaseRunOptions{
			autoMergeSet: true,
			autoMerge:    false,
		}

		// when: applying options
		applyReleaseBehaviorOptions(cfg, options)

		// then: the explicit flag disables both normal and forced auto-merge
		testastic.False(t, cfg.Release.AutoMerge)
		testastic.False(t, cfg.Release.AutoMergeForce)
	})

	t.Run("auto merge force implies auto merge", func(t *testing.T) {
		t.Parallel()

		// given: a config with auto merge disabled and force enabled via options
		cfg := config.Default()
		cfg.Release.AutoMerge = false

		options := releaseRunOptions{
			autoMergeForceSet: true,
			autoMergeForce:    true,
		}

		// when: applying options
		applyReleaseBehaviorOptions(cfg, options)

		// then: auto merge is enabled by force
		testastic.True(t, cfg.Release.AutoMerge)
		testastic.True(t, cfg.Release.AutoMergeForce)
	})

	t.Run("auto merge method is set", func(t *testing.T) {
		t.Parallel()

		// given: options specifying a merge method
		cfg := config.Default()

		options := releaseRunOptions{
			autoMergeMethodSet: true,
			autoMergeMethod:    string(config.AutoMergeMethodSquash),
		}

		// when: applying options
		applyReleaseBehaviorOptions(cfg, options)

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

func clearBranchEnv(t *testing.T) {
	t.Helper()

	for _, envName := range []string{"GITHUB_REF_NAME", "CI_COMMIT_BRANCH", "BRANCH_NAME"} {
		t.Setenv(envName, "")
	}
}
