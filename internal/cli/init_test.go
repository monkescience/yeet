package cli //nolint:testpackage // validates unexported runInit behavior directly

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monkescience/testastic"
	"github.com/monkescience/yeet/internal/config"
)

func TestRunInit(t *testing.T) {
	t.Run("root command honors config flag for init", func(t *testing.T) {
		// given: an empty temporary workspace and a custom config destination
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		cfgFile = ""

		t.Cleanup(func() {
			cfgFile = ""
		})

		command := rootCmd()
		command.SetArgs([]string{"--config", "custom.toml", "init"})

		// when: executing init through the root command with --config
		err := command.Execute()

		// then: the custom path is written instead of the default path
		testastic.NoError(t, err)

		_, statErr := os.Stat("custom.toml")
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

		cfgFile = ""

		t.Cleanup(func() {
			cfgFile = ""
		})

		path := filepath.Join("missing", "custom.toml")
		command := rootCmd()
		command.SetArgs([]string{"--config", path, "init"})

		// when: executing init through the root command with a missing parent directory
		err := command.Execute()

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
		err := runInit("custom.toml")

		// then: config is created at the requested path and can be parsed
		testastic.NoError(t, err)

		content, readErr := os.ReadFile("custom.toml")
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

		firstErr := runInit("custom.toml")
		testastic.NoError(t, firstErr)

		// when: initializing config again at the same custom path
		err := runInit("custom.toml")

		// then: command fails with existing-config error
		testastic.Error(t, err)
		testastic.ErrorIs(t, err, ErrConfigExists)
	})
}
