package commands //nolint:testpackage // validates unexported release helpers directly

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/changelog"
	"github.com/monkescience/yeet/internal/commit"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/forge"
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
		err := executeRootCommand(t, "release")

		// then: the release failure retains typed configuration facts
		testastic.Error(t, err)
		failure := assertReleaseFailure(t, err, release.FailureConfigInvalid, filepath.Join(tempDir, config.DefaultFile))

		var validationErr *config.ValidationError
		testastic.True(t, errors.As(failure, &validationErr))
		testastic.Equal(t, "versioning must be \"semver\" or \"calver\", got \"broken\"", validationErr.Problem)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
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
		err = executeRootCommand(t, "release")

		// then: the ancestor config is loaded and its invalid fact is preserved
		testastic.Error(t, err)
		failure := assertReleaseFailure(t, err, release.FailureConfigInvalid, configPath)

		var validationErr *config.ValidationError
		testastic.True(t, errors.As(failure, &validationErr))
		testastic.Equal(t, "versioning must be \"semver\" or \"calver\", got \"broken\"", validationErr.Problem)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
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
		err := executeRootCommand(t, "release", "--provider", "github")

		// then: repository resolution succeeds and missing-token identity is retained
		testastic.Error(t, err)
		failure := assertReleaseFailure(t, err, release.FailureAuthentication, filepath.Join(tempDir, config.DefaultFile))

		var tokenErr *provider.MissingTokenError
		testastic.True(t, errors.As(failure, &tokenErr))
		testastic.Equal(t, "github", tokenErr.Provider)
		testastic.SliceEqual(t, []string{"GITHUB_TOKEN", "GH_TOKEN"}, tokenErr.Variables)
		testastic.ErrorIs(t, err, provider.ErrMissingToken)
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
		err := executeRootCommand(t, "release", "--provider", "github", "--owner", "platform", "--repo", "yeet")

		// then: the github override wins and missing-token identity is retained
		testastic.Error(t, err)
		failure := assertReleaseFailure(t, err, release.FailureAuthentication, filepath.Join(tempDir, config.DefaultFile))

		var tokenErr *provider.MissingTokenError
		testastic.True(t, errors.As(failure, &tokenErr))
		testastic.Equal(t, "github", tokenErr.Provider)
		testastic.SliceEqual(t, []string{"GITHUB_TOKEN", "GH_TOKEN"}, tokenErr.Variables)
		testastic.ErrorIs(t, err, provider.ErrMissingToken)
	})

	t.Run("rejects Azure Pipelines non-branch ref without channels", func(t *testing.T) {
		// given: a tag-triggered Azure Pipeline and a stable-only release config
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		clearBranchEnv(t)
		t.Setenv("BUILD_SOURCEBRANCH", "refs/tags/v1.2.3")
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running a mutating release
		err := executeRootCommand(t, "release")

		// then: the non-branch ref is classified as a release-branch failure
		testastic.Error(t, err)
		_ = assertReleaseFailure(t, err, release.FailureReleaseBranch, filepath.Join(tempDir, config.DefaultFile))
	})

	t.Run("rejects GitHub Actions non-branch ref without channels", func(t *testing.T) {
		// given: a tag-triggered GitHub workflow and a stable-only release config
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		clearBranchEnv(t)
		t.Setenv("GITHUB_REF", "refs/tags/v1.2.3")
		t.Setenv("GITHUB_REF_NAME", "v1.2.3")
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running a mutating release
		err := executeRootCommand(t, "release")

		// then: the non-branch ref is classified as a release-branch failure
		testastic.Error(t, err)
		_ = assertReleaseFailure(t, err, release.FailureReleaseBranch, filepath.Join(tempDir, config.DefaultFile))
	})

	t.Run("conflicting repository flags fail as invalid release options", func(t *testing.T) {
		// given: a valid config file and provider-incompatible CLI flags
		tempDir := t.TempDir()
		t.Chdir(tempDir)
		clearBranchEnv(t)
		writeTestConfig(t, func(cfg *config.Config) {})

		// when: running release with gitlab provider but github-shaped --owner/--repo
		err := executeRootCommand(
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

		// then: the override set is rejected with typed configuration facts
		testastic.Error(t, err)
		failure := assertReleaseFailure(t, err, release.FailureConfigInvalid, filepath.Join(tempDir, config.DefaultFile))

		var validationErr *config.ValidationError
		testastic.True(t, errors.As(failure, &validationErr))
		testastic.Equal(t, "--owner/--repo are not valid for provider gitlab. Use --project", validationErr.Problem)
		testastic.ErrorIs(t, err, config.ErrInvalidConfig)
	})
}

