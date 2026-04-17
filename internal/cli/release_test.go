package cli //nolint:testpackage // validates unexported release helpers directly

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
	t.Run("help matches expected release CLI contract", func(t *testing.T) {
		// given: the release command help is requested

		// when: rendering help output
		stdout, stderr, err := executeCommand(t, "release", "--help")

		// then: the release help text matches the expected CLI contract
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)
		testastic.AssertFile(t, "testdata/release_help.expected.txt", stdout)
	})

	t.Run("reports missing config file with next step", func(t *testing.T) {
		// given: an empty workspace without a default config file
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: running release without initializing config
		_, _, err := executeCommand(t, "release")

		// then: the error points to the missing config and next action
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "configuration file not found")
		testastic.ErrorContains(t, err, config.DefaultFile)
		testastic.ErrorContains(t, err, "run `yeet init` or pass --config")
	})

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

	t.Run("reports malformed yaml as invalid configuration", func(t *testing.T) {
		// given: a config file with invalid YAML syntax
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		err := os.WriteFile(config.DefaultFile, []byte("release: ["), 0o644)
		testastic.NoError(t, err)

		// when: running release with malformed YAML
		_, _, err = executeCommand(t, "release")

		// then: the CLI keeps it in the invalid configuration category
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid configuration")
		testastic.ErrorContains(t, err, "parse config")
	})

	t.Run("reports missing auth token as provider setup failure", func(t *testing.T) {
		// given: a repository config that resolves directly to GitHub without auth tokens
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Provider = config.ProviderGitHub
			cfg.Repository.Owner = "platform"
			cfg.Repository.Repo = "yeet"
		})
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		// when: running release without provider credentials
		_, _, err := executeCommand(t, "release")

		// then: the CLI points at provider setup and the required token names
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "provider setup failed")
		testastic.ErrorContains(t, err, "GITHUB_TOKEN or GH_TOKEN")
	})

	t.Run("reports unsupported host as repository resolution failure", func(t *testing.T) {
		// given: repository coordinates on a host yeet cannot classify automatically
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Repository.Host = "code.company.com"
			cfg.Repository.Owner = "platform"
			cfg.Repository.Repo = "yeet"
		})

		// when: running release without an explicit provider
		_, _, err := executeCommand(t, "release")

		// then: the CLI categorizes the failure as repository resolution
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "repository resolution failed")
		testastic.ErrorContains(t, err, "unsupported remote host")
	})

	t.Run("provider flag overrides unsupported host auto detection", func(t *testing.T) {
		// given: repository coordinates on an unknown host plus an explicit provider flag
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Repository.Host = "code.company.com"
			cfg.Repository.Owner = "platform"
			cfg.Repository.Repo = "yeet"
		})
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
		writeTestConfig(t, func(cfg *config.Config) {
			cfg.Provider = config.ProviderGitLab
			cfg.Repository.Host = "gitlab.company.com"
			cfg.Repository.Project = "group/subgroup/service"
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
		// given: a valid config file and conflicting explicit repository flags
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running release with mismatched project and owner repo overrides
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

		// then: the override set is validated before repository resolution
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid release options")
		testastic.ErrorContains(t, err, "repository.project must match repository.owner/repo")
	})
}

