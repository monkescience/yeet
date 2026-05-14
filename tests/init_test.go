package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/monkescience/testastic"
)

const schemaDirective = "# yaml-language-server: $schema=" +
	"https://raw.githubusercontent.com/monkescience/yeet/main/yeet.schema.json"

func TestInit(t *testing.T) {
	t.Run("writes config at explicit path", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		result := binary.Run(t, "init", "--config", configPath)

		testastic.Equal(t, 0, result.ExitCode)
		testastic.Equal(t, "", result.Stdout)

		content, err := os.ReadFile(configPath)
		testastic.NoError(t, err)
		testastic.HasPrefix(t, string(content), schemaDirective+"\n")
	})

	t.Run("writes config at cwd when --config omitted", func(t *testing.T) {
		tempDir := t.TempDir()

		result := binary.RunWithOptions(t, []string{"init"}, testastic.WithRunWorkDir(tempDir))

		testastic.Equal(t, 0, result.ExitCode)

		content, err := os.ReadFile(filepath.Join(tempDir, ".yeet.yaml"))
		testastic.NoError(t, err)
		testastic.HasPrefix(t, string(content), schemaDirective+"\n")
	})

	t.Run("fails when config already exists", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".yeet.yaml")

		first := binary.Run(t, "init", "--config", configPath)
		testastic.Equal(t, 0, first.ExitCode)

		second := binary.Run(t, "init", "--config", configPath)

		testastic.Equal(t, 1, second.ExitCode)
		testastic.Contains(t, second.Stderr, "config file already exists")
	})
}