func assertReleaseFailure(t *testing.T, err error, kind release.FailureKind, configPath string) *release.Failure {
	t.Helper()

	var failure *release.Failure
	testastic.True(t, errors.As(err, &failure))
	testastic.Equal(t, kind, failure.Kind())
	testastic.Equal(t, configPath, failure.ConfigPath())

	return failure
}

func TestHandleReleaseResult(t *testing.T) {
	t.Parallel()

	t.Run("no plans and no releases is a no-op", func(t *testing.T) {
		t.Parallel()

		// given: an empty result
		result := &release.Result{}

		var buf bytes.Buffer

		// when: handling the result
		testastic.NoError(t, handleReleaseResult(context.Background(), &buf, result, false))

		// then: nothing is written
		testastic.Equal(t, "", buf.String())
	})

	t.Run("finalized release without plans does not write output", func(t *testing.T) {
		t.Parallel()

		// given: a result with finalized releases but no new plans
		result := &release.Result{
			Releases: []release.FinalizedRelease{{
				TargetID:  "default",
				CommitSHA: "abc1234",
				Release:   &forge.Release{TagName: "v1.2.3"},
			}},
		}

		var buf bytes.Buffer

		// when: handling the result
		testastic.NoError(t, handleReleaseResult(context.Background(), &buf, result, false))

		// then: the writer is untouched because the message goes through slog
		testastic.Equal(t, "", buf.String())
	})

	t.Run("dry run with plans writes the dry-run output", func(t *testing.T) {
		t.Parallel()

		// given: a result with one plan and dry-run enabled
		result := &release.Result{
			Text: &release.RenderedRelease{PROptions: forge.ReleasePROptions{
				Title:         "chore: release 1.1.0",
				Body:          dryRunTestBody,
				BaseBranch:    "main",
				ReleaseBranch: "release",
			}},
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
		testastic.NoError(t, handleReleaseResult(context.Background(), &buf, result, true))

		// then: the writer receives the dry-run summary
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
		testastic.NoError(t, handleReleaseResult(context.Background(), &buf, result, false))

		// then: the writer is untouched
		testastic.Equal(t, "", buf.String())
	})
}

func TestReleaseCategoryDiagnostic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		kind       release.FailureKind
		configPath string
		reason     release.MergeReason
		message    string
		hint       string
	}{
		{
			name:       "missing config",
			kind:       release.FailureConfigMissing,
			configPath: ".yeet.yaml",
			message:    "configuration file was not found",
			hint:       "run yeet init or pass --config",
		},
		{
			name:       "invalid config",
			kind:       release.FailureConfigInvalid,
			configPath: "config/release.yaml",
			message:    "invalid configuration",
			hint:       "fix the reported configuration values",
		},
		{
			name:    "authentication",
			kind:    release.FailureAuthentication,
			message: "missing authentication token",
		},
		{
			name:    "repository",
			kind:    release.FailureRepository,
			message: "could not resolve repository",
			hint:    "check provider settings and the configured git remote",
		},
		{
			name:    "host trust",
			kind:    release.FailureHostTrust,
			message: "provider host could not be trusted",
			hint:    "align the configured host, git remote, and provider url override",
		},
		{
			name:    "checkout",
			kind:    release.FailureCheckout,
			message: "local checkout cannot be used for release",
			hint:    "fetch full history and check out the current remote release branch",
		},
		{
			name:    "release branch",
			kind:    release.FailureReleaseBranch,
			message: "invalid release branch or channel",
			hint:    "use a configured branch or channel",
		},
		{
			name:    "release state",
			kind:    release.FailureReleaseState,
			message: "multiple pending release pull requests",
			hint:    "close or relabel stale pending release pull requests",
		},
		{
			name:    "merge blocked",
			kind:    release.FailureMergeBlocked,
			reason:  release.MergeReasonPolicy,
			message: "merge is blocked by repository policy",
			hint:    "satisfy required approvals and checks",
		},
		{
			name:    "merge timeout",
			kind:    release.FailureMergeTimeout,
			message: "merge finalization timed out",
			hint:    "inspect provider state before retrying",
		},
		{
			name:    "auto-merge unsupported",
			kind:    release.FailureAutoMergeUnsupported,
			message: "provider-managed auto-merge is unsupported",
			hint:    "check provider prerequisites or use --auto-merge-mode direct",
		},
		{
			name:    "reviewer",
			kind:    release.FailureReviewer,
			message: "release reviewer could not be applied",
			hint:    "check reviewer identity, membership, permissions, and provider limits",
		},
		{
			name:    "labels",
			kind:    release.FailureLabels,
			message: "release labels could not be applied",
			hint:    "restore or create the configured labels",
		},
		{
			name:    "unexpected",
			kind:    release.FailureUnexpected,
			message: "release could not be completed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a release-owned failure kind and its expected diagnostic facts

			// when: selecting its diagnostic category
			actual := releaseCategoryDiagnostic(testCase.kind, testCase.reason)

			// then: the command provides the expected message and recovery hint
			testastic.Equal(t, testCase.message, actual.message)
			testastic.Equal(t, testCase.hint, actual.hint)
		})
	}
}

