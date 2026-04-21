package cli //nolint:testpackage // validates unexported runInit behavior directly

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
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
		_, _, err := executeCommand(t, "--config", "custom.yaml", "init")

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
		_, _, err := executeCommand(t, "--config", path, "init")

		// then: command fails with a not-exist error that mentions the requested path
		testastic.Error(t, err)

		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected not-exist error, got %v", err)
		}

		if !strings.Contains(err.Error(), path) {
			t.Fatalf("expected error to mention %s, got %v", path, err)
		}

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
		_, _, err = executeCommand(t, "init")

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
		_, _, err = executeCommand(t, "init")

		// then: init reports that the repository root config already exists
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
		testastic.ErrorContains(t, err, filepath.Join(repositoryPath, config.DefaultFile))
	})

	t.Run("writes config with schema directive", func(t *testing.T) {
		// given: an empty temporary workspace
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: initializing config
		err := runInit(config.DefaultFile)

		// then: config is created with schema directive and can be parsed
		testastic.NoError(t, err)

		content, readErr := os.ReadFile(config.DefaultFile)
		testastic.NoError(t, readErr)
		testastic.HasPrefix(t, string(content), config.SchemaDirective+"\n")

		_, parseErr := config.Parse(content)
		testastic.NoError(t, parseErr)
	})

	t.Run("writes requested config path with schema directive", func(t *testing.T) {
		// given: an empty temporary workspace and a custom config path
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		// when: initializing config at the custom path
		err := runInit("custom.yaml")

		// then: config is created at the requested path and can be parsed
		testastic.NoError(t, err)

		content, readErr := os.ReadFile("custom.yaml")
		testastic.NoError(t, readErr)
		testastic.HasPrefix(t, string(content), config.SchemaDirective+"\n")

		_, parseErr := config.Parse(content)
		testastic.NoError(t, parseErr)
	})

	t.Run("fails when config already exists", func(t *testing.T) {
		// given: a workspace where config was already created
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		firstErr := runInit(config.DefaultFile)
		testastic.NoError(t, firstErr)

		// when: initializing config again
		err := runInit(config.DefaultFile)

		// then: command fails with existing-config error
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
	})

	t.Run("fails when requested config already exists", func(t *testing.T) {
		// given: a workspace where the custom config path was already created
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		firstErr := runInit("custom.yaml")
		testastic.NoError(t, firstErr)

		// when: initializing config again at the same custom path
		err := runInit("custom.yaml")

		// then: command fails with existing-config error
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
	})
}

func TestRootCommand(t *testing.T) {
	t.Run("root help includes getting started examples", func(t *testing.T) {
		// given: the root command help is requested

		// when: rendering the top-level help output
		stdout, stderr, err := executeCommand(t, "--help")

		// then: the root help text matches the expected CLI contract
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)
		testastic.AssertFile(t, "testdata/root_help.expected.txt", stdout)
	})

	t.Run("init help includes default and custom config examples", func(t *testing.T) {
		// given: the init command help is requested

		// when: rendering init help output
		stdout, stderr, err := executeCommand(t, "init", "--help")

		// then: the init help text matches the expected CLI contract
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)
		testastic.AssertFile(t, "testdata/init_help.expected.txt", stdout)
	})

	t.Run("version prints build information", func(t *testing.T) {
		// given: build metadata provided by the build package (ldflag or ReadBuildInfo fallback)

		// when: printing the CLI version
		stdout, stderr, err := executeCommand(t, "version")

		// then: three human-readable lines are written to stdout with non-empty values
		testastic.NoError(t, err)
		testastic.Equal(t, "", stderr)

		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines, got %d: %q", len(lines), stdout)
		}

		for index, prefix := range []string{"version: ", "commit: ", "built: "} {
			if !strings.HasPrefix(lines[index], prefix) {
				t.Errorf("line %d %q missing prefix %q", index, lines[index], prefix)
			}

			if strings.TrimSpace(strings.TrimPrefix(lines[index], prefix)) == "" {
				t.Errorf("line %d %q has empty value after prefix %q", index, lines[index], prefix)
			}
		}
	})

	t.Run("completion command is available for bash", func(t *testing.T) {
		// given: the root command tree
		command := rootCmd()

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
		stdout, stderr, err := executeCommand(t, "--quiet", "init")

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
		stdout, stderr, err := executeCommand(t, "--verbose", "init")

		// then: debug and info logs are emitted to stderr
		testastic.NoError(t, err)
		testastic.Equal(t, "", stdout)
		testastic.Contains(t, stderr, "DEBU")
		testastic.Contains(t, stderr, "initializing config file")
		testastic.Contains(t, stderr, "INFO")
		testastic.Contains(t, stderr, "created config file")
	})

	t.Run("verbose and quiet flags conflict", func(t *testing.T) {
		// given: conflicting root logging flags

		// when: executing any command with both flags enabled
		_, _, err := executeCommand(t, "--verbose", "--quiet", "version")

		// then: the command fails before running the subcommand
		testastic.Error(t, err)
		testastic.ErrorContains(t, err, "--verbose and --quiet cannot be used together")
	})
}

func executeCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer

	var stderr bytes.Buffer

	command := rootCmd()
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