func TestApplyReleaseOptions(t *testing.T) {
	t.Parallel()

	t.Run("repository overrides update config when set", func(t *testing.T) {
		t.Parallel()

		// given: a config with existing repository values
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.Remote = "origin"
		cfg.Repository.Host = "github.com"
		cfg.Repository.Owner = "platform"
		cfg.Repository.Repo = "yeet"

		// when: applying explicit repository overrides
		applyReleaseOptions(cfg, releaseRunOptions{
			provider:             string(config.ProviderGitLab),
			providerSet:          true,
			repositoryRemote:     "upstream",
			repositoryRemoteSet:  true,
			repositoryHost:       "gitlab.company.com",
			repositoryHostSet:    true,
			repositoryOwner:      "group/subgroup",
			repositoryOwnerSet:   true,
			repositoryRepo:       "service",
			repositoryRepoSet:    true,
			repositoryProject:    "group/subgroup/service",
			repositoryProjectSet: true,
		})

		// then: the overrides become the effective release config
		testastic.Equal(t, config.ProviderGitLab, cfg.Provider)
		testastic.Equal(t, "upstream", cfg.Repository.Remote)
		testastic.Equal(t, "gitlab.company.com", cfg.Repository.Host)
		testastic.Equal(t, "group/subgroup", cfg.Repository.Owner)
		testastic.Equal(t, "service", cfg.Repository.Repo)
		testastic.Equal(t, "group/subgroup/service", cfg.Repository.Project)
	})

	t.Run("owner and repo overrides clear stale project", func(t *testing.T) {
		t.Parallel()

		// given: a gitlab-style repository config
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.Host = "gitlab.company.com"
		cfg.Repository.Project = "group/subgroup/service"

		// when: applying github-style owner repo overrides
		applyReleaseOptions(cfg, releaseRunOptions{
			provider:           string(config.ProviderGitHub),
			providerSet:        true,
			repositoryOwner:    "platform",
			repositoryOwnerSet: true,
			repositoryRepo:     "yeet",
			repositoryRepoSet:  true,
		})

		// then: the stale project path is removed
		testastic.Equal(t, config.ProviderGitHub, cfg.Provider)
		testastic.Equal(t, "", cfg.Repository.Host)
		testastic.Equal(t, "platform", cfg.Repository.Owner)
		testastic.Equal(t, "yeet", cfg.Repository.Repo)
		testastic.Equal(t, "", cfg.Repository.Project)
	})

	t.Run("provider override without host falls back to provider default host", func(t *testing.T) {
		t.Parallel()

		// given: gitlab config overridden to github without an explicit host override
		cfg := config.Default()
		cfg.Provider = config.ProviderGitLab
		cfg.Repository.Host = "gitlab.company.com"
		cfg.Repository.Project = "group/subgroup/service"

		applyReleaseOptions(cfg, releaseRunOptions{
			provider:           string(config.ProviderGitHub),
			providerSet:        true,
			repositoryOwner:    "platform",
			repositoryOwnerSet: true,
			repositoryRepo:     "yeet",
			repositoryRepoSet:  true,
		})

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

	t.Run("project override clears stale owner and repo", func(t *testing.T) {
		t.Parallel()

		// given: a github-style repository config
		cfg := config.Default()
		cfg.Provider = config.ProviderGitHub
		cfg.Repository.Owner = "platform"
		cfg.Repository.Repo = "yeet"

		// when: applying a gitlab project override
		applyReleaseOptions(cfg, releaseRunOptions{
			provider:             string(config.ProviderGitLab),
			providerSet:          true,
			repositoryProject:    "group/subgroup/service",
			repositoryProjectSet: true,
		})

		// then: the stale owner repo pair is removed
		testastic.Equal(t, config.ProviderGitLab, cfg.Provider)
		testastic.Equal(t, "", cfg.Repository.Owner)
		testastic.Equal(t, "", cfg.Repository.Repo)
		testastic.Equal(t, "group/subgroup/service", cfg.Repository.Project)
	})
}

func TestWrapReleaseExecutionError(t *testing.T) {
	t.Run("merge blocked suggests the next action", func(t *testing.T) {
		// given: an auto-merge attempt blocked by provider readiness rules
		err := wrapReleaseExecutionError(fmt.Errorf("%w: required checks pending", provider.ErrMergeBlocked))

		// then: the top-level message explains how to proceed
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "release execution failed: merge blocked")
		testastic.ErrorContains(t, err, "--auto-merge-force")
	})

	t.Run("multiple pending PRs advises cleanup", func(t *testing.T) {
		// given: multiple pending release PRs found
		err := wrapReleaseExecutionError(fmt.Errorf("%w: found 2", release.ErrMultiplePendingReleasePRs))

		// then: the message advises closing stale entries
		testastic.ErrorIs(t, err, release.ErrMultiplePendingReleasePRs)
		testastic.ErrorContains(t, err, "multiple pending release PRs/MRs found")
	})

	t.Run("generic error wraps with execution prefix", func(t *testing.T) {
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