func TestMergeDiagnostic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		reason  release.MergeReason
		message string
		hint    string
	}{
		{
			name:    "conflicts",
			reason:  release.MergeReasonConflicts,
			message: "merge is blocked by conflicts",
			hint:    "resolve conflicts on the release branch",
		},
		{
			name:    "draft",
			reason:  release.MergeReasonDraft,
			message: "release pull request is a draft",
			hint:    "mark the release pull request ready to merge",
		},
		{
			name:    "closed",
			reason:  release.MergeReasonClosed,
			message: "release pull request is closed",
			hint:    "reopen it or let the next run open a new one",
		},
		{
			name:    "policy",
			reason:  release.MergeReasonPolicy,
			message: "merge is blocked by repository policy",
			hint:    "satisfy required approvals and checks",
		},
		{
			name:    "method",
			reason:  release.MergeReasonMethod,
			message: "merge method is unavailable",
			hint:    "enable the method in the provider settings or choose another --auto-merge-method",
		},
		{
			name:    "provider",
			reason:  release.MergeReasonProvider,
			message: "provider refused the merge",
			hint:    "inspect the release pull request in the provider",
		},
		{
			name:    "unknown",
			reason:  release.MergeReasonUnknown,
			message: "merge readiness is unknown",
			hint:    "inspect the release pull request in the provider",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// given: a release-owned merge refusal reason

			// when: selecting its diagnostic category
			actual := mergeDiagnostic(testCase.reason)

			// then: the command gives reason-specific facts and remediation
			testastic.Equal(t, testCase.message, actual.message)
			testastic.Equal(t, testCase.hint, actual.hint)
		})
	}
}

func TestReleaseLogMessages(t *testing.T) {
	t.Run("finalized release log states that no new release is needed", func(t *testing.T) {
		// given: a finalized release and an info logger
		result := &release.Result{
			Releases: []release.FinalizedRelease{{
				TargetID:  "default",
				CommitSHA: "abc1234",
				Release:   &forge.Release{TagName: "v1.2.3"},
			}},
		}

		var logOutput bytes.Buffer

		previousLogger := slog.Default()

		slog.SetDefault(slog.New(slog.NewJSONHandler(&logOutput, nil)))
		t.Cleanup(func() {
			slog.SetDefault(previousLogger)
		})

		// when: handling the finalized release
		testastic.NoError(t, handleReleaseResult(t.Context(), &bytes.Buffer{}, result, false))

		// then: the log message uses plain sentence wording
		testastic.AssertJSON(t, "testdata/release_log/finalized.expected.json", logOutput.Bytes())
	})
}

