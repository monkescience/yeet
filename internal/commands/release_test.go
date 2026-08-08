package commands //nolint:testpackage // validates unexported release helpers directly

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
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
		_, _, err := executeRootCommand(t, "release")

		// then: the CLI categorizes the failure as configuration-related
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid configuration: load config: invalid config: versioning must be \"semver\" or "+
				"\"calver\", got \"broken\"",
			err.Error(),
		)
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
		_, _, err = executeRootCommand(t, "release")

		// then: the ancestor config is loaded instead of reporting a missing file
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"invalid configuration: load config: invalid config: versioning must be \"semver\" or "+
				"\"calver\", got \"broken\"",
			err.Error(),
		)
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
		_, _, err := executeRootCommand(t, "release", "--provider", "github")

		// then: repository resolution succeeds and provider setup uses the override
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"provider setup failed: missing auth token: GITHUB_TOKEN or GH_TOKEN environment variable "+
				"is required",
			err.Error(),
		)
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
		_, _, err := executeRootCommand(t, "release", "--provider", "github", "--owner", "platform", "--repo", "yeet")

		// then: the github override wins
		testastic.Error(t, err)
		testastic.Equal(
			t,
			"provider setup failed: missing auth token: GITHUB_TOKEN or GH_TOKEN environment variable "+
				"is required",
			err.Error(),
		)
	})

	t.Run("rejects Azure Pipelines non-branch ref without channels", func(t *testing.T) {
		// given: a tag-triggered Azure Pipeline and a stable-only release config
		t.Chdir(t.TempDir())
		clearBranchEnv(t)
		t.Setenv("BUILD_SOURCEBRANCH", "refs/tags/v1.2.3")
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running a mutating release
		_, _, err := executeRootCommand(t, "release")

		// then: the non-branch ref is rejected before stable release fallback
		testastic.Error(t, err)
		testastic.Equal(t, "resolve current branch: ci ref is not a branch: \"refs/tags/v1.2.3\"", err.Error())
	})

	t.Run("rejects GitHub Actions non-branch ref without channels", func(t *testing.T) {
		// given: a tag-triggered GitHub workflow and a stable-only release config
		t.Chdir(t.TempDir())
		clearBranchEnv(t)
		t.Setenv("GITHUB_REF", "refs/tags/v1.2.3")
		t.Setenv("GITHUB_REF_NAME", "v1.2.3")
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running a mutating release
		_, _, err := executeRootCommand(t, "release")

		// then: the non-branch ref is rejected before stable release fallback
		testastic.Error(t, err)
		testastic.Equal(t, "resolve current branch: ci ref is not a branch: \"refs/tags/v1.2.3\"", err.Error())
	})

	t.Run("conflicting repository flags fail as invalid release options", func(t *testing.T) {
		// given: a valid config file and provider-incompatible CLI flags
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running release with gitlab provider but github-shaped --owner/--repo
		_, _, err := executeRootCommand(
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
		testastic.Equal(
			t,
			"invalid release options: invalid config: --owner/--repo are not valid for provider "+
				"gitlab. Use --project",
			err.Error(),
		)
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
			Releases: []release.FinalizedRelease{{
				TargetID:  "default",
				CommitSHA: "abc1234",
				Release:   &provider.Release{TagName: "v1.2.3"},
			}},
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
					Entry:          changelog.ParseEntry("## v1.1.0 (2026-03-01)\n\n### Features\n\n- something new\n"),
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
		testastic.AssertFile(
			t,
			commandTestFilePath(
				t,
				"testdata/handle_release_result/dry_run_with_plans_writes_the_dry_run_output/"+
					"output.expected.txt",
			),
			output,
		)
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
		testastic.Equal(
			t,
			"release execution failed: merge blocked. Resolve pull request or merge request readiness, or use "+
				"--auto-merge-force when appropriate: release PR merge blocked: required checks pending",
			err.Error(),
		)
	})

	t.Run("multiple pending PRs advises cleanup", func(t *testing.T) {
		t.Parallel()

		// given: multiple pending release PRs found
		err := wrapReleaseExecutionError(fmt.Errorf("%w: found 2", release.ErrMultiplePendingReleasePRs))

		// then: the message advises closing stale entries
		testastic.ErrorIs(t, err, release.ErrMultiplePendingReleasePRs)
		testastic.Equal(
			t,
			"release execution failed: multiple pending release changes found "+
				"(pull requests or merge requests). Close or relabel stale entries: "+
				"multiple pending release PRs found: found 2",
			err.Error(),
		)
	})

	t.Run("generic error wraps with execution prefix", func(t *testing.T) {
		t.Parallel()

		// given: an unrecognized error
		err := wrapReleaseExecutionError(errors.New("unexpected failure"))

		// then: the message wraps with the generic prefix
		testastic.Equal(t, "release execution failed: unexpected failure", err.Error())
	})
}

func TestReleaseLogMessages(t *testing.T) {
	t.Run("finalized release log states that no new release is needed", func(t *testing.T) {
		// given: a finalized release and an info logger
		result := &release.Result{
			Releases: []release.FinalizedRelease{{
				TargetID:  "default",
				CommitSHA: "abc1234",
				Release:   &provider.Release{TagName: "v1.2.3"},
			}},
		}

		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, nil)))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		// when: handling the finalized release
		err := handleReleaseResult(t.Context(), &bytes.Buffer{}, result, false)

		// then: the log message uses plain sentence wording
		testastic.NoError(t, err)
		testastic.True(
			t,
			strings.Contains(
				logOutput.String(),
				`"msg":"release finalized with no new release needed"`,
			),
		)
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
					Entry:          changelog.ParseEntry("## v1.1.0 (2026-03-01)\n\n### Features\n\n- something new\n"),
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

	for _, envName := range []string{
		"GITHUB_REF",
		"GITHUB_REF_NAME",
		"CI_COMMIT_BRANCH",
		"BRANCH_NAME",
		"BUILD_SOURCEBRANCH",
	} {
		t.Setenv(envName, "")
	}
}
