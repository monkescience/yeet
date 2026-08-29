package commands //nolint:testpackage // validates unexported init command wiring directly

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/telemetry"
	"github.com/spf13/cobra"
)

func TestInitCommand(t *testing.T) {
	t.Run("writes minimal config with target named after the config directory", func(t *testing.T) {
		// given: a workspace whose basename is a valid bare YAML key
		parentDir := t.TempDir()
		projectDir := filepath.Join(parentDir, "my-cool-app")
		err := os.MkdirAll(projectDir, 0o755)
		testastic.NoError(t, err)
		t.Chdir(projectDir)

		// when: initializing config
		err = config.Initialize(t.Context(), config.DefaultFile)
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

		cfg, _, parseErr := config.LoadResolved(t.Context(), config.DefaultFile)
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
		err = config.Initialize(t.Context(), config.DefaultFile)
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

		cfg, _, parseErr := config.LoadResolved(t.Context(), config.DefaultFile)
		testastic.NoError(t, parseErr)

		_, exists := cfg.Targets["root"]
		testastic.True(t, exists)
	})
}

func executeRootCommand(t *testing.T, args ...string) error {
	t.Helper()

	var stdout bytes.Buffer

	var stderr bytes.Buffer

	command := newTestRoot()
	setCommandWriters(command, &stdout, &stderr)
	command.SetArgs(args)

	previousLogger := slog.Default()

	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	return command.Execute()
}

func newTestRoot() *cobra.Command {
	return NewRoot(telemetry.New("dev"))
}

func setCommandWriters(command *cobra.Command, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	command.SetOut(stdout)
	command.SetErr(stderr)

	for _, subcommand := range command.Commands() {
		setCommandWriters(subcommand, stdout, stderr)
	}
}
