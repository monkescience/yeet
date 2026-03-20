package cli //nolint:testpackage // validates unexported release helpers directly

import (
	"fmt"
	"os"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/provider"
	"github.com/monkescience/yeet/internal/release"
	"github.com/pelletier/go-toml/v2"
)

func TestReleaseCommand(t *testing.T) {
	t.Run("help explains auto-merge method effective default", func(t *testing.T) {
		// given: the release command help is requested

		// when: rendering help output
		stdout, stderr, err := executeCommand(t, "release", "--help")

		// then: the flag description explains the config-backed default
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)
		testastic.Contains(t, stdout, "defaults to config value; built-in default: auto")
	})

	t.Run("help includes the main release examples", func(t *testing.T) {
		// given: the release command help is requested

		// when: rendering help output
		stdout, stderr, err := executeCommand(t, "release", "--help")

		// then: the help text shows dry-run, preview, and auto-merge entry points
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)
		testastic.Contains(t, stdout, "Examples:")
		testastic.Contains(t, stdout, "yeet release --dry-run")
		testastic.Contains(t, stdout, "yeet release --preview --dry-run")
		testastic.Contains(t, stdout, "yeet release --auto-merge")
		testastic.Contains(t, stdout, "provider rules may still apply")
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
		writeTestConfig(t, config.DefaultFile, func(cfg *config.Config) {
			cfg.Versioning = "broken"
		})

		// when: running release with the invalid config
		_, _, err := executeCommand(t, "release")

		// then: the CLI categorizes the failure as configuration-related
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "invalid configuration")
		testastic.ErrorContains(t, err, "versioning must be")
	})

	t.Run("reports malformed toml as invalid configuration", func(t *testing.T) {
		// given: a config file with invalid TOML syntax
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		err := os.WriteFile(config.DefaultFile, []byte("release = ["), 0o644)
		testastic.NoError(t, err)

		// when: running release with malformed TOML
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
		writeTestConfig(t, config.DefaultFile, func(cfg *config.Config) {
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
		writeTestConfig(t, config.DefaultFile, func(cfg *config.Config) {
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
}

func TestWrapReleaseExecutionError(t *testing.T) {
	t.Run("invalid preview hash length stays in release options category", func(t *testing.T) {
		// given: a preview hash validation error from release execution
		err := wrapReleaseExecutionError(fmt.Errorf("%w: got %d", release.ErrInvalidPreviewHashLength, 0))

		// then: the top-level message stays user-focused while preserving the sentinel
		testastic.ErrorIs(t, err, release.ErrInvalidPreviewHashLength)
		testastic.ErrorContains(t, err, "invalid release options")
	})

	t.Run("merge blocked suggests the next action", func(t *testing.T) {
		// given: an auto-merge attempt blocked by provider readiness rules
		err := wrapReleaseExecutionError(fmt.Errorf("%w: required checks pending", provider.ErrMergeBlocked))

		// then: the top-level message explains how to proceed
		testastic.ErrorIs(t, err, provider.ErrMergeBlocked)
		testastic.ErrorContains(t, err, "release execution failed: merge blocked")
		testastic.ErrorContains(t, err, "--auto-merge-force")
	})
}

func writeTestConfig(t *testing.T, path string, mutate func(*config.Config)) {
	t.Helper()

	cfg := config.Default()
	mutate(cfg)

	data, err := toml.Marshal(cfg)
	testastic.NoError(t, err)

	err = os.WriteFile(path, data, 0o644)
	testastic.NoError(t, err)
}
