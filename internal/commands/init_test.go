package commands //nolint:testpackage // validates unexported runInit behavior directly

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	git "github.com/go-git/go-git/v6"
	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/spf13/cobra"
)

func TestRunInit(t *testing.T) {
	t.Run("root command honors config flag for init", func(t *testing.T) {
		// given: an empty temporary workspace and a custom config destination
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: executing init through the root command with --config
		_, _, err := executeRootCommand(t, "--config", "custom.yaml", "init")

		// then: the custom path is written instead of the default path
		testastic.NoError(t, err)

		_, statErr := os.Stat("custom.yaml")
		testastic.NoError(t, statErr)

		_, statErr = os.Stat(config.DefaultFile)
		testastic.Error(t, statErr)

		if !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to be absent, got %v", config.DefaultFile, statErr)
		}
	})

	t.Run("root command fails when config parent directory is missing", func(t *testing.T) {
		// given: an empty temporary workspace and a missing parent directory in the requested path
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		path := filepath.Join("missing", "custom.yaml")

		// when: executing init through the root command with a missing parent directory
		_, _, err := executeRootCommand(t, "--config", path, "init")

		// then: command fails with a not-exist error that mentions the requested path
		testastic.Error(t, err)

		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected not-exist error, got %v", err)
		}

		pathErr := &os.PathError{Op: "open", Path: path, Err: syscall.ENOENT}
		testastic.Equal(t, "write "+path+": "+pathErr.Error(), err.Error())

		_, statErr := os.Stat(config.DefaultFile)
		testastic.Error(t, statErr)

		if !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to be absent, got %v", config.DefaultFile, statErr)
		}
	})

	t.Run("root command writes default config at repository root from nested directory", func(t *testing.T) {
		// given: a nested directory inside a git repository without an existing config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "cmd", "yeet")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: executing init from the nested directory
		_, _, err = executeRootCommand(t, "init")

		// then: the config file is created at the repository root
		testastic.NoError(t, err)

		_, statErr := os.Stat(filepath.Join(repositoryPath, config.DefaultFile))
		testastic.NoError(t, statErr)

		_, statErr = os.Stat(filepath.Join(nestedPath, config.DefaultFile))
		testastic.Error(t, statErr)

		if !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to be absent in nested directory, got %v", config.DefaultFile, statErr)
		}
	})

	t.Run("root command fails when repository root config already exists", func(t *testing.T) {
		// given: a nested directory below an existing repository root config file
		repositoryPath := t.TempDir()
		_, err := git.PlainInit(repositoryPath, false)
		testastic.NoError(t, err)

		err = os.WriteFile(filepath.Join(repositoryPath, config.DefaultFile), []byte(config.SchemaDirective+"\n"), 0o644)
		testastic.NoError(t, err)

		nestedPath := filepath.Join(repositoryPath, "internal", "cli")
		err = os.MkdirAll(nestedPath, 0o755)
		testastic.NoError(t, err)
		t.Chdir(nestedPath)

		// when: executing init from the nested directory
		_, _, err = executeRootCommand(t, "init")

		// then: init reports that the repository root config already exists
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
		testastic.Equal(
			t,
			"config file already exists: "+filepath.Join(repositoryPath, config.DefaultFile),
			err.Error(),
		)
	})

	t.Run("writes minimal config with target named after the config directory", func(t *testing.T) {
		// given: a workspace whose basename is a valid bare YAML key
		parentDir := t.TempDir()
		projectDir := filepath.Join(parentDir, "my-cool-app")
		err := os.MkdirAll(projectDir, 0o755)
		testastic.NoError(t, err)
		t.Chdir(projectDir)

		// when: initializing config
		err = runInit(t.Context(), config.DefaultFile)
		testastic.NoError(t, err)

		// then: config is minimal, parseable, and names the target after the directory
		content, readErr := os.ReadFile(config.DefaultFile)
		testastic.NoError(t, readErr)

		contentStr := string(content)
		testastic.AssertFile(
			t,
			commandTestFilePath(
				t,
				"testdata/run_init/writes_minimal_config_with_target_named_after_the_config_directory/"+
					"config.expected.yaml",
			),
			contentStr,
		)

		cfg, parseErr := config.Parse(content)
		testastic.NoError(t, parseErr)

		_, exists := cfg.Targets["my-cool-app"]
		testastic.True(t, exists)
	})

	t.Run("falls back to root target name when directory basename is not a safe bare key", func(t *testing.T) {
		// given: a workspace whose basename starts with a dot
		parentDir := t.TempDir()
		projectDir := filepath.Join(parentDir, ".hidden")
		err := os.MkdirAll(projectDir, 0o755)
		testastic.NoError(t, err)
		t.Chdir(projectDir)

		// when: initializing config
		err = runInit(t.Context(), config.DefaultFile)
		testastic.NoError(t, err)

		// then: the target name falls back to "root"
		content, readErr := os.ReadFile(config.DefaultFile)
		testastic.NoError(t, readErr)

		contentStr := string(content)
		testastic.AssertFile(
			t,
			commandTestFilePath(
				t,
				"testdata/run_init/"+
					"falls_back_to_root_target_name_when_directory_basename_is_not_a_safe_bare_key/"+
					"config.expected.yaml",
			),
			contentStr,
		)

		cfg, parseErr := config.Parse(content)
		testastic.NoError(t, parseErr)

		_, exists := cfg.Targets["root"]
		testastic.True(t, exists)
	})
}

func TestRootCommand(t *testing.T) {
	t.Run("completion command is available for bash", func(t *testing.T) {
		// given: the root command tree
		command := NewRoot()

		// when: resolving the bash completion subcommand
		completionCommand, _, err := command.Find([]string{"completion", "bash"})

		// then: cobra exposes the bash completion command
		testastic.NoError(t, err)
		testastic.Equal(t, "bash", completionCommand.Name())
		testastic.Equal(t, "yeet completion bash", completionCommand.CommandPath())
	})

	t.Run("quiet suppresses init info logs", func(t *testing.T) {
		// given: an empty temporary workspace
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: initializing config with quiet logging
		stdout, stderr, err := executeRootCommand(t, "--quiet", "init")

		// then: config is created without emitting info logs
		testastic.NoError(t, err)
		testastic.Equal(t, "", stdout)
		testastic.Equal(t, "", stderr)

		_, statErr := os.Stat(config.DefaultFile)
		testastic.NoError(t, statErr)
	})

	t.Run("verbose emits debug logs for init", func(t *testing.T) {
		// given: an empty temporary workspace
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: initializing config with verbose logging
		stdout, stderr, err := executeRootCommand(t, "--verbose", "init")

		// then: debug and info logs are emitted to stderr
		testastic.NoError(t, err)
		testastic.Equal(t, "", stdout)
		testastic.AssertFile(
			t,
			commandTestFilePath(t, "testdata/root_command/verbose_emits_debug_logs_for_init/stderr.expected.txt"),
			stderr,
		)
	})
}

func executeRootCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer

	var stderr bytes.Buffer

	command := NewRoot()
	setCommandWriters(command, &stdout, &stderr)
	command.SetArgs(args)

	previousLogger := slog.Default()

	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	err := command.Execute()

	return stdout.String(), stderr.String(), err
}

func setCommandWriters(command *cobra.Command, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	command.SetOut(stdout)
	command.SetErr(stderr)

	for _, subcommand := range command.Commands() {
		setCommandWriters(subcommand, stdout, stderr)
	}
}