func TestRenderDryRunBody(t *testing.T) {
	t.Parallel()

	t.Run("shows link destinations without terminal hyperlinks", func(t *testing.T) {
		t.Parallel()

		// given: Markdown containing an attacker-controlled link
		markdown := "[review failed checks](https://attacker.example/login)"

		// when: rendering the dry-run body
		body, err := renderDryRunBody(markdown)

		// then: the raw output shows the destination without a terminal hyperlink
		testastic.NoError(t, err)
		testastic.AssertFile(t, "testdata/dry_run_body_link.expected.txt", trimDryRunBody(body)+"\n")
	})

	t.Run("shows complete link destinations in tables", func(t *testing.T) {
		t.Parallel()

		// given: a table containing an attacker-controlled link with a long destination
		markdown := "| Status |\n| --- |\n| [review failed checks](" +
			"https://attacker.example/review/failed/checks/login?return=release-dry-run-preview) |"

		// when: rendering the dry-run body
		body, err := renderDryRunBody(markdown)

		// then: the raw output matches the inline, untruncated golden file
		testastic.NoError(t, err)
		testastic.AssertFile(t, "testdata/dry_run_body_table_link.expected.txt", trimDryRunBody(body)+"\n")
	})
}

func TestPrintDryRun(t *testing.T) {
	t.Parallel()

	t.Run("prints plan details", func(t *testing.T) {
		t.Parallel()

		// given: a result with one plan
		result := &release.Result{
			Text: &release.RenderedRelease{PROptions: forge.ReleasePROptions{
				Title:         "chore: release 1.1.0",
				Body:          dryRunTestBody,
				BaseBranch:    "main",
				ReleaseBranch: "release",
			}},
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
		testastic.NoError(t, printDryRun(&buf, result))

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
		testastic.NoError(t, printDryRun(&buf, result))

		// then: output matches expected empty layout
		output := ansi.Strip(buf.String())
		testastic.AssertFile(t, "testdata/dry_run_empty.expected.txt", output)
	})

	t.Run("prints every independent release unit", func(t *testing.T) {
		t.Parallel()

		// given: a dry run with two independently releasable units
		result := &release.Result{
			PullRequestMode: config.PullRequestModeIndependent,
			Plans: []release.TargetPlan{
				{
					ID: "api", CurrentVersion: "1.0.0", NextVersion: "1.1.0",
					NextTag: "api-v1.1.0", BumpType: commit.BumpMinor, CommitCount: 1,
				},
				{
					ID: "web", CurrentVersion: "2.0.0", NextVersion: "2.0.1",
					NextTag: "web-v2.0.1", BumpType: commit.BumpPatch, CommitCount: 2,
				},
			},
			Units: []release.UnitResult{
				{Unit: "target:api", Text: &release.RenderedRelease{PROptions: forge.ReleasePROptions{
					Title: "chore: release api", Body: dryRunTestBody,
					BaseBranch: "main", ReleaseBranch: "release-api",
				}}},
				{Unit: "target:web", Text: &release.RenderedRelease{PROptions: forge.ReleasePROptions{
					Title: "chore: release web", Body: dryRunTestBody,
					BaseBranch: "main", ReleaseBranch: "release-web",
				}}},
			},
		}

		var buf bytes.Buffer

		// when: printing the dry run
		err := printDryRun(&buf, result)

		// then: both unit previews and the expanded summary are visible
		testastic.NoError(t, err)

		output := ansi.Strip(buf.String())
		testastic.AssertFile(t, "testdata/dry_run_independent.expected.txt", output)
	})
}

const dryRunTestBody = `## ٩(^ᴗ^)۶ release created

## [v1.1.0](https://example.com/compare/v1.0.0...v1.1.0) (2026-03-01)

### Features

- something new ([abc1234](https://example.com/commit/abc1234))

<!-- yeet-release-manifest
{"base_branch":"main"}
-->

_Auto-generated preview, edit CHANGELOG.md to customize release notes._

_Made with [yeet](https://github.com/monkescience/yeet) - yeet it._
`

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
