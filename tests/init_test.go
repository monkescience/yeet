package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/tests/internal/fixture"
)

func TestInit(t *testing.T) {
	t.Parallel()

	t.Run("writes config at the explicit --config path", func(t *testing.T) {
		t.Parallel()

		// given: a fresh tempdir and an explicit --config target path
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "custom.yaml")

		// when: running `yeet init --config <path>`
		result := binary.RunWithOptions(
			t,
			[]string{"init", "--config", configPath},
			testastic.WithRunWorkDir(tempDir),
		)

		// then: the binary exits 0 and writes the schema-stamped config at that path
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/init/writes_config_at_the_explicit___config_path/stdout.expected.txt",
			result.Stdout,
		)

		content, err := os.ReadFile(configPath)
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/init/writes_config_at_the_explicit___config_path/config.expected.yaml",
			string(content),
		)

		_, defaultErr := os.Stat(filepath.Join(tempDir, ".yeet.yaml"))
		testastic.ErrorIs(t, defaultErr, os.ErrNotExist)
	})

	t.Run("writes config in the working dir when --config is omitted", func(t *testing.T) {
		t.Parallel()

		// given: a fresh tempdir used as the working directory, no --config flag
		tempDir := t.TempDir()

		// when: running `yeet init` from that tempdir
		result := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(tempDir))

		// then: the binary exits 0 and the config lands at ./.yeet.yaml
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(
			t,
			"testdata/init/writes_config_in_the_working_dir_when___config_is_omitted/stdout.expected.txt",
			result.Stdout,
		)

		content, err := os.ReadFile(filepath.Join(tempDir, ".yeet.yaml"))
		testastic.NoError(t, err)
		testastic.AssertFile(
			t,
			"testdata/init/writes_config_in_the_working_dir_when___config_is_omitted/config.expected.yaml",
			string(content),
		)
	})

	t.Run("fails when the config already exists", func(t *testing.T) {
		t.Parallel()

		// given: a config that was already written by a previous `yeet init`
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		first := binary.Run(t, "init", "--config", configPath)
		testastic.Equal(t, 0, first.ExitCode)

		// when: invoking `yeet init` against the same path a second time
		second := binary.Run(t, "init", "--config", configPath)

		// then: the binary exits 1 and stderr explains why
		testastic.Equal(t, 1, second.ExitCode)
		testastic.AssertFile(t, "testdata/init/config_already_exists/stderr.expected.txt", second.Stderr)
	})

	t.Run("writes the default config at the repository root from a nested directory", func(t *testing.T) {
		t.Parallel()

		// given: a nested directory inside a git repository without a config file
		repositoryPath := fixture.WriteRepo(t, "https://github.com/testorg/testrepo.git")
		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err := os.MkdirAll(nestedPath, 0o750)
		testastic.NoError(t, err)

		// when: invoking `yeet init` from the nested directory
		result := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(nestedPath))

		// then: the command succeeds and writes only at the repository root
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(t, "testdata/init/repository_root/stdout.expected.txt", result.Stdout)

		_, rootErr := os.Stat(filepath.Join(repositoryPath, ".yeet.yaml"))
		testastic.NoError(t, rootErr)

		_, nestedErr := os.Stat(filepath.Join(nestedPath, ".yeet.yaml"))
		testastic.ErrorIs(t, nestedErr, os.ErrNotExist)
	})

	t.Run("fails from a nested directory when the repository root config exists", func(t *testing.T) {
		t.Parallel()

		// given: a repository initialized once and a nested working directory
		repositoryPath := fixture.WriteRepo(t, "https://github.com/testorg/testrepo.git")
		first := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(repositoryPath))
		testastic.Equal(t, 0, first.ExitCode)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err := os.MkdirAll(nestedPath, 0o750)
		testastic.NoError(t, err)

		// when: invoking `yeet init` again from the nested directory
		second := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(nestedPath))

		// then: the command fails and names the existing repository-root config
		testastic.Equal(t, 1, second.ExitCode)
		testastic.AssertFile(t, "testdata/init/repository_root_exists/stderr.expected.txt", second.Stderr)
	})

	t.Run("fails when the explicit config parent directory is missing", func(t *testing.T) {
		t.Parallel()

		// given: an empty workspace and an explicit path below a missing directory
		workDir := t.TempDir()

		// when: invoking `yeet init --config missing/custom.yaml`
		result := binary.RunWithOptions(
			t,
			[]string{"init", "--config", filepath.Join("missing", "custom.yaml")},
			testastic.WithRunWorkDir(workDir),
		)

		// then: the command fails with the filesystem diagnostic on stderr
		testastic.Equal(t, 1, result.ExitCode)
		testastic.AssertFile(t, "testdata/init/missing_parent/stderr.expected.txt", result.Stderr)
	})

	t.Run("quiet suppresses init logs", func(t *testing.T) {
		t.Parallel()

		// given: an empty workspace
		workDir := t.TempDir()

		// when: initializing with quiet logging
		result := binary.RunWithOptions(
			t,
			[]string{"--quiet", "init"},
			testastic.WithRunWorkDir(workDir),
		)

		// then: the command succeeds without stdout or stderr
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(t, "testdata/init/quiet/stdout.expected.txt", result.Stdout)
		testastic.Equal(t, "", result.Stderr)
	})

	t.Run("verbose emits init debug logs", func(t *testing.T) {
		t.Parallel()

		// given: an empty workspace
		workDir := t.TempDir()

		// when: initializing with verbose logging
		result := binary.RunWithOptions(
			t,
			[]string{"--verbose", "init"},
			testastic.WithRunWorkDir(workDir),
		)

		// then: the command succeeds with debug and info logs on stderr
		testastic.Equal(t, 0, result.ExitCode)
		testastic.AssertFile(t, "testdata/init/verbose/stdout.expected.txt", result.Stdout)
		testastic.AssertFile(t, "testdata/init/verbose/stderr.expected.txt", result.Stderr)
	})
}
